// Package auth 提供管理员账号、登录会话与两步验证能力。
// 首次访问需要设置用户名和密码；开启 Telegram 后登录还需输入动态验证码。
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math/big"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/yourname/oci-manager/internal/notify"
	"github.com/yourname/oci-manager/internal/store"
)

const (
	// sessionTTL 是会话有效期，最后一次访问后会续期。
	sessionTTL = 7 * 24 * time.Hour
	// otpTTL 是登录验证码的有效期。
	otpTTL = 5 * time.Minute
	// maxOTPAttempts 是单个验证码允许的错误次数。
	maxOTPAttempts = 5
	// maxCode 是验证码的取值上限（6 位数字）。
	maxCode = 1000000
	// 登录失败限流
	maxLoginFailures   = 10
	loginLockout       = 15 * time.Minute
	loginFailureWindow = 15 * time.Minute
)

// Status 描述当前鉴权状态，供前端决定展示设置页还是登录页。
type Status struct {
	// Initialized 为 false 表示还没有设置管理员账号，需要先完成初始化。
	Initialized bool `json:"initialized"`
	// Authenticated 表示当前请求的会话有效，由接口层根据 Cookie 填写。
	Authenticated bool   `json:"authenticated"`
	Username      string `json:"username"`
	// LoginOTP 为 true 表示登录需要 Telegram 验证码。
	LoginOTP bool `json:"login_otp"`
	// TelegramEnabled 为 true 表示已配置好 Telegram 通知。
	TelegramEnabled bool `json:"telegram_enabled"`
}

// LoginResult 是登录接口的返回。需要两步验证时只返回 challenge。
type LoginResult struct {
	OTPRequired bool   `json:"otp_required"`
	Challenge   string `json:"challenge"`
	Username    string `json:"username"`
}

// Service 提供鉴权相关操作。
type Service struct {
	store  *store.Store
	notify *notify.Service

	mu       sync.Mutex
	failures map[string]*failureRecord
}

type failureRecord struct {
	count       int
	lastFail    time.Time
	lockedUntil time.Time
}

// New 构造鉴权服务。
func New(st *store.Store, n *notify.Service) *Service {
	return &Service{store: st, notify: n, failures: map[string]*failureRecord{}}
}

// Status 查询当前鉴权状态。
func (s *Service) Status() (Status, error) {
	admin, exists, err := s.store.GetAdmin()
	if err != nil {
		return Status{}, err
	}
	st := Status{}
	if exists {
		st.Initialized = true
		st.Username = admin.Username
	}
	if s.notify != nil {
		st.LoginOTP = s.notify.LoginOTPEnabled()
		st.TelegramEnabled = s.notify.Enabled()
	}
	return st, nil
}

// Setup 首次使用时设置管理员账号。已初始化过则拒绝。
func (s *Service) Setup(username, password string) error {
	username = normalizeUsername(username)
	if err := validateCredentials(username, password); err != nil {
		return err
	}
	_, exists, err := s.store.GetAdmin()
	if err != nil {
		return err
	}
	if exists {
		return errors.New("管理员账号已设置，如需修改请使用修改密码功能")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("生成密码哈希失败: %w", err)
	}
	if err := s.store.CreateAdmin(username, string(hash)); err != nil {
		return err
	}
	log.Printf("管理员账号已创建：%s", username)
	return nil
}

