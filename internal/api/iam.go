package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/yourname/oci-manager/internal/oci"
	"github.com/yourname/oci-manager/internal/store"
)

const iamRequestTimeout = 60 * time.Second

func (s *Server) iamEnabled() bool {
	v, err := s.store.GetSetting("iam_enabled")
	return err == nil && v == "true"
}

func (s *Server) iamMode() string {
	v, err := s.store.GetSetting("iam_mode")
	if err != nil || v == "" {
		return "classic"
	}
	if v == "domains" {
		return "domains"
	}
	return "classic"
}

func (s *Server) iamHomeRegion() string {
	v, _ := s.store.GetSetting("iam_home_region")
	return strings.TrimSpace(v)
}

func (s *Server) iamSettingsResponse() gin.H {
	has, expires := s.iamVault.status()
	telegramReady := s.notify != nil && s.notify.Enabled()
	return gin.H{
		"enabled":               s.iamEnabled(),
		"mode":                  s.iamMode(),
		"home_region":           s.iamHomeRegion(),
		"has_credential":        has,
		"credential_expires_at": expires,
		"telegram_ready":        telegramReady,
		"demo":                  s.demoEnabled(),
	}
}

func (s *Server) getIAMSettings(c *gin.Context) {
	respondOK(c, s.iamSettingsResponse())
}

func (s *Server) saveIAMSettings(c *gin.Context) {
	var body struct {
		Enabled    bool   `json:"enabled"`
		Mode       string `json:"mode"`
		HomeRegion string `json:"home_region"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, err)
		return
	}
	if body.Mode != "" && body.Mode != "classic" && body.Mode != "domains" {
		respondError(c, errors.New("IAM 模式只能是 classic 或 domains"))
		return
	}
	home := strings.TrimSpace(body.HomeRegion)
	if len(home) > 64 {
		respondError(c, errors.New("home region 过长"))
		return
	}
	value := "false"
	if body.Enabled {
		value = "true"
	}
	if err := s.store.SetSetting("iam_enabled", value); err != nil {
		respondError(c, err)
		return
	}
	if body.Mode != "" {
		if err := s.store.SetSetting("iam_mode", body.Mode); err != nil {
			respondError(c, err)
			return
		}
	}
	if err := s.store.SetSetting("iam_home_region", home); err != nil {
		respondError(c, err)
		return
	}
	if !body.Enabled {
		// 关闭模块时立即销毁内存中的高权限凭证。
		s.iamVault.clear()
	}
	s.recordAudit(c, "", "iam", "iam_settings", "", value,
		"模式 "+s.iamMode()+" / home region "+home)
	respondOK(c, s.iamSettingsResponse())
}

// setIAMCredential 临时导入高权限凭证。只驻留内存 10 分钟，绝不落库或回显。
func (s *Server) setIAMCredential(c *gin.Context) {
	if !s.rateLimit(c, "iam_credential", 10, time.Hour) {
		return
	}
	var body struct {
		TenancyOCID   string `json:"tenancy_ocid"`
		UserOCID      string `json:"user_ocid"`
		Fingerprint   string `json:"fingerprint"`
		PrivateKeyPEM string `json:"private_key_pem"`
		Passphrase    string `json:"passphrase"`
		Region        string `json:"region"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, err)
		return
	}
	tenancy := strings.TrimSpace(body.TenancyOCID)
	user := strings.TrimSpace(body.UserOCID)
	fp := strings.TrimSpace(body.Fingerprint)
	home := strings.TrimSpace(body.Region)
	pem := strings.TrimSpace(body.PrivateKeyPEM)
	if tenancy == "" || user == "" || fp == "" || pem == "" {
		respondError(c, errors.New("tenancy / user / fingerprint / 私钥均不能为空"))
		return
	}
	if !strings.Contains(pem, "PRIVATE KEY") {
		respondError(c, errors.New("请提供 PEM 格式的 RSA 私钥"))
		return
	}
	if len(pem) > 32768 {
		respondError(c, errors.New("私钥内容过长"))
		return
	}
	if home == "" {
		home = s.iamHomeRegion()
	}
	if home == "" {
		respondError(c, errors.New("请填写 home region（IAM API 只能打主区域端点）"))
		return
	}

	creds := oci.Credentials{
		TenancyOCID:   tenancy,
		UserOCID:      user,
		Fingerprint:   fp,
		PrivateKeyPEM: pem,
		Passphrase:    body.Passphrase,
	}
	s.iamVault.put(creds, tenancy, home)
	if err := s.store.SetSetting("iam_home_region", home); err != nil {
		respondError(c, err)
		return
	}
	// 审计详情不含私钥、指纹之外的任何敏感内容。
	s.recordAudit(c, "", "iam", "iam_credential_import", "", "ok", "临时凭证已导入内存（10 分钟后自动失效）")
	respondOK(c, s.iamSettingsResponse())
}

