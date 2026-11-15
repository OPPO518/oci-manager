// Package tlsmanager 负责在后台申请、加载与自动续期 HTTPS 证书。
//
// 采用 Let's Encrypt 的 DNS-01 挑战，通过 Cloudflare API 自动添加/删除
// _acme-challenge TXT 记录，因此不要求 80 端口可被公网访问，也不需要
// 提前把域名解析到本机。证书申请成功后通过 GetCertificate 热加载，
// 无需重启容器即可生效。
package tlsmanager

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/providers/dns/cloudflare"
	"github.com/go-acme/lego/v4/registration"

	"github.com/yourname/oci-manager/internal/store"
)

const (
	keyEnabled   = "tls_enabled"
	keyDomain    = "tls_domain"
	keyEmail     = "tls_email"
	keyToken     = "tls_cf_token" // 加密存储
	keyCADir     = "tls_ca_dir"
	keyExpiresAt = "tls_expires_at"
	keyLastError = "tls_last_error"

	certFile = "fullchain.pem"
	keyFile  = "privkey.pem"

	// 剩余有效期不足 30 天时触发续期。
	renewBefore = 30 * 24 * time.Hour
)

// ConfigInput 是后台保存 HTTPS 配置的请求体。
// CFAPIToken 留空表示沿用已保存的 Token。
type ConfigInput struct {
	Enabled    bool   `json:"enabled"`
	Domain     string `json:"domain"`
	Email      string `json:"email"`
	CFAPIToken string `json:"cf_api_token"`
	// Staging 为 true 时使用 Let's Encrypt 测试环境，用于调试，不产生真实证书。
	Staging bool `json:"staging"`
}

// Status 是返回给前端的 HTTPS 状态（永不包含 Token 明文）。
type Status struct {
	Enabled    bool   `json:"enabled"`
	Domain     string `json:"domain"`
	Email      string `json:"email"`
	HasToken   bool   `json:"has_token"`
	CertIssued bool   `json:"cert_issued"`
	ExpiresAt  string `json:"expires_at"`
	LastError  string `json:"last_error"`
	Staging    bool   `json:"staging"`
	HTTPSPort  string `json:"https_port"`
}

// Manager 管理证书生命周期。
type Manager struct {
	store    *store.Store
	dir      string
	httpsURL string // 供前端展示的监听地址，如 ":8443"

	mu      sync.Mutex // 保护申请/续期过程，避免并发触发 ACME
	current atomic.Pointer[tls.Certificate]
	domain  atomic.Value // string
	expires atomic.Value // time.Time
}

// New 创建证书管理器。dir 为证书存放目录，通常在数据卷下。
func New(st *store.Store, dir, httpsAddr string) *Manager {
	m := &Manager{store: st, dir: dir, httpsURL: httpsAddr}
	m.domain.Store("")
	m.expires.Store(time.Time{})
	return m
}

// Load 在进程启动时加载磁盘上已签发的证书。
// 找不到文件或文件损坏时仅记录日志，不影响 HTTP 服务启动。
func (m *Manager) Load() {
	if err := os.MkdirAll(m.dir, 0o700); err != nil {
		log.Printf("创建证书目录失败: %v", err)
		return
	}
	cert, err := m.readCert()
	if err != nil {
		log.Printf("未加载 HTTPS 证书（尚未配置或已损坏）: %v", err)
		return
	}
	m.current.Store(cert)
	m.domain.Store(leafDomain(cert.Leaf))
	m.expires.Store(cert.Leaf.NotAfter)
	log.Printf("已加载 HTTPS 证书：%s，有效期至 %s",
		leafDomain(cert.Leaf), cert.Leaf.NotAfter.Format("2006-01-02"))
}

// leafDomain 从证书的 SAN 中取域名。Go 1.27 已移除 x509.Certificate.CommonName，
// 而 Let's Encrypt 证书必然带有 DNS SAN，因此以 DNSNames 为准。
func leafDomain(leaf *x509.Certificate) string {
	if leaf == nil {
		return ""
	}
	if len(leaf.DNSNames) > 0 {
		return leaf.DNSNames[0]
	}
	return ""
}

