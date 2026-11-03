package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/yourname/oci-manager/internal/oci"
	"github.com/yourname/oci-manager/internal/store"
)

// accountRequest 是账号的写入模型。私钥只在提交时传入，读取接口永不返回。
type accountRequest struct {
	Name          string `json:"name"`
	TenancyOCID   string `json:"tenancy_ocid"`
	UserOCID      string `json:"user_ocid"`
	Fingerprint   string `json:"fingerprint"`
	Region        string `json:"region"`
	ProxyURL      string `json:"proxy_url"`
	PrivateKeyPEM string `json:"private_key_pem"`
	Passphrase    string `json:"passphrase"`
}

func (r accountRequest) toInput(keepExistingKey bool) store.AccountInput {
	return store.AccountInput{
		Name:            strings.TrimSpace(r.Name),
		TenancyOCID:     strings.TrimSpace(r.TenancyOCID),
		UserOCID:        strings.TrimSpace(r.UserOCID),
		Fingerprint:     strings.TrimSpace(r.Fingerprint),
		Region:          strings.TrimSpace(r.Region),
		ProxyURL:        strings.TrimSpace(r.ProxyURL),
		PrivateKeyPEM:   r.PrivateKeyPEM,
		Passphrase:      r.Passphrase,
		KeepExistingKey: keepExistingKey,
	}
}

func (s *Server) listAccounts(c *gin.Context) {
	accounts, err := s.store.ListAccounts()
	if err != nil {
		respondError(c, err)
		return
	}
	respondOK(c, accounts)
}

func (s *Server) getAccount(c *gin.Context) {
	account, err := s.store.GetAccount(c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	respondOK(c, account)
}

func (s *Server) createAccount(c *gin.Context) {
	var req accountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, err)
		return
	}
	account, err := s.store.CreateAccount(req.toInput(false))
	if err != nil {
		respondError(c, err)
		return
	}
	respondCreated(c, account)
}

func (s *Server) updateAccount(c *gin.Context) {
	var req accountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, err)
		return
	}
	account, err := s.store.UpdateAccount(c.Param("id"), req.toInput(true))
	if err != nil {
		respondError(c, err)
		return
	}
	// 凭证可能已变更，丢弃缓存的客户端
	s.pool.Invalidate(account.ID)
	respondOK(c, account)
}

func (s *Server) deleteAccount(c *gin.Context) {
	id := c.Param("id")
	if err := s.store.DeleteAccount(id); err != nil {
		respondError(c, err)
		return
	}
	s.pool.Invalidate(id)
	c.Status(http.StatusNoContent)
}

// testAccount 验证凭证与代理是否可用，返回租户名称与订阅区域数量。
func (s *Server) testAccount(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	start := time.Now()
	cs, ok := s.clients(c, "")
	if !ok {
		return
	}
	account, err := s.store.GetAccount(c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}

	name, err := oci.GetTenancyName(ctx, cs, account.TenancyOCID)
	if err != nil {
		respondError(c, err)
		return
	}
	regions, err := oci.ListRegions(ctx, cs, account.TenancyOCID)
	if err != nil {
		respondError(c, err)
		return
	}

	respondOK(c, gin.H{
		"tenancy_name":  name,
		"region":        account.Region,
		"proxy_url":     account.ProxyURL,
		"region_count":  len(regions),
		"latency_ms":    time.Since(start).Milliseconds(),
	})
}
