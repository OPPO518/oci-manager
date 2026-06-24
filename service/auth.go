package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// CheckSystemStatus 提供给前端路由守卫的探针接口
func CheckSystemStatus(c *gin.Context) {
	if !IsSystemInitialized() {
		// 返回 200，但明确告知业务状态为需要初始化
		c.JSON(http.StatusOK, gin.H{
			"status":  "init_required",
			"message": "System requires initial setup",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "login_required",
		"message": "System is ready for login",
	})
}

// SetupInitialAdmin 处理首次运行时的管理员账号设置
func SetupInitialAdmin(c *gin.Context) {
	if IsSystemInitialized() {
		c.JSON(http.StatusForbidden, gin.H{"error": "System is already initialized"})
		return
	}

	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	// 将密码进行 SHA256 哈希处理后存入 Settings 表
	hashedPassword := hashPassword(req.Password)
	_ = SetSetting("admin_username", req.Username)
	_ = SetSetting("admin_password_hash", hashedPassword)

	// 标记系统已完成初始化
	_ = SetSetting("sys_initialized", "true")

	c.JSON(http.StatusOK, gin.H{"message": "System initialized successfully"})
}

// --- 👇 恢复并升级的真实登录与鉴权逻辑 👇 ---

// VerifyLogin 校验登录，比对数据库中的哈希密码，并返回 Token
func VerifyLogin(username, password string) (string, error) {
	if !IsSystemInitialized() {
		return "", errors.New("系统尚未初始化，请先设置管理员账号")
	}

	// 从底层 SQLite 数据库中读取刚才配置的管理员账号密码
	savedUser, _ := GetSetting("admin_username")
	savedHash, _ := GetSetting("admin_password_hash")

	// 严密比对用户名与哈希值
	if username == savedUser && hashPassword(password) == savedHash {
		// MVP 阶段：签发一个固定的高强度标识符（未来可升级为真正的 JWT）
		return "oci-go-auth-token-v1", nil
	}

	return "", errors.New("用户名或密码错误")
}

// CheckToken 校验每次前端请求携带的 Token
func CheckToken(token string) bool {
	// 拦截非法请求
	return token == "oci-go-auth-token-v1"
}

// hashPassword 密码哈希生成器 (防止数据库泄露导致明文密码丢失)
func hashPassword(password string) string {
	hash := sha256.Sum256([]byte(password + "oci_go_salt_2026")) // 加盐哈希
	return hex.EncodeToString(hash[:])
}