func (m *Manager) readCert() (*tls.Certificate, error) {
	pair, err := tls.LoadX509KeyPair(
		filepath.Join(m.dir, certFile),
		filepath.Join(m.dir, keyFile),
	)
	if err != nil {
		return nil, err
	}
	// 解析叶子证书以取得域名与过期时间。
	block, _ := pem.Decode(pair.Certificate[0])
	if block == nil {
		return nil, errors.New("证书 PEM 解析失败")
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("解析证书失败: %w", err)
	}
	pair.Leaf = leaf
	return &pair, nil
}

// GetCertificate 供 tls.Config 使用，实现证书热加载：
// 后台新申请成功后原子替换指针，正在运行的 HTTPS 监听立即生效。
func (m *Manager) GetCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	cert := m.current.Load()
	if cert == nil {
		return nil, errors.New("尚未配置 HTTPS 证书，请先在「设置」页启用")
	}
	return cert, nil
}

// HasCertificate 报告当前是否已有可用证书。
func (m *Manager) HasCertificate() bool { return m.current.Load() != nil }

// TLSConfig 构造用于 HTTPS 监听的 tls.Config。
func (m *Manager) TLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		// 只允许 ECDHE + AEAD 套件，排除 TLS 1.2 下可能协商到的 CBC / 静态 RSA 套件。
		// TLS 1.3 使用固定的安全套件，不受此列表影响。
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		},
		GetCertificate: m.GetCertificate,
	}
}

// Status 返回当前 HTTPS 配置与证书状态。
func (m *Manager) Status() (Status, error) {
	out := Status{HTTPSPort: m.httpsURL}
	get := func(key string) string {
		v, err := m.store.GetSetting(key)
		if err != nil {
			log.Printf("读取设置 %s 失败: %v", key, err)
		}
		return v
	}
	token, _ := m.store.GetSecretSetting(keyToken)

	out.Enabled = get(keyEnabled) == "1"
	out.Domain = get(keyDomain)
	out.Email = get(keyEmail)
	out.HasToken = token != ""
	out.Staging = get(keyCADir) == lego.LEDirectoryStaging
	out.ExpiresAt = get(keyExpiresAt)
	out.LastError = get(keyLastError)
	out.CertIssued = m.HasCertificate()
	if c := m.current.Load(); c != nil && c.Leaf != nil {
		out.ExpiresAt = c.Leaf.NotAfter.UTC().Format(time.RFC3339)
	}
	return out, nil
}

// savedConfig 从数据库还原完整的申请配置。
func (m *Manager) savedConfig() (ConfigInput, error) {
	get := func(key string) (string, error) { return m.store.GetSetting(key) }
	domain, err := get(keyDomain)
	if err != nil {
		return ConfigInput{}, err
	}
	email, err := get(keyEmail)
	if err != nil {
		return ConfigInput{}, err
	}
	token, err := m.store.GetSecretSetting(keyToken)
	if err != nil {
		return ConfigInput{}, err
	}
	caDir, err := get(keyCADir)
	if err != nil {
		return ConfigInput{}, err
	}
	enabled, err := get(keyEnabled)
	if err != nil {
		return ConfigInput{}, err
	}
	return ConfigInput{
		Enabled:    enabled == "1",
		Domain:     domain,
		Email:      email,
		CFAPIToken: token,
		Staging:    caDir == lego.LEDirectoryStaging,
	}, nil
}

