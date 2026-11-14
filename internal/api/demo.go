package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/yourname/oci-manager/internal/oci"
)

// iamCredentialTTL 是高权限 IAM 凭证在内存中的驻留时长。
// 凭证绝不写库、绝不回显、绝不进日志；超时立即失效。
const iamCredentialTTL = 10 * time.Minute

type iamCredential struct {
	Creds       oci.Credentials
	TenancyOCID string
	HomeRegion  string
	ExpiresAt   time.Time
}

// iamVault 只保存一份「当前正在使用」的高权限凭证。
// 双钥匙设计：日常资源操作用库里的低权限凭证，IAM 高权限凭证临时导入、用完即焚。
type iamVault struct {
	mu   sync.Mutex
	cred *iamCredential
}

func newIAMVault() *iamVault { return &iamVault{} }

func (v *iamVault) put(creds oci.Credentials, tenancy, homeRegion string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.cred = &iamCredential{
		Creds:       creds,
		TenancyOCID: tenancy,
		HomeRegion:  homeRegion,
		ExpiresAt:   time.Now().Add(iamCredentialTTL),
	}
}

// get 校验凭证未过期且租户匹配，避免把 A 租户的高权限凭证用到 B 租户上。
func (v *iamVault) get(tenancy string) (*iamCredential, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.cred == nil || !time.Now().Before(v.cred.ExpiresAt) {
		v.cred = nil
		return nil, errors.New("IAM 高权限凭证未导入或已过期，请在「设置 → IAM」重新导入")
	}
	if tenancy != "" && v.cred.TenancyOCID != tenancy {
		return nil, errors.New("导入的 IAM 凭证属于其他租户，请重新导入当前账号的凭证")
	}
	return v.cred, nil
}

func (v *iamVault) clear() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.cred = nil
}

func (v *iamVault) status() (bool, string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.cred == nil || !time.Now().Before(v.cred.ExpiresAt) {
		v.cred = nil
		return false, ""
	}
	return true, v.cred.ExpiresAt.UTC().Format(time.RFC3339)
}

/* ------------------------------------------------------------------
   通用 helper
   ------------------------------------------------------------------ */

type otpPayload struct {
	Challenge string `json:"challenge"`
	Code      string `json:"code"`
}

// verifyOperationOTP 校验并消费一次性操作验证码。demo 决定期望的挑战类型，
// 真实账号绝不可能用演示固定码通过。
func (s *Server) verifyOperationOTP(c *gin.Context, p otpPayload, operation string, demo bool) bool {
	if strings.TrimSpace(p.Challenge) == "" || strings.TrimSpace(p.Code) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先完成验证码确认"})
		return false
	}
	if err := s.auth.VerifyOperationOTP(p.Challenge, p.Code, operation, demo); err != nil {
		respondError(c, err)
		return false
	}
	return true
}

func (s *Server) recordAudit(c *gin.Context, accountID, category, operation, target, result, detail string) {
	s.store.AppendAudit(category, operation, target, result, detail, accountID, c.ClientIP())
}

func (s *Server) intSetting(key string, fallback int) int {
	raw, err := s.store.GetSetting(key)
	if err != nil || strings.TrimSpace(raw) == "" {
		return fallback
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 0 {
		return fallback
	}
	return n
}

func (s *Server) demoEnabled() bool {
	v, err := s.store.GetSetting("demo_enabled")
	if err != nil || v == "" {
		return true
	}
	return v != "false"
}

/* ------------------------------------------------------------------
   演示模式开关
   ------------------------------------------------------------------ */

func (s *Server) getDemo(c *gin.Context) {
	respondOK(c, gin.H{"enabled": s.demoEnabled()})
}

func (s *Server) saveDemo(c *gin.Context) {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, err)
		return
	}
	value := "true"
	if !body.Enabled {
		value = "false"
	}
	if err := s.store.SetSetting("demo_enabled", value); err != nil {
		respondError(c, err)
		return
	}
	s.recordAudit(c, "", "settings", "demo_mode", "", value, "切换演示模式开关")
	respondOK(c, gin.H{"enabled": body.Enabled})
}

