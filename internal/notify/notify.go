// Package notify 负责把系统事件推送到 Telegram。
// 目前用于登录验证码、登录提醒与 ARM 抢购结果通知。
package notify

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// 设置项键名。bot_token 属于敏感信息，单独加密存储。
const (
	KeyEnabled   = "notify.telegram.enabled"
	KeyBotToken  = "notify.telegram.bot_token"
	KeyChatID    = "notify.telegram.chat_id"
	KeyLoginOTP  = "notify.telegram.login_otp"
	KeyEventGrab = "notify.telegram.event_grab"
)

// Config 是 Telegram 通知的运行时配置。
type Config struct {
	// Enabled 为 true 时才会真正发送。
	Enabled bool `json:"enabled"`
	// ChatID 是消息接收方，可以是用户 ID 或频道 @name。
	ChatID string `json:"chat_id"`
	// HasToken 表示是否已配置 Bot Token（接口永不返回明文）。
	HasToken bool `json:"has_token"`
	// LoginOTP 为 true 时，登录需要输入 Telegram 收到的验证码。
	LoginOTP bool `json:"login_otp"`
	// EventGrab 为 true 时，抢购任务状态变化会推送通知。
	EventGrab bool `json:"event_grab"`
}

// ConfigInput 是保存配置的入参。BotToken 为空表示保留原值。
type ConfigInput struct {
	Enabled   bool   `json:"enabled"`
	BotToken  string `json:"bot_token"`
	ChatID    string `json:"chat_id"`
	LoginOTP  bool   `json:"login_otp"`
	EventGrab bool   `json:"event_grab"`
}

// Store 是 notify 需要的持久化能力。
type Store interface {
	GetSetting(key string) (string, error)
	SetSetting(key, value string) error
	GetSecretSetting(key string) (string, error)
	SetSecretSetting(key, value string) error
}

// Service 封装 Telegram 推送。未配置或已禁用时所有发送都是空操作。
type Service struct {
	store Store
	http  *http.Client
}

// New 构造通知服务。
func New(st Store) *Service {
	return &Service{
		store: st,
		http:  &http.Client{Timeout: 15 * time.Second},
	}
}

// Config 读取当前配置。
func (s *Service) Config() (Config, error) {
	cfg := Config{}
	enabled, err := s.store.GetSetting(KeyEnabled)
	if err != nil {
		return cfg, err
	}
	cfg.Enabled = enabled == "1"
	cfg.ChatID, err = s.store.GetSetting(KeyChatID)
	if err != nil {
		return cfg, err
	}
	token, err := s.store.GetSecretSetting(KeyBotToken)
	if err != nil {
		return cfg, err
	}
	cfg.HasToken = token != ""
	cfg.LoginOTP, err = s.boolSetting(KeyLoginOTP, true)
	if err != nil {
		return cfg, err
	}
	cfg.EventGrab, err = s.boolSetting(KeyEventGrab, true)
	if err != nil {
		return cfg, err
	}
	return cfg, nil
}

// boolSetting 读取布尔设置项，未设置过时使用默认值。
func (s *Service) boolSetting(key string, fallback bool) (bool, error) {
	raw, err := s.store.GetSetting(key)
	if err != nil {
		return fallback, err
	}
	if raw == "" {
		return fallback, nil
	}
	return raw == "1", nil
}

// Save 保存配置。BotToken 为空时保留已保存的值。
func (s *Service) Save(in ConfigInput) error {
	if err := s.store.SetSetting(KeyEnabled, boolText(in.Enabled)); err != nil {
		return err
	}
	if err := s.store.SetSetting(KeyChatID, strings.TrimSpace(in.ChatID)); err != nil {
		return err
	}
	if err := s.store.SetSetting(KeyLoginOTP, boolText(in.LoginOTP)); err != nil {
		return err
	}
	if err := s.store.SetSetting(KeyEventGrab, boolText(in.EventGrab)); err != nil {
		return err
	}
	if strings.TrimSpace(in.BotToken) != "" {
		if err := s.store.SetSecretSetting(KeyBotToken, strings.TrimSpace(in.BotToken)); err != nil {
			return err
		}
	}
	return nil
}

// Enabled 返回通知是否可用（已启用且 token 与 chat id 齐备）。
func (s *Service) Enabled() bool {
	cfg, err := s.Config()
	if err != nil || !cfg.Enabled {
		return false
	}
	return cfg.HasToken && cfg.ChatID != ""
}

// LoginOTPEnabled 返回是否开启登录验证码。
func (s *Service) LoginOTPEnabled() bool {
	cfg, err := s.Config()
	if err != nil {
		return false
	}
	return cfg.Enabled && cfg.LoginOTP && cfg.HasToken && cfg.ChatID != ""
}

// GrabEventEnabled 返回是否推送抢购事件。
func (s *Service) GrabEventEnabled() bool {
	cfg, err := s.Config()
	if err != nil {
		return false
	}
	return cfg.Enabled && cfg.EventGrab && cfg.HasToken && cfg.ChatID != ""
}

// Send 向 Telegram 推送一条纯文本消息。未启用时直接返回 nil。
func (s *Service) Send(text string) error {
	cfg, err := s.Config()
	if err != nil {
		return err
	}
	if !cfg.Enabled || !cfg.HasToken || cfg.ChatID == "" {
		return errors.New("Telegram 通知未启用或未配置完整")
	}
	token, err := s.store.GetSecretSetting(KeyBotToken)
	if err != nil {
		return err
	}
	return s.send(token, cfg.ChatID, text)
}

// Test 发送一条测试消息，用于配置页面验证。
func (s *Service) Test() error {
	return s.Send("✅ OCI 管理器通知测试\n配置正确，后续登录验证码与抢购结果会发到这里。")
}

func (s *Service) send(token, chatID, text string) error {
	payload, err := json.Marshal(map[string]any{
		"chat_id":                  chatID,
		"text":                     text,
		"disable_web_page_preview": true,
	})
	if err != nil {
		return err
	}
	url := "https://api.telegram.org/bot" + token + "/sendMessage"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		// 解析失败时错误信息会带上完整 URL（含 Bot Token），必须先脱敏。
		return fmt.Errorf("Telegram 请求构造失败: %s", strings.ReplaceAll(err.Error(), token, "***"))
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := s.http.Do(req)
	if err != nil {
		// *url.Error 会把 https://api.telegram.org/bot<TOKEN>/sendMessage 原样带出来。
		// 这个错误会经登录接口/设置页回显并写进日志，绝不能透传原始 err。
		log.Printf("Telegram API 请求失败（已脱敏）: %s", strings.ReplaceAll(err.Error(), token, "***"))
		return fmt.Errorf("无法连接 Telegram API（网络错误）")
	}
	defer res.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("Telegram 返回 HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil && !parsed.OK {
		desc := parsed.Description
		if desc == "" {
			desc = "未知错误"
		}
		return fmt.Errorf("Telegram 拒绝了该请求: %s", desc)
	}
	return nil
}

func boolText(v bool) string {
	if v {
		return "1"
	}
	return "0"
}