// Login 校验用户名密码。开启两步验证时创建挑战并推送验证码，不直接发放会话。
func (s *Service) Login(username, password, clientIP string) (LoginResult, error) {
	username = normalizeUsername(username)
	if locked := s.checkLocked(username, clientIP); locked {
		return LoginResult{}, errors.New("登录失败次数过多，请 15 分钟后重试")
	}

	admin, exists, err := s.store.GetAdmin()
	if err != nil {
		return LoginResult{}, err
	}
	if !exists {
		return LoginResult{}, errors.New("尚未设置管理员账号")
	}
	if admin.Username != username {
		s.recordFailure(username, clientIP)
		return LoginResult{}, errors.New("用户名或密码错误")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(password)); err != nil {
		s.recordFailure(username, clientIP)
		return LoginResult{}, errors.New("用户名或密码错误")
	}
	s.clearFailures(username, clientIP)

	// 未开启两步验证：直接发放会话
	if s.notify == nil || !s.notify.LoginOTPEnabled() {
		s.notifyLogin(username, clientIP, false)
		return LoginResult{Username: username}, nil
	}

	code, err := randomCode()
	if err != nil {
		return LoginResult{}, err
	}
	token, err := randomToken()
	if err != nil {
		return LoginResult{}, err
	}
	codeHash, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
	if err != nil {
		return LoginResult{}, err
	}
	if err := s.store.CreateChallenge(token, username, string(codeHash), otpTTL); err != nil {
		return LoginResult{}, err
	}

	// 验证码发送失败不能静默放行，否则等于绕过了两步验证
	if err := s.notify.Send(fmt.Sprintf(
		"🔐 OCI 管理器登录验证码\n\n验证码：%s\n有效期：%d 分钟\n来源 IP：%s\n\n如果不是本人操作，请立即修改密码。",
		code, int(otpTTL.Minutes()), displayIP(clientIP),
	)); err != nil {
		_ = s.store.DeleteChallenge(token)
		return LoginResult{}, fmt.Errorf("发送登录验证码失败: %w", err)
	}

	return LoginResult{OTPRequired: true, Challenge: token, Username: username}, nil
}

// VerifyOTP 校验验证码并发放会话。
func (s *Service) VerifyOTP(challenge, code, clientIP string) (string, string, error) {
	ch, exists, err := s.store.GetChallenge(challenge)
	if err != nil {
		return "", "", err
	}
	if !exists {
		return "", "", errors.New("验证已失效，请重新登录")
	}
	if ch.Attempts >= maxOTPAttempts {
		_ = s.store.DeleteChallenge(challenge)
		return "", "", errors.New("验证码错误次数过多，请重新登录")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(ch.CodeHash), []byte(code)); err != nil {
		_ = s.store.BumpChallengeAttempts(challenge)
		return "", "", errors.New("验证码不正确")
	}
	// 原子消费挑战：并发请求里只有一个能删到这条记录，杜绝一个验证码换多张会话。
	consumed, err := s.store.ConsumeChallenge(challenge)
	if err != nil {
		return "", "", err
	}
	if !consumed {
		return "", "", errors.New("验证已失效，请重新登录")
	}

	token, err := randomToken()
	if err != nil {
		return "", "", err
	}
	if err := s.store.CreateSession(token, ch.Username, sessionTTL); err != nil {
		return "", "", err
	}
	s.notifyLogin(ch.Username, clientIP, true)
	return token, ch.Username, nil
}

// CreateSession 直接发放会话（内部使用，例如两步验证未开启时）。
func (s *Service) CreateSession(username string) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	if err := s.store.CreateSession(token, username, sessionTTL); err != nil {
		return "", err
	}
	return token, nil
}

// Validate 校验会话令牌，有效时返回用户名并续期。
func (s *Service) Validate(token string) (string, bool) {
	if token == "" {
		return "", false
	}
	sess, exists, err := s.store.GetSession(token)
	if err != nil || !exists {
		return "", false
	}
	_ = s.store.TouchSession(token, sessionTTL)
	return sess.Username, true
}

// Logout 注销会话。
func (s *Service) Logout(token string) {
	_ = s.store.DeleteSession(token)
}

// ChangePassword 修改管理员密码。
func (s *Service) ChangePassword(username, oldPassword, newPassword string) error {
	admin, exists, err := s.store.GetAdmin()
	if err != nil {
		return err
	}
	if !exists {
		return errors.New("尚未设置管理员账号")
	}
	if admin.Username != normalizeUsername(username) {
		return errors.New("用户名不正确")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(oldPassword)); err != nil {
		return errors.New("当前密码不正确")
	}
	if err := validateCredentials(admin.Username, newPassword); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := s.store.UpdateAdminPassword(string(hash)); err != nil {
		return err
	}
	// 改密码后立即撤销所有既有会话：否则已泄露的旧 Cookie 还能继续读取账号与私钥。
	if err := s.store.DeleteAllSessions(); err != nil {
		return fmt.Errorf("密码已更新，但撤销旧会话失败: %w", err)
	}
	log.Printf("管理员密码已修改，已撤销全部既有会话")
	return nil
}