func (s *Server) deleteIAMCredential(c *gin.Context) {
	s.iamVault.clear()
	s.recordAudit(c, "", "iam", "iam_credential_clear", "", "ok", "已从内存清除高权限凭证")
	respondOK(c, gin.H{"status": "ok"})
}

// iamClients 构建 IAM 客户端。真实账号用临时高权限凭证 + 固定 home region，
// 演示账号直接返回内存模拟客户端。
func (s *Server) iamClients(c *gin.Context, account *store.Account) (*oci.ClientSet, bool) {
	home := s.iamHomeRegion()
	if account.IsDemo {
		return s.pool.DemoClientSet(home), true
	}
	cred, err := s.iamVault.get(account.TenancyOCID)
	if err != nil {
		respondError(c, err)
		return nil, false
	}
	if home == "" {
		home = cred.HomeRegion
	}
	cs, err := s.pool.IAMClientSet(account.ID, cred.Creds, home)
	if err != nil {
		respondError(c, err)
		return nil, false
	}
	return cs, true
}

func (s *Server) iamOverview(c *gin.Context) {
	account, err := s.store.GetAccount(c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	mode := s.iamMode()
	home := s.iamHomeRegion()
	result := oci.IAMOverview{
		Mode:             mode,
		HomeRegion:       home,
		Users:            []oci.IAMUser{},
		Groups:           []oci.IAMGroup{},
		Memberships:      []oci.IAMMembership{},
		APIKeys:          []oci.IAMApiKey{},
		Limits:           []oci.IAMLimit{},
		ProtectedUserIDs: []string{},
		ReadOnly:         true,
		Hint:             "",
	}

	switch {
	case !s.iamEnabled():
		if account.IsDemo {
			result.Hint = "IAM 管理模块默认关闭（保护真实账号）。演示账号无需导入任何凭证：点击本页或「设置 → IAM」的开关启用后，即可用固定验证码 123456 试用全部功能。"
		} else {
			result.Hint = "IAM 管理模块默认关闭。请在「设置 → IAM」中开启并临时导入高权限凭证后使用。"
		}
		respondOK(c, result)
		return
	case mode == "domains":
		result.Hint = "当前租户可能使用 Identity Domains（SCIM API），与经典 IAM 是两套接口。本版本仅支持经典 IAM，写操作已禁用以免产生不可预期的结果。"
		respondOK(c, result)
		return
	case !account.IsDemo:
		if _, err := s.iamVault.get(account.TenancyOCID); err != nil {
			result.Hint = err.Error()
			respondOK(c, result)
			return
		}
	}

	cs, ok := s.iamClients(c, account)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), iamRequestTimeout)
	defer cancel()
	data, err := oci.GetIAMOverview(ctx, cs, account.TenancyOCID, account.UserOCID, mode, home)
	if err != nil {
		respondError(c, err)
		return
	}
	result = data
	// 面板自用 API 用户必须出现在禁止清单里，避免把自己删掉或改到无法登录。
	result.ProtectedUserIDs = appendUnique(result.ProtectedUserIDs, account.UserOCID)
	if s.notify == nil || !s.notify.Enabled() {
		if account.IsDemo {
			result.Hint = "演示账号使用固定验证码 123456，可放心试用全部写操作。"
		} else {
			result.ReadOnly = true
			result.Hint = "写操作需要 Telegram 验证码二次确认，请先在「设置 → Telegram」启用通知。"
		}
	} else {
		result.ReadOnly = false
		result.Hint = "写操作需 Telegram 验证码确认；Administrators 组与面板自用 API 用户受服务端硬保护。"
	}
	respondOK(c, result)
}

func appendUnique(list []string, value string) []string {
	if value == "" {
		return list
	}
	for _, v := range list {
		if v == value {
			return list
		}
	}
	return append(list, value)
}

