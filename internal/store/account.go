package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// DemoAccountID 是内置演示账号的固定 ID。
// 它是一条真实的数据库记录（让抢购任务的外键约束成立），但所有 OCI 调用
// 都会在客户端池里被短路到内存模拟数据，绝不发出网络请求。
const DemoAccountID = "demo-account"

// Account 是一个 OCI 账号的公开信息，不含私钥等敏感内容。
type Account struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	TenancyOCID string    `json:"tenancy_ocid"`
	UserOCID    string    `json:"user_ocid"`
	Fingerprint string    `json:"fingerprint"`
	Region      string    `json:"region"`
	ProxyURL    string    `json:"proxy_url"`
	IsDemo      bool      `json:"is_demo"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// demoEnabled 读取演示模式开关，默认开启；未写入设置时视为启用。
func (s *Store) demoEnabled() bool {
	v, err := s.GetSetting("demo_enabled")
	if err != nil || v == "" {
		return true
	}
	return v != "false"
}

// seedDemoAccount 保证演示账号始终存在。删除或库损坏后重启会自动补回，
// 且不会覆盖用户自己修改过的任何真实账号。
func (s *Store) seedDemoAccount() error {
	placeholder, err := s.sealer.Encrypt("DEMO-ACCOUNT-NO-PRIVATE-KEY")
	if err != nil {
		return fmt.Errorf("初始化演示账号失败: %w", err)
	}
	ts := now().Format(time.RFC3339)
	_, err = s.db.Exec(`INSERT OR IGNORE INTO accounts
		(id, name, tenancy_ocid, user_ocid, fingerprint, region,
		 private_key, key_passphrase, proxy_url, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		DemoAccountID,
		"演示账号（不发真实 OCI 请求）",
		"ocid1.tenancy.oc1..demo",
		"ocid1.user.oc1..demo-api-user",
		"demo:no:real:credential",
		"ap-tokyo-1",
		placeholder, placeholder, "", ts, ts,
	)
	if err != nil {
		return fmt.Errorf("初始化演示账号失败: %w", err)
	}
	return nil
}

// AccountSecret 是账号的敏感凭据，存储时加密、读取时解密。
type AccountSecret struct {
	// PrivateKeyPEM 是 PEM 格式的 RSA 私钥全文。
	PrivateKeyPEM string `json:"private_key_pem,omitempty"`
	// Passphrase 是私钥口令，可为空。
	Passphrase string `json:"passphrase,omitempty"`
}

// AccountInput 用于创建或更新账号。
type AccountInput struct {
	Name          string
	TenancyOCID   string
	UserOCID      string
	Fingerprint   string
	Region        string
	ProxyURL      string
	PrivateKeyPEM string
	Passphrase    string
	// KeepExistingKey 为 true 时保留库中已有的私钥（用于只改代理等场景）。
	KeepExistingKey bool
}

const accountColumns = `id, name, tenancy_ocid, user_ocid, fingerprint, region,
	private_key, key_passphrase, proxy_url, created_at, updated_at`

func scanAccount(row interface{ Scan(...any) error }) (*Account, error) {
	var (
		a         Account
		privKey   string
		pass      string
		createdAt string
		updatedAt string
	)
	err := row.Scan(&a.ID, &a.Name, &a.TenancyOCID, &a.UserOCID, &a.Fingerprint,
		&a.Region, &privKey, &pass, &a.ProxyURL, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	a.CreatedAt = parseTime(createdAt)
	a.UpdatedAt = parseTime(updatedAt)
	a.IsDemo = a.ID == DemoAccountID
	return &a, nil
}

func parseTime(v string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t
		}
	}
	return time.Time{}
}

