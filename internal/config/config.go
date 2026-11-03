// Package config 负责加载服务的运行配置。
// 全部配置均可通过环境变量覆盖，便于容器化部署。
package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
)

// Config 保存服务的全部运行时配置。
type Config struct {
	// Addr 是 HTTP 服务监听地址，例如 ":8080"。
	Addr string
	// DBPath 是 SQLite 数据库文件路径。
	DBPath string
	// EncryptionKey 用于加密数据库中存储的 API 私钥，必须是 32 字节。
	EncryptionKey []byte
	// WebDir 是前端静态资源目录。为空时使用内嵌资源。
	WebDir string
	// WarnDevelopmentKey 为 true 表示未设置加密密钥、正在使用默认开发密钥。
	WarnDevelopmentKey bool
}

const (
	envAddr          = "OCI_MANAGER_ADDR"
	envDBPath        = "OCI_MANAGER_DB"
	envEncryptionKey = "OCI_MANAGER_ENCRYPTION_KEY"
	envWebDir        = "OCI_MANAGER_WEB_DIR"
)

// Load 从环境变量读取配置，缺失项使用默认值。
// EncryptionKey 允许传入任意长度的字符串，内部会做 SHA-256 归一化到 32 字节；
// 未设置时使用固定的开发用默认值，并明确提示不要用于生产环境。
func Load() (*Config, error) {
	cfg := &Config{
		Addr:   envOrDefault(envAddr, ":8080"),
		DBPath: envOrDefault(envDBPath, "./data/oci-manager.db"),
		WebDir: os.Getenv(envWebDir),
	}

	raw := os.Getenv(envEncryptionKey)
	if raw == "" {
		raw = "oci-manager-development-key"
		cfg.WarnDevelopmentKey = true
	}
	sum := sha256.Sum256([]byte(raw))
	cfg.EncryptionKey = sum[:]

	return cfg, nil
}

// WarnDevelopmentKey 为 true 表示正在使用默认加密密钥。
var _ = fmt.Sprintf

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// KeyFingerprintHex 返回加密密钥的十六进制摘要，用于启动日志中的自检展示。
func (c *Config) KeyFingerprintHex() string {
	sum := sha256.Sum256(c.EncryptionKey)
	return hex.EncodeToString(sum[:8])
}
