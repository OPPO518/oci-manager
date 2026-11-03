package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// sessionCookie 是会话 Cookie 的名称。
const sessionCookie = "oci_session"

// sessionMaxAge 与 auth 包的会话有效期保持一致。
const sessionMaxAge = 7 * 24 * int(time.Hour/time.Second)

// requireAuth 校验会话 Cookie，未登录时返回 401。
func (s *Server) requireAuth(c *gin.Context) {
	token, err := c.Cookie(sessionCookie)
	if err != nil || token == "" {
		respondAuthError(c)
		return
	}
	username, ok := s.auth.Validate(token)
	if !ok {
		respondAuthError(c)
		return
	}
	c.Set("username", username)
	c.Next()
}

func respondAuthError(c *gin.Context) {
	c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录或会话已过期"})
	c.Abort()
}

// setSessionCookie 下发会话 Cookie。
// 仅当请求走 HTTPS 时加 Secure 标记，避免本地 http 调试时 Cookie 被丢弃。
func setSessionCookie(c *gin.Context, token string) {
	secure := c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookie, token, sessionMaxAge, "/", "", secure, true)
}

func clearSessionCookie(c *gin.Context) {
	secure := c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookie, "", -1, "/", "", secure, true)
}

// authStatus 返回当前鉴权状态，前端据此决定展示初始化页还是登录页。
func (s *Server) authStatus(c *gin.Context) {
	st, err := s.auth.Status()
	if err != nil {
		respondError(c, err)
		return
	}
	// 带上当前请求是否已登录，前端刷新页面时才能保持登录态
	if token, err := c.Cookie(sessionCookie); err == nil {
		if username, ok := s.auth.Validate(token); ok {
			st.Authenticated = true
			st.Username = username
		}
	}
	respondOK(c, st)
}

// authSetup 首次使用时设置管理员账号。
func (s *Server) authSetup(c *gin.Context) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, err)
		return
	}
	if err := s.auth.Setup(body.Username, body.Password); err != nil {
		respondError(c, err)
		return
	}
	respondOK(c, gin.H{"status": "ok"})
}

// authLogin 校验用户名密码。开启两步验证时返回 challenge 而不发放会话。
func (s *Server) authLogin(c *gin.Context) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, err)
		return
	}
	result, err := s.auth.Login(body.Username, body.Password, c.ClientIP())
	if err != nil {
		respondError(c, err)
		return
	}
	if !result.OTPRequired {
		token, err := s.auth.CreateSession(result.Username)
		if err != nil {
			respondError(c, err)
			return
		}
		setSessionCookie(c, token)
	}
	c.JSON(http.StatusOK, result)
}

// authVerifyOTP 校验 Telegram 验证码并发放会话。
func (s *Server) authVerifyOTP(c *gin.Context) {
	var body struct {
		Challenge string `json:"challenge"`
		Code      string `json:"code"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, err)
		return
	}
	token, username, err := s.auth.VerifyOTP(body.Challenge, body.Code, c.ClientIP())
	if err != nil {
		respondError(c, err)
		return
	}
	setSessionCookie(c, token)
	respondOK(c, gin.H{"status": "ok", "username": username})
}

// authLogout 注销当前会话。
func (s *Server) authLogout(c *gin.Context) {
	if token, err := c.Cookie(sessionCookie); err == nil {
		s.auth.Logout(token)
	}
	clearSessionCookie(c)
	respondOK(c, gin.H{"status": "ok"})
}

// authChangePassword 修改管理员密码。
func (s *Server) authChangePassword(c *gin.Context) {
	var body struct {
		Username    string `json:"username"`
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, err)
		return
	}
	if err := s.auth.ChangePassword(body.Username, body.OldPassword, body.NewPassword); err != nil {
		respondError(c, err)
		return
	}
	respondOK(c, gin.H{"status": "ok"})
}
