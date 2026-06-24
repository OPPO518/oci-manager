package service

import (
	"crypto/sha256"
	"encoding/hex"
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

// hashPassword 密码哈希生成器 (防止数据库泄露导致明文密码丢失)
func hashPassword(password string) string {
	hash := sha256.Sum256([]byte(password + "oci_go_salt_2026")) // 加盐哈希
	return hex.EncodeToString(hash[:])
}