// Apply 保存配置并立即申请证书。
// 申请是同步的（DNS 传播通常需要 1–3 分钟），前端应展示加载状态。
func (m *Manager) Apply(in ConfigInput) error {
	domain := strings.ToLower(strings.TrimSpace(in.Domain))
	if in.Enabled {
		if domain == "" {
			return errors.New("请填写域名")
		}
		if strings.Contains(domain, " ") || strings.Contains(domain, "http") {
			return errors.New("域名格式不正确，请只填写如 oci.example.com")
		}
		if strings.TrimSpace(in.Email) == "" {
			return errors.New("请填写用于接收 Let's Encrypt 通知的邮箱")
		}
	}

	// Token 留空表示沿用已保存值。
	token := strings.TrimSpace(in.CFAPIToken)
	if token == "" {
		existing, err := m.store.GetSecretSetting(keyToken)
		if err != nil {
			return err
		}
		token = existing
	}
	if in.Enabled && token == "" {
		return errors.New("请填写 Cloudflare API Token（权限需要 Zone:DNS:Edit 与 Zone:Zone:Read）")
	}

	caDir := lego.LEDirectoryProduction
	if in.Staging {
		caDir = lego.LEDirectoryStaging
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	writes := map[string]string{
		keyDomain: strings.ToLower(strings.TrimSpace(in.Domain)),
		keyEmail:  strings.TrimSpace(in.Email),
		keyCADir:  caDir,
	}
	for k, v := range writes {
		if err := m.store.SetSetting(k, v); err != nil {
			return err
		}
	}
	if strings.TrimSpace(in.CFAPIToken) != "" {
		if err := m.store.SetSecretSetting(keyToken, token); err != nil {
			return err
		}
	}

	if !in.Enabled {
		_ = m.store.SetSetting(keyEnabled, "0")
		_ = m.store.SetSetting(keyLastError, "")
		return nil
	}

	if err := m.store.SetSetting(keyEnabled, "1"); err != nil {
		return err
	}

	cfg := ConfigInput{
		Enabled: true, Domain: domain, Email: strings.TrimSpace(in.Email),
		CFAPIToken: token, Staging: in.Staging,
	}
	err := m.obtain(cfg, caDir)
	if err != nil {
		_ = m.store.SetSetting(keyLastError, err.Error())
		return err
	}
	_ = m.store.SetSetting(keyLastError, "")
	return nil
}

// Disable 关闭 HTTPS，清除证书缓存。HTTP 服务始终保持可用，避免误操作把
// 自己锁在面板外面。
func (m *Manager) Disable() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.store.SetSetting(keyEnabled, "0"); err != nil {
		return err
	}
	m.current.Store(nil)
	m.domain.Store("")
	m.expires.Store(time.Time{})
	_ = os.Remove(filepath.Join(m.dir, certFile))
	_ = os.Remove(filepath.Join(m.dir, keyFile))
	return nil
}