// Cleanup 定期清理过期会话与挑战，返回是否出错。
func (s *Service) Cleanup() error {
	if err := s.store.DeleteExpiredSessions(); err != nil {
		return err
	}
	return s.store.DeleteExpiredChallenges()
}

// notifyLogin 推送登录提醒，失败只记日志，不影响登录流程。
func (s *Service) notifyLogin(username, clientIP string, withOTP bool) {
	if s.notify == nil || !s.notify.Enabled() {
		return
	}
	extra := ""
	if withOTP {
		extra = "（已通过 Telegram 验证码）"
	}
	msg := fmt.Sprintf(
		"✅ OCI 管理器登录成功\n\n账号：%s\n来源 IP：%s\n时间：%s%s",
		username, displayIP(clientIP), time.Now().Format("2006-01-02 15:04:05"), extra,
	)
	if err := s.notify.Send(msg); err != nil {
		log.Printf("推送登录提醒失败: %v", err)
	}
}

// failureKeys 同时按用户名和来源 IP 计数：只按用户名会让分布式慢速爆破
// 绕过锁定，只按 IP 又挡不住同一 IP 换用户名试探。
func failureKeys(username, clientIP string) []string {
	keys := []string{"u:" + username}
	if clientIP != "" {
		keys = append(keys, "ip:"+clientIP)
	}
	return keys
}

func (s *Service) checkLocked(username, clientIP string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, key := range failureKeys(username, clientIP) {
		if rec, ok := s.failures[key]; ok && time.Now().Before(rec.lockedUntil) {
			return true
		}
	}
	return false
}

func (s *Service) recordFailure(username, clientIP string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 随机用户名刷接口会无限增长内存，超过上限先清理过期项，仍超限就整体重置。
	if len(s.failures) > 4096 {
		now := time.Now()
		for k, rec := range s.failures {
			if now.Sub(rec.lastFail) > loginFailureWindow && now.After(rec.lockedUntil) {
				delete(s.failures, k)
			}
		}
		if len(s.failures) > 4096 {
			s.failures = map[string]*failureRecord{}
		}
	}
	for _, key := range failureKeys(username, clientIP) {
		rec, ok := s.failures[key]
		if !ok || time.Since(rec.lastFail) > loginFailureWindow {
			rec = &failureRecord{}
			s.failures[key] = rec
		}
		rec.count++
		rec.lastFail = time.Now()
		if rec.count >= maxLoginFailures {
			rec.lockedUntil = time.Now().Add(loginLockout)
			log.Printf("登录失败次数过多，已临时锁定 %s", key)
		}
	}
}

func (s *Service) clearFailures(username, clientIP string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, key := range failureKeys(username, clientIP) {
		delete(s.failures, key)
	}
}

func validateCredentials(username, password string) error {
	if len(username) < 3 || len(username) > 32 {
		return errors.New("用户名长度需为 3–32 个字符")
	}
	if len(password) < 8 {
		return errors.New("密码至少 8 位")
	}
	if len(password) > 128 {
		return errors.New("密码过长")
	}
	return nil
}

func normalizeUsername(v string) string {
	return trimSpace(v)
}

func trimSpace(v string) string {
	start, end := 0, len(v)
	for start < end && (v[start] == ' ' || v[start] == '\t' || v[start] == '\n' || v[start] == '\r') {
		start++
	}
	for end > start && (v[end-1] == ' ' || v[end-1] == '\t' || v[end-1] == '\n' || v[end-1] == '\r') {
		end--
	}
	return v[start:end]
}

func displayIP(ip string) string {
	if ip == "" {
		return "未知"
	}
	return ip
}

// randomToken 生成 32 字节随机令牌的十六进制串。
func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	sum := sha256.Sum256(buf)
	return hex.EncodeToString(sum[:]), nil
}

// randomCode 生成 6 位数字验证码。
func randomCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(maxCode))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}
