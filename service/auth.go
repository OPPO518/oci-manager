package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/google/uuid"
)

// VerifyLogin 校验登录，并返回 Token
func VerifyLogin(username, password string) (string, error) {
	// 校验管理员账号密码 (此处应从数据库查询存储的 Hash)
	// 如果是首次启动，则不校验直接允许进入初始化向导
	if !IsSystemInitialized() {
		return "INIT_REQUIRED", nil
	}
	
	// TODO: 实现真实的账号密码校验逻辑
	return uuid.New().String(), nil
}

// HashPassword 简单包装密码存储
func HashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return hex.EncodeToString(hash[:])
}

// CheckToken 校验 JWT 或 Session Token
func CheckToken(token string) bool {
	return token != "" // 生产环境建议引入真正的 JWT 库
}