// obtain 执行一次完整的 ACME 申请并写入证书文件。
func (m *Manager) obtain(cfg ConfigInput, caDir string) error {
	accountKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("生成 ACME 账户密钥失败: %w", err)
	}
	user := &acmeUser{Email: cfg.Email, key: accountKey}

	lc := lego.NewConfig(user)
	lc.CADirURL = caDir

	client, err := lego.NewClient(lc)
	if err != nil {
		return fmt.Errorf("初始化 ACME 客户端失败: %w", err)
	}

	cfCfg := cloudflare.NewDefaultConfig()
	cfCfg.AuthToken = cfg.CFAPIToken
	// Cloudflare/lego 要求 TTL 必须 >= 120（lego v4 校验 config.TTL < 120 即报错，
	// 0 也会触发），固定写小值（如 60）会在初始化提供商阶段直接失败。
	// 120 即 lego 的默认下限，对短生命周期的 _acme-challenge TXT 记录足够。
	cfCfg.TTL = 120
	cfCfg.PropagationTimeout = 3 * time.Minute
	cfCfg.PollingInterval = 10 * time.Second
	provider, err := cloudflare.NewDNSProviderConfig(cfCfg)
	if err != nil {
		return fmt.Errorf("初始化 Cloudflare DNS 提供商失败: %w", err)
	}

	// 国内 VPS 的本地 DNS 常无法解析刚写入的 TXT 记录，指定公共递归解析器
	// 可以让传播检查直接向权威/公共 DNS 查询，避免误判为未生效。
	err = client.Challenge.SetDNS01Provider(provider,
		dns01.AddDNSTimeout(3*time.Minute),
		dns01.AddRecursiveNameservers(dns01.ParseNameservers([]string{
			"1.1.1.1:53", "8.8.8.8:53", "9.9.9.9:53",
		})),
	)
	if err != nil {
		return fmt.Errorf("配置 DNS-01 挑战失败: %w", err)
	}

	reg, err := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
	if err != nil {
		return fmt.Errorf("注册 Let's Encrypt 账户失败: %w", err)
	}
	user.registration = reg

	certs, err := client.Certificate.Obtain(certificate.ObtainRequest{
		Domains: []string{cfg.Domain},
		Bundle:  true,
	})
	if err != nil {
		return fmt.Errorf("申请证书失败（请确认域名 DNS 托管在 Cloudflare、Token 具备 Zone:DNS:Edit 权限）: %w", err)
	}

	if err := os.MkdirAll(m.dir, 0o700); err != nil {
		return fmt.Errorf("创建证书目录失败: %w", err)
	}
	if err := os.WriteFile(filepath.Join(m.dir, certFile), certs.Certificate, 0o644); err != nil {
		return fmt.Errorf("写入证书失败: %w", err)
	}
	if err := os.WriteFile(filepath.Join(m.dir, keyFile), certs.PrivateKey, 0o600); err != nil {
		return fmt.Errorf("写入私钥失败: %w", err)
	}

	pair, err := m.readCert()
	if err != nil {
		return fmt.Errorf("回读证书失败: %w", err)
	}
	m.current.Store(pair)
	m.domain.Store(cfg.Domain)
	m.expires.Store(pair.Leaf.NotAfter)
	_ = m.store.SetSetting(keyExpiresAt, pair.Leaf.NotAfter.UTC().Format(time.RFC3339))
	log.Printf("HTTPS 证书申请成功：%s，有效期至 %s", cfg.Domain, pair.Leaf.NotAfter.Format("2006-01-02"))
	return nil
}

// StartRenewLoop 每天检查一次，剩余有效期不足 30 天时自动续期。
func (m *Manager) StartRenewLoop() {
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		// 启动后先立即检查一次，覆盖"重启时证书已快过期"的情况。
		m.renewIfNeeded()
		for range ticker.C {
			m.renewIfNeeded()
		}
	}()
}

func (m *Manager) renewIfNeeded() {
	m.mu.Lock()
	defer m.mu.Unlock()

	cert := m.current.Load()
	if cert == nil || cert.Leaf == nil {
		return
	}
	cfg, err := m.savedConfig()
	if err != nil || !cfg.Enabled || cfg.Domain == "" || cfg.CFAPIToken == "" {
		return
	}
	if time.Until(cert.Leaf.NotAfter) > renewBefore {
		return
	}

	caDir := lego.LEDirectoryProduction
	if cfg.Staging {
		caDir = lego.LEDirectoryStaging
	}
	log.Printf("证书 %s 即将到期，开始自动续期", cfg.Domain)
	if err := m.obtain(cfg, caDir); err != nil {
		log.Printf("自动续期失败（将在下次检查时重试）: %v", err)
		_ = m.store.SetSetting(keyLastError, "自动续期失败："+err.Error())
	} else {
		_ = m.store.SetSetting(keyLastError, "")
	}
}

// acmeUser 实现 lego 的 registration.User 接口。
type acmeUser struct {
	Email        string
	registration *registration.Resource
	key          *rsa.PrivateKey
}

func (u *acmeUser) GetEmail() string                        { return u.Email }
func (u *acmeUser) GetRegistration() *registration.Resource { return u.registration }
func (u *acmeUser) GetPrivateKey() crypto.PrivateKey        { return u.key }
