package service

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"

	"golang.org/x/crypto/bcrypt"
)

// ActiveTokens 在内存中记录当前有效的通行证 (重启服务器会失效，需要重新登录，非常适合个人安全使用)
var ActiveTokens = make(map[string]bool)

// VerifyLogin 验证用户名和密码，成功则返回一个随机生成的 Token
func VerifyLogin(username, password string) (string, error) {
	var hash string
	// 去数据库里把对应的“碎纸密码”捞出来
	err := DB.QueryRow("SELECT password_hash FROM admin_users WHERE username = ?", username).Scan(&hash)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", errors.New("账号或密码错误") // 故意不提示具体是哪个错，防黑客探测
		}
		return "", err
	}

	// 将用户输入的明文密码再次丢进碎纸机，和数据库里的做比对
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return "", errors.New("账号或密码错误")
	}

	// 校验通过！生成一个 32 位的随机安全字符串作为 Token
	bytes := make([]byte, 16)
	rand.Read(bytes)
	token := hex.EncodeToString(bytes)
	
	// 把通行证记录到系统里
	ActiveTokens[token] = true

	return token, nil
}

// CheckToken 检查通行证是否有效
func CheckToken(token string) bool {
	_, exists := ActiveTokens[token]
	return exists
}