// ListAccounts 返回全部账号，按创建时间升序。
func (s *Store) ListAccounts() ([]Account, error) {
	rows, err := s.db.Query(`SELECT ` + accountColumns + ` FROM accounts ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	showDemo := s.demoEnabled()
	accounts := make([]Account, 0)
	var demo *Account
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		if a.IsDemo {
			if showDemo {
				demo = a
			}
			continue
		}
		accounts = append(accounts, *a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// 演示账号排在最前：用户未录入任何真实凭证时，一登录就能零风险试用。
	if demo != nil {
		accounts = append([]Account{*demo}, accounts...)
	}
	return accounts, nil
}

// GetAccount 按 ID 取账号。
func (s *Store) GetAccount(id string) (*Account, error) {
	if id == DemoAccountID && !s.demoEnabled() {
		return nil, ErrNotFound
	}
	row := s.db.QueryRow(`SELECT `+accountColumns+` FROM accounts WHERE id = ?`, id)
	a, err := scanAccount(row)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return a, err
}

// CreateAccount 写入新账号，私钥在落库前加密。
func (s *Store) CreateAccount(in AccountInput) (*Account, error) {
	if err := validateAccountInput(in, false); err != nil {
		return nil, err
	}
	encryptedKey, err := s.sealer.Encrypt(in.PrivateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("加密私钥失败: %w", err)
	}
	encryptedPass, err := s.sealer.Encrypt(in.Passphrase)
	if err != nil {
		return nil, fmt.Errorf("加密私钥口令失败: %w", err)
	}

	id, err := newID()
	if err != nil {
		return nil, err
	}
	ts := now()
	_, err = s.db.Exec(`INSERT INTO accounts (`+accountColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		id, in.Name, in.TenancyOCID, in.UserOCID, in.Fingerprint, in.Region,
		encryptedKey, encryptedPass, in.ProxyURL, ts, ts)
	if err != nil {
		return nil, err
	}
	return s.GetAccount(id)
}

// UpdateAccount 更新账号。KeepExistingKey 为 true 时不动私钥字段。
func (s *Store) UpdateAccount(id string, in AccountInput) (*Account, error) {
	if id == DemoAccountID {
		return nil, fmt.Errorf("演示账号不可修改，请在设置中关闭演示模式或添加真实账号")
	}
	if err := validateAccountInput(in, in.KeepExistingKey); err != nil {
		return nil, err
	}
	existing, err := s.GetAccount(id)
	if err != nil {
		return nil, err
	}

	privKey := in.PrivateKeyPEM
	if in.KeepExistingKey || strings.TrimSpace(privKey) == "" {
		secret, err := s.AccountSecret(id)
		if err != nil {
			return nil, err
		}
		privKey = secret.PrivateKeyPEM
		if in.KeepExistingKey {
			in.Passphrase = secret.Passphrase
		}
	}
	encryptedKey, err := s.sealer.Encrypt(privKey)
	if err != nil {
		return nil, err
	}
	encryptedPass, err := s.sealer.Encrypt(in.Passphrase)
	if err != nil {
		return nil, err
	}
	_ = existing

	if _, err := s.db.Exec(`UPDATE accounts SET name=?, tenancy_ocid=?, user_ocid=?, fingerprint=?,
		region=?, private_key=?, key_passphrase=?, proxy_url=?, updated_at=? WHERE id=?`,
		in.Name, in.TenancyOCID, in.UserOCID, in.Fingerprint, in.Region,
		encryptedKey, encryptedPass, in.ProxyURL, now(), id); err != nil {
		return nil, err
	}
	return s.GetAccount(id)
}

// DeleteAccount 删除账号及其关联任务。
func (s *Store) DeleteAccount(id string) error {
	if id == DemoAccountID {
		return fmt.Errorf("演示账号不可删除，可在设置中关闭演示模式")
	}
	res, err := s.db.Exec(`DELETE FROM accounts WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return ensureAffected(res, err)
}

// AccountSecret 读取并解密账号私钥。
func (s *Store) AccountSecret(id string) (AccountSecret, error) {
	var encKey, encPass string
	err := s.db.QueryRow(`SELECT private_key, key_passphrase FROM accounts WHERE id = ?`, id).
		Scan(&encKey, &encPass)
	if err == sql.ErrNoRows {
		return AccountSecret{}, ErrNotFound
	}
	if err != nil {
		return AccountSecret{}, err
	}
	key, err := s.sealer.Decrypt(encKey)
	if err != nil {
		return AccountSecret{}, fmt.Errorf("解密私钥失败: %w", err)
	}
	pass, err := s.sealer.Decrypt(encPass)
	if err != nil {
		return AccountSecret{}, fmt.Errorf("解密私钥口令失败: %w", err)
	}
	return AccountSecret{PrivateKeyPEM: key, Passphrase: pass}, nil
}

func validateAccountInput(in AccountInput, allowEmptyKey bool) error {
	if strings.TrimSpace(in.Name) == "" {
		return fmt.Errorf("账号名称不能为空")
	}
	if strings.TrimSpace(in.TenancyOCID) == "" {
		return fmt.Errorf("tenancy OCID 不能为空")
	}
	if strings.TrimSpace(in.UserOCID) == "" {
		return fmt.Errorf("user OCID 不能为空")
	}
	if strings.TrimSpace(in.Fingerprint) == "" {
		return fmt.Errorf("公钥指纹不能为空")
	}
	if strings.TrimSpace(in.Region) == "" {
		return fmt.Errorf("区域不能为空")
	}
	if !allowEmptyKey && strings.TrimSpace(in.PrivateKeyPEM) == "" {
		return fmt.Errorf("私钥不能为空")
	}
	return nil
}

func newID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("生成 ID 失败: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func ensureAffected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
