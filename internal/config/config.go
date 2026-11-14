// Package config 负责加载服务的运行配置。
// 全部配置均可通过环境变量覆盖，便于容器化部署。
package config

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
)

// Config 保存服务的全部运行时配置。
type Config struct {
	// Addr 是 HTTP 服务监听地址，例如 ":8080"。
	Addr string
	// HTTPSAddr 是 HTTPS 服务监听地址。容器内使用高位端口（默认 :8443），
	// 由 Docker 映射到宿主机 443，避免非 root 用户无法绑定特权端口。
	HTTPSAddr string
	// DBPath 是 SQLite 数据库文件路径。
	DBPath string
	// EncryptionKey 用于加密数据库中存储的 API 私钥，必须是 32 字节。
	EncryptionKey []byte
	// WebDir 是前端静态资源目录。为空时使用内嵌资源。
	WebDir string
	// DisablePlainHTTP 为 true 时，明文 HTTP 的业务请求一律 403（仅 /api/health 放行）。
	// 由 install.sh 在「HTTPS 模式」下注入：只有脚本确实把宿主机端口切成只映射 443
	// 之后才禁用 HTTP，避免证书刚签发、443 尚未映射时把用户锁在门外。
	DisablePlainHTTP bool
	// AllowInsecureHTTP 为 true 时，即使处于 HTTPS 模式也放行明文 HTTP。
	// 仅用于排障或反代已终结 TLS 的场景；开启后登录密码、OTP 与 OCI 私钥
	// 会重新暴露在明文 HTTP 下。
	AllowInsecureHTTP bool
	// PublicHTTPSPort 保留兼容旧配置：曾用于构造 HTTP→HTTPS 跳转地址。
	// 证书启用后明文 HTTP 改为 403 拒绝（见 internal/api 的 rejectPlainHTTP），
	// 不再跳转，此值当前不参与判定，仅为兼容已有 .env 而保留读取。
	PublicHTTPSPort string
}

const (
	envAddr              = "OCI_MANAGER_ADDR"
	envHTTPSAddr         = "OCI_MANAGER_HTTPS_ADDR"
	envDBPath            = "OCI_MANAGER_DB"
	envEncryptionKey     = "OCI_MANAGER_ENCRYPTION_KEY"
	envWebDir            = "OCI_MANAGER_WEB_DIR"
	envAllowInsecureHTTP = "OCI_MANAGER_ALLOW_HTTP"
	envDisablePlainHTTP  = "OCI_MANAGER_HTTP_DISABLED"
	envPublicHTTPSPort   = "OCI_MANAGER_PUBLIC_HTTPS_PORT"
)

// Load 从环境变量读取配置。
//
// 加密密钥是唯一能解出数据库中 OCI 私钥的凭据，绝不能留空或回退到
// 写在源码里的默认值——那等于把私钥明文公开。缺失或过短时直接拒绝启动。
func Load() (*Config, error) {
	cfg := &Config{
		Addr:              envOrDefault(envAddr, ":8080"),
		HTTPSAddr:         envOrDefault(envHTTPSAddr, ":8443"),
		DBPath:            envOrDefault(envDBPath, "./data/oci-manager.db"),
		WebDir:            os.Getenv(envWebDir),
		AllowInsecureHTTP: os.Getenv(envAllowInsecureHTTP) == "true" || os.Getenv(envAllowInsecureHTTP) == "1",
		DisablePlainHTTP:  os.Getenv(envDisablePlainHTTP) == "true" || os.Getenv(envDisablePlainHTTP) == "1",
		PublicHTTPSPort:   envOrDefault(envPublicHTTPSPort, "443"),
	}

	raw := os.Getenv(envEncryptionKey)
	if raw == "" {
		return nil, errors.New("缺少 OCI_MANAGER_ENCRYPTION_KEY：请用 openssl rand -hex 32 生成随机密钥并写入 .env；该密钥用于加密 OCI API 私钥，不能留空")
	}
	if len(raw) < 32 {
		return nil, errors.New("OCI_MANAGER_ENCRYPTION_KEY 过短（至少 32 个字符），请用 openssl rand -hex 32 重新生成")
	}
	sum := sha256.Sum256([]byte(raw))
	cfg.EncryptionKey = sum[:]

	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// KeyFingerprintHex 返回加密密钥的十六进制摘要，用于启动日志中的自检展示。
// 只展示前 8 字节，不会泄露密钥本身。
func (c *Config) KeyFingerprintHex() string {
	sum := sha256.Sum256(c.EncryptionKey)
	return hex.EncodeToString(sum[:8])
}
