package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// Account 是一个 OCI 账号的公开信息，不含私钥等敏感内容。
type Account struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	TenancyOCID  string    `json:"tenancy_ocid"`
	UserOCID     string    `json:"user_ocid"`
	Fingerprint  string    `json:"fingerprint"`
	Region       string    `json:"region"`
	ProxyURL     string    `json:"proxy_url"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
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
	Name         string
	TenancyOCID  string
	UserOCID     string
	Fingerprint  string
	Region       string
	ProxyURL     string
	PrivateKeyPEM string
	Passphrase   string
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

	accounts := make([]Account, 0)
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, *a)
	}
	return accounts, rows.Err()
}

// GetAccount 按 ID 取账号。
func (s *Store) GetAccount(id string) (*Account, error) {
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