/* ------------------------------------------------------------------
   通用操作验证码
   ------------------------------------------------------------------ */

func (s *Server) requestOTP(c *gin.Context) {
	if !s.rateLimit(c, "otp_request", 30, time.Minute) {
		return
	}
	var body struct {
		Operation string `json:"operation"`
		Summary   string `json:"summary"`
		Demo      bool   `json:"demo"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, err)
		return
	}
	// 只有系统仍开启演示模式且确实存在演示账号时才允许演示挑战。
	demo := body.Demo && s.demoEnabled()
	token, err := s.auth.RequestOperationOTP(body.Operation, body.Summary, demo)
	if err != nil {
		respondError(c, err)
		return
	}
	result := gin.H{"challenge": token}
	if demo {
		result["demo_code"] = "123456"
	}
	respondOK(c, result)
}

/* ------------------------------------------------------------------
   审计日志
   ------------------------------------------------------------------ */

func (s *Server) listAuditLogs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	logs, err := s.store.ListAuditLogs(limit)
	if err != nil {
		respondError(c, err)
		return
	}
	respondOK(c, logs)
}

/* ------------------------------------------------------------------
   换 IP 保护策略
   ------------------------------------------------------------------ */

const (
	defaultRotateCooldown = 300
	defaultRotateDaily    = 5
)

func (s *Server) rotationState() (cooldown, daily, used int, nextAllowed string) {
	cooldown = s.intSetting("ip_rotate_cooldown_sec", defaultRotateCooldown)
	daily = s.intSetting("ip_rotate_daily_limit", defaultRotateDaily)
	today := time.Now().UTC().Format("2006-01-02")
	used = s.intSetting("ip_rotate_count:"+today, 0)
	if last, err := s.store.GetSetting("ip_rotate_last"); err == nil && last != "" {
		if t, err := time.Parse(time.RFC3339, last); err == nil {
			next := t.Add(time.Duration(cooldown) * time.Second)
			if time.Now().Before(next) {
				nextAllowed = next.UTC().Format(time.RFC3339)
			}
		}
	}
	return
}

func (s *Server) rotationResponse() gin.H {
	cooldown, daily, used, next := s.rotationState()
	remaining := daily - used
	if remaining < 0 {
		remaining = 0
	}
	return gin.H{
		"cooldown_sec":    cooldown,
		"daily_limit":     daily,
		"used_today":      used,
		"remaining_today": remaining,
		"next_allowed_at": next,
	}
}

func (s *Server) getIPRotation(c *gin.Context) {
	respondOK(c, s.rotationResponse())
}

func (s *Server) saveIPRotation(c *gin.Context) {
	var body struct {
		CooldownSec int `json:"cooldown_sec"`
		DailyLimit  int `json:"daily_limit"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, err)
		return
	}
	if body.CooldownSec < 60 || body.CooldownSec > 86400 {
		respondError(c, errors.New("冷却时间需在 60–86400 秒之间"))
		return
	}
	if body.DailyLimit < 1 || body.DailyLimit > 50 {
		respondError(c, errors.New("每日上限需在 1–50 次之间"))
		return
	}
	if err := s.store.SetSetting("ip_rotate_cooldown_sec", strconv.Itoa(body.CooldownSec)); err != nil {
		respondError(c, err)
		return
	}
	if err := s.store.SetSetting("ip_rotate_daily_limit", strconv.Itoa(body.DailyLimit)); err != nil {
		respondError(c, err)
		return
	}
	s.recordAudit(c, "", "settings", "ip_rotation_policy", "", "updated",
		"冷却 "+strconv.Itoa(body.CooldownSec)+"s / 每日 "+strconv.Itoa(body.DailyLimit)+" 次")
	respondOK(c, s.rotationResponse())
}
