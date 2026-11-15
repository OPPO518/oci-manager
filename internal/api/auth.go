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

// sessionMaxAgeRemember 勾选「记住我」后的会话有效期（30 天）。
const sessionMaxAgeRemember = 30 * 24 * int(time.Hour/time.Second)

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

// sessionSecure 只依据服务端是否已签发证书判断，
// 不再信任客户端可伪造的 X-Forwarded-Proto。
// 一旦启用 HTTPS，Cookie 必须带 Secure，杜绝明文 HTTP 上被嗅探。
func (s *Server) sessionSecure() bool {
	return s.tlsMgr != nil && s.tlsMgr.HasCertificate()
}

// setSessionCookie 写入会话 Cookie。maxAge<=0 表示会话级（浏览器关闭即失效），
// 正数表示有效期秒数（勾选「记住我」时传 sessionMaxAgeRemember）。
func (s *Server) setSessionCookie(c *gin.Context, token string, maxAge int) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookie, token, maxAge, "/", "", s.sessionSecure(), true)
}

func (s *Server) clearSessionCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookie, "", -1, "/", "", s.sessionSecure(), true)
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
	// 初始化窗口是唯一的抢注点，严格限制尝试频率。
	if !s.rateLimit(c, "setup", 5, time.Minute) {
		return
	}
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
	if !s.rateLimit(c, "login", 10, time.Minute) {
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Remember bool   `json:"remember"`
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
		// 勾选「记住我」延长到 30 天，否则会话级（关闭浏览器即失效）。
		maxAge := 0
		if body.Remember {
			maxAge = sessionMaxAgeRemember
		}
		s.setSessionCookie(c, token, maxAge)
	}
	c.JSON(http.StatusOK, result)
}

// authVerifyOTP 校验 Telegram 验证码并发放会话。
func (s *Server) authVerifyOTP(c *gin.Context) {
	if !s.rateLimit(c, "otp", 20, time.Minute) {
		return
	}
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
	s.setSessionCookie(c, token, sessionMaxAge)
	respondOK(c, gin.H{"status": "ok", "username": username})
}

// authLogout 注销当前会话。
func (s *Server) authLogout(c *gin.Context) {
	if token, err := c.Cookie(sessionCookie); err == nil {
		s.auth.Logout(token)
	}
	s.clearSessionCookie(c)
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
	// ChangePassword 已撤销全部会话，给当前浏览器补发一张新会话，其它设备保持下线。
	username := body.Username
	if st, err := s.auth.Status(); err == nil && st.Username != "" {
		username = st.Username
	}
	token, err := s.auth.CreateSession(username)
	if err != nil {
		respondError(c, err)
		return
	}
	s.setSessionCookie(c, token, sessionMaxAge)
	respondOK(c, gin.H{"status": "ok"})
}