// iamWriteGuard 是所有 IAM 写操作的统一前置：限流 → 模块/模式检查 → 验证码 → 客户端。
func (s *Server) iamWriteGuard(c *gin.Context, operation string, otp otpPayload) (*store.Account, *oci.ClientSet, bool) {
	if !s.rateLimit(c, "iam_write", 30, time.Hour) {
		return nil, nil, false
	}
	// 删除用户与重置密码额外限流，避免批量破坏性操作。
	if operation == "iam_delete_user" || operation == "iam_reset_password" {
		if !s.rateLimit(c, "iam_destructive", 5, time.Hour) {
			return nil, nil, false
		}
	}
	account, err := s.store.GetAccount(c.Param("id"))
	if err != nil {
		respondError(c, err)
		return nil, nil, false
	}
	if !s.iamEnabled() {
		c.JSON(http.StatusForbidden, gin.H{"error": errIAMModuleDisabled.Error()})
		return nil, nil, false
	}
	if s.iamMode() == "domains" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Identity Domains（SCIM）暂不支持，请在设置中切回经典 IAM"})
		return nil, nil, false
	}
	if !s.verifyOperationOTP(c, otp, operation, account.IsDemo) {
		return nil, nil, false
	}
	cs, ok := s.iamClients(c, account)
	if !ok {
		return nil, nil, false
	}
	return account, cs, true
}

// iamAssertUserAllowed 服务端硬保护：禁止操作面板自用 API 用户。
func (s *Server) iamAssertUserAllowed(c *gin.Context, account *store.Account, userID string) bool {
	if userID == account.UserOCID {
		c.JSON(http.StatusForbidden, gin.H{"error": "禁止操作面板自用的 API 用户，否则会导致本管理器无法再调用 OCI"})
		s.recordAudit(c, account.ID, "iam", "guard_block", userID, "blocked", "尝试操作面板自用 API 用户")
		return false
	}
	return true
}

// iamAssertGroupAllowed 服务端硬保护：Administrators 组成员一律不可变更。
func (s *Server) iamAssertGroupAllowed(c *gin.Context, account *store.Account, cs *oci.ClientSet, groupID string) bool {
	ctx, cancel := context.WithTimeout(c.Request.Context(), iamRequestTimeout)
	defer cancel()
	name, err := oci.GetIAMGroupName(ctx, cs, groupID)
	if err != nil {
		respondError(c, err)
		return false
	}
	if strings.EqualFold(name, "Administrators") {
		c.JSON(http.StatusForbidden, gin.H{"error": "禁止变更 Administrators 组成员，这是最高危的提权路径"})
		s.recordAudit(c, account.ID, "iam", "guard_block", groupID, "blocked", "尝试变更 Administrators 组成员")
		return false
	}
	return true
}

/* ------------------------------------------------------------------
   IAM 写操作
   ------------------------------------------------------------------ */

func (s *Server) iamCreateUser(c *gin.Context) {
	var body struct {
		Name        string     `json:"name"`
		Description string     `json:"description"`
		Email       string     `json:"email"`
		OTP         otpPayload `json:"otp"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, err)
		return
	}
	account, cs, ok := s.iamWriteGuard(c, "iam_create_user", body.OTP)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), iamRequestTimeout)
	defer cancel()
	user, err := oci.CreateIAMUser(ctx, cs, account.TenancyOCID, oci.IAMUserInput{
		Name: body.Name, Description: body.Description, Email: body.Email,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	s.recordAudit(c, account.ID, "iam", "iam_create_user", user.ID, "ok", "创建用户 "+body.Name)
	respondCreated(c, gin.H{"user": user})
}

func (s *Server) iamUpdateUser(c *gin.Context) {
	var body struct {
		Description *string    `json:"description"`
		Email       *string    `json:"email"`
		Disabled    *bool      `json:"disabled"`
		OTP         otpPayload `json:"otp"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, err)
		return
	}
	userID := c.Param("userId")
	account, cs, ok := s.iamWriteGuard(c, "iam_update_user", body.OTP)
	if !ok {
		return
	}
	if !s.iamAssertUserAllowed(c, account, userID) {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), iamRequestTimeout)
	defer cancel()
	user, err := oci.UpdateIAMUser(ctx, cs, userID, oci.IAMUserUpdate{
		Description: body.Description, Email: body.Email, Disabled: body.Disabled,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	s.recordAudit(c, account.ID, "iam", "iam_update_user", userID, "ok", "更新用户资料")
	respondOK(c, gin.H{"user": user})
}

func (s *Server) iamDeleteUser(c *gin.Context) {
	var body struct {
		OTP otpPayload `json:"otp"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, err)
		return
	}
	userID := c.Param("userId")
	account, cs, ok := s.iamWriteGuard(c, "iam_delete_user", body.OTP)
	if !ok {
		return
	}
	if !s.iamAssertUserAllowed(c, account, userID) {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), iamRequestTimeout)
	defer cancel()
	if err := oci.DeleteIAMUser(ctx, cs, userID); err != nil {
		respondError(c, err)
		return
	}
	s.recordAudit(c, account.ID, "iam", "iam_delete_user", userID, "ok", "删除用户")
	respondOK(c, gin.H{"status": "ok"})
}

func (s *Server) iamResetPassword(c *gin.Context) {
	var body struct {
		OTP otpPayload `json:"otp"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, err)
		return
	}
	userID := c.Param("userId")
	account, cs, ok := s.iamWriteGuard(c, "iam_reset_password", body.OTP)
	if !ok {
		return
	}
	if !s.iamAssertUserAllowed(c, account, userID) {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), iamRequestTimeout)
	defer cancel()
	password, err := oci.ResetIAMConsolePassword(ctx, cs, userID)
	if err != nil {
		respondError(c, err)
		return
	}
	// 一次性密码只从本响应返回，绝不入库、不写日志、不进审计详情、不发 Telegram。
	s.recordAudit(c, account.ID, "iam", "iam_reset_password", userID, "ok", "已重置控制台密码（一次性密码未记录）")
	respondOK(c, gin.H{
		"password": password,
		"hint":     "此密码仅显示一次，关闭窗口后无法找回。请立即复制并安全保存。",
	})
}

