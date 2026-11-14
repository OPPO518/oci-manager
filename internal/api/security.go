package api

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// ipRateLimiter 是按 key 计数的固定窗口限流器，用于保护公开接口。
type ipRateLimiter struct {
	mu      sync.Mutex
	entries map[string]*rateEntry
}

type rateEntry struct {
	count int
	reset time.Time
}

func newIPRateLimiter() *ipRateLimiter {
	return &ipRateLimiter{entries: make(map[string]*rateEntry)}
}

func (l *ipRateLimiter) allow(key string, limit int, window time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	// 防止伪造大量随机 key 撑爆内存。
	if len(l.entries) > 4096 {
		for k, e := range l.entries {
			if now.After(e.reset) {
				delete(l.entries, k)
			}
		}
		if len(l.entries) > 4096 {
			l.entries = make(map[string]*rateEntry)
		}
	}
	e, ok := l.entries[key]
	if !ok || now.After(e.reset) {
		l.entries[key] = &rateEntry{count: 1, reset: now.Add(window)}
		return true
	}
	if e.count >= limit {
		return false
	}
	e.count++
	return true
}

// rateLimit 对公开接口做来源 IP 级限流。
// ClientIP 已被 SetTrustedProxies(nil) 固定为 TCP 对端地址，
// 不会被伪造的 X-Forwarded-For 绕过。
func (s *Server) rateLimit(c *gin.Context, scope string, limit int, window time.Duration) bool {
	if s.limiter.allow(scope+":"+c.ClientIP(), limit, window) {
		return true
	}
	c.JSON(http.StatusTooManyRequests, gin.H{"error": "请求过于频繁，请稍后再试"})
	c.Abort()
	return false
}

// securityHeaders 补齐纵深防御响应头。前端本身没有 dangerouslySetInnerHTML，
// 但这些头能在注入/点击劫持/降级场景下兜底。
func (s *Server) securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		h.Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data:; font-src 'self' data:; connect-src 'self'; "+
				"frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		// 证书已签发时，即便本次是经反代进来的明文请求也声明 HSTS。
		// 浏览器只会接受安全连接上的 HSTS，因此在纯 HTTP 上是无害的。
		if c.Request.TLS != nil || (s.tlsMgr != nil && s.tlsMgr.HasCertificate()) {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		c.Next()
	}
}

var (
	reURLUserinfo   = regexp.MustCompile(`(https?://)[^/@\s]+@`)
	reBotToken      = regexp.MustCompile(`(/bot)[^/\s]+`)
	reSensitivePath = regexp.MustCompile(`(?:[A-Za-z]:[\\/]|/)[^\s"',，)）]*\.(?:db|pem|key|sqlite)`)
)

// sanitizeError 脱敏对外错误信息。
// Telegram 的 *url.Error 会把 Bot Token 拼在 URL 里回显；代理地址可能带
// user:pass@；SQLite 错误可能带绝对路径。这些都不该出现在浏览器或日志里。
func sanitizeError(msg string) string {
	msg = reURLUserinfo.ReplaceAllString(msg, "${1}***@")
	msg = reBotToken.ReplaceAllString(msg, "${1}***")
	msg = reSensitivePath.ReplaceAllString(msg, "[路径已隐藏]")
	return msg
}

// redactProxyURL 只保留代理的 host:port，隐藏用户名密码与查询参数。
func redactProxyURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "[代理地址已隐藏]"
	}
	if u.User != nil {
		u.User = url.User("***")
	}
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimSpace(u.String())
}
