package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/yourname/oci-manager/internal/oci"
	"github.com/yourname/oci-manager/internal/store"
)

// resourceContext 取账号、客户端与区间，是资源类接口的统一入口。
func (s *Server) resourceContext(c *gin.Context) (*store.Account, *oci.ClientSet, string, bool) {
	account, cs, ok := s.accountAndClients(c)
	if !ok {
		return nil, nil, "", false
	}
	return account, cs, compartmentOf(c, account), true
}

func (s *Server) instanceDetail(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()
	_, cs, compartment, ok := s.resourceContext(c)
	if !ok {
		return
	}
	detail, err := oci.GetInstanceDetail(ctx, cs, compartment, c.Param("instanceId"))
	if err != nil {
		respondError(c, err)
		return
	}
	respondOK(c, detail)
}

func (s *Server) renameInstance(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()
	var body struct {
		DisplayName string `json:"display_name"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, err)
		return
	}
	account, cs, _, ok := s.resourceContext(c)
	if !ok {
		return
	}
	if err := oci.UpdateInstanceDisplayName(ctx, cs, c.Param("instanceId"), body.DisplayName); err != nil {
		respondError(c, err)
		return
	}
	s.recordAudit(c, account.ID, "compute", "rename_instance", c.Param("instanceId"), "ok", "新名称 "+body.DisplayName)
	respondOK(c, gin.H{"status": "ok"})
}

func (s *Server) listBootVolumes(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()
	_, cs, compartment, ok := s.resourceContext(c)
	if !ok {
		return
	}
	volumes, err := oci.ListBootVolumes(ctx, cs, compartment)
	if err != nil {
		respondError(c, err)
		return
	}
	respondOK(c, volumes)
}

func (s *Server) updateBootVolume(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()
	var body struct {
		DisplayName *string `json:"display_name"`
		SizeGBs     *int64  `json:"size_gbs"`
		VpusPerGB   *int64  `json:"vpus_per_gb"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, err)
		return
	}
	account, cs, _, ok := s.resourceContext(c)
	if !ok {
		return
	}
	if err := oci.UpdateBootVolume(ctx, cs, c.Param("volumeId"), oci.BootVolumeUpdate{
		DisplayName: body.DisplayName, SizeGBs: body.SizeGBs, VpusPerGB: body.VpusPerGB,
	}); err != nil {
		respondError(c, err)
		return
	}
	s.recordAudit(c, account.ID, "storage", "update_boot_volume", c.Param("volumeId"), "ok", "改名 / 容量 / VPU")
	respondOK(c, gin.H{"status": "ok"})
}

func (s *Server) listVnics(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()
	_, cs, compartment, ok := s.resourceContext(c)
	if !ok {
		return
	}
	vnics, err := oci.ListVnics(ctx, cs, compartment, c.Query("instance_id"))
	if err != nil {
		respondError(c, err)
		return
	}
	respondOK(c, vnics)
}

func (s *Server) updateVnic(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()
	var body struct {
		DisplayName   *string `json:"display_name"`
		HostnameLabel *string `json:"hostname_label"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, err)
		return
	}
	account, cs, _, ok := s.resourceContext(c)
	if !ok {
		return
	}
	if err := oci.UpdateVnic(ctx, cs, c.Param("vnicId"), oci.VnicUpdate{
		DisplayName: body.DisplayName, HostnameLabel: body.HostnameLabel,
	}); err != nil {
		respondError(c, err)
		return
	}
	s.recordAudit(c, account.ID, "network", "update_vnic", c.Param("vnicId"), "ok", "改名 / 主机名标签")
	respondOK(c, gin.H{"status": "ok"})
}

func (s *Server) listSecurityLists(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()
	_, cs, compartment, ok := s.resourceContext(c)
	if !ok {
		return
	}
	lists, err := oci.ListSecurityLists(ctx, cs, compartment, c.Query("vcn_id"))
	if err != nil {
		respondError(c, err)
		return
	}
	respondOK(c, lists)
}

func (s *Server) updateSecurityList(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()
	var body struct {
		IngressRules []oci.SecurityRule `json:"ingress_rules"`
		EgressRules  []oci.SecurityRule `json:"egress_rules"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, err)
		return
	}
	account, cs, _, ok := s.resourceContext(c)
	if !ok {
		return
	}
	if err := oci.UpdateSecurityList(ctx, cs, c.Param("listId"), body.IngressRules, body.EgressRules); err != nil {
		respondError(c, err)
		return
	}
	s.recordAudit(c, account.ID, "network", "update_security_list", c.Param("listId"), "ok",
		"入站 "+strconv.Itoa(len(body.IngressRules))+" 条 / 出站 "+strconv.Itoa(len(body.EgressRules))+" 条")
	respondOK(c, gin.H{"status": "ok"})
}