func (s *Server) iamCreateAPIKey(c *gin.Context) {
	var body struct {
		PublicKeyPEM string     `json:"public_key_pem"`
		OTP          otpPayload `json:"otp"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, err)
		return
	}
	userID := c.Param("userId")
	account, cs, ok := s.iamWriteGuard(c, "iam_create_api_key", body.OTP)
	if !ok {
		return
	}
	if !s.iamAssertUserAllowed(c, account, userID) {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), iamRequestTimeout)
	defer cancel()
	key, err := oci.UploadIAMAPIKey(ctx, cs, userID, body.PublicKeyPEM)
	if err != nil {
		respondError(c, err)
		return
	}
	s.recordAudit(c, account.ID, "iam", "iam_create_api_key", userID, "ok", "上传 API 签名公钥")
	respondCreated(c, gin.H{"api_key": key})
}

func (s *Server) iamDeleteAPIKey(c *gin.Context) {
	var body struct {
		OTP otpPayload `json:"otp"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, err)
		return
	}
	userID, fingerprint := c.Param("userId"), c.Param("fingerprint")
	account, cs, ok := s.iamWriteGuard(c, "iam_delete_api_key", body.OTP)
	if !ok {
		return
	}
	if !s.iamAssertUserAllowed(c, account, userID) {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), iamRequestTimeout)
	defer cancel()
	if err := oci.DeleteIAMAPIKey(ctx, cs, userID, fingerprint); err != nil {
		respondError(c, err)
		return
	}
	s.recordAudit(c, account.ID, "iam", "iam_delete_api_key", userID, "ok", "删除 API Key "+fingerprint)
	respondOK(c, gin.H{"status": "ok"})
}

func (s *Server) iamAddMember(c *gin.Context) {
	var body struct {
		UserID string     `json:"user_id"`
		OTP    otpPayload `json:"otp"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, err)
		return
	}
	groupID := c.Param("groupId")
	account, cs, ok := s.iamWriteGuard(c, "iam_add_member", body.OTP)
	if !ok {
		return
	}
	if !s.iamAssertGroupAllowed(c, account, cs, groupID) {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), iamRequestTimeout)
	defer cancel()
	membership, err := oci.AddIAMGroupMember(ctx, cs, groupID, body.UserID)
	if err != nil {
		respondError(c, err)
		return
	}
	s.recordAudit(c, account.ID, "iam", "iam_add_member", groupID, "ok", "用户 "+body.UserID+" 加入组")
	respondCreated(c, gin.H{"membership": membership})
}

func (s *Server) iamRemoveMember(c *gin.Context) {
	var body struct {
		OTP otpPayload `json:"otp"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, err)
		return
	}
	groupID, userID := c.Param("groupId"), c.Param("userId")
	account, cs, ok := s.iamWriteGuard(c, "iam_remove_member", body.OTP)
	if !ok {
		return
	}
	if !s.iamAssertGroupAllowed(c, account, cs, groupID) {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), iamRequestTimeout)
	defer cancel()
	if err := oci.RemoveIAMGroupMember(ctx, cs, account.TenancyOCID, groupID, userID); err != nil {
		respondError(c, err)
		return
	}
	s.recordAudit(c, account.ID, "iam", "iam_remove_member", groupID, "ok", "用户 "+userID+" 移出组")
	respondOK(c, gin.H{"status": "ok"})
}