func (s *Server) listPublicIPs(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()
	_, cs, compartment, ok := s.resourceContext(c)
	if !ok {
		return
	}
	ips, err := oci.ListPublicIPs(ctx, cs, compartment)
	if err != nil {
		respondError(c, err)
		return
	}
	respondOK(c, ips)
}

// rotatePublicIP 更换公网 IP。护栏顺序：限流 → 冷却/每日上限 → 验证码 → 执行。
// 任何一步失败都不会发出 OCI 写请求。
func (s *Server) rotatePublicIP(c *gin.Context) {
	if !s.rateLimit(c, "rotate_public_ip", 10, time.Hour) {
		return
	}
	var body struct {
		PrivateIPID string     `json:"private_ip_id"`
		OTP         otpPayload `json:"otp"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, err)
		return
	}
	account, cs, compartment, ok := s.resourceContext(c)
	if !ok {
		return
	}
	cooldown, daily, used, nextAllowed := s.rotationState()
	// 演示账号不会发出真实 OCI 写请求，跳过冷却与次数限制以便完整试用；
	// 真实账号的护栏一步不少，且顺序在验证码之前。
	if !account.IsDemo {
		if nextAllowed != "" {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "换 IP 冷却中，请在 " + nextAllowed + " 之后重试（冷却 " + strconv.Itoa(cooldown) + " 秒）",
			})
			return
		}
		if used >= daily {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "今日换 IP 次数已用完（上限 " + strconv.Itoa(daily) + " 次），请明天再试",
			})
			return
		}
	}
	if !s.verifyOperationOTP(c, body.OTP, "rotate_public_ip", account.IsDemo) {
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 90*time.Second)
	defer cancel()
	newIP, err := oci.RotatePublicIP(ctx, cs, body.PrivateIPID, compartment)
	if err != nil {
		s.recordAudit(c, account.ID, "network", "rotate_public_ip", body.PrivateIPID, "failed", err.Error())
		respondError(c, err)
		return
	}

	today := time.Now().UTC().Format("2006-01-02")
	_ = s.store.SetSetting("ip_rotate_last", time.Now().UTC().Format(time.RFC3339))
	_ = s.store.SetSetting("ip_rotate_count:"+today, strconv.Itoa(used+1))
	s.recordAudit(c, account.ID, "network", "rotate_public_ip", body.PrivateIPID, "ok", "新地址 "+newIP.IPAddress)
	message := "已更换公网 IP。注意：Cloudflare / DNS 记录不会自动更新，请手动同步；" +
		"冷却 " + strconv.Itoa(cooldown) + " 秒后可再次更换，今日剩余 " + strconv.Itoa(daily-used-1) + " 次。"
	if account.IsDemo {
		message = "演示模式：已模拟更换公网 IP（未发出任何真实 OCI 请求）。真实账号会受冷却时间与每日次数上限保护，" +
			"且 Cloudflare / DNS 记录不会自动跟随变更。"
	}
	respondOK(c, gin.H{"status": "ok", "new_ip": newIP, "message": message})
}

// errIAMModuleDisabled 供 IAM 层复用。
var errIAMModuleDisabled = errors.New("IAM 管理模块未启用，请先在「设置 → IAM」中开启")
