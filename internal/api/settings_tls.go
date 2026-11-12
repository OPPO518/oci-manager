package api

import (
	"github.com/gin-gonic/gin"

	"github.com/yourname/oci-manager/internal/tlsmanager"
)

// getTLS 返回 HTTPS 配置状态（不含 Cloudflare Token 明文）。
func (s *Server) getTLS(c *gin.Context) {
	st, err := s.tlsMgr.Status()
	if err != nil {
		respondError(c, err)
		return
	}
	respondOK(c, st)
}

// saveTLS 保存 HTTPS 配置并申请证书。
// 申请过程同步执行，DNS 传播通常需要 1–3 分钟，前端需展示等待状态。
func (s *Server) saveTLS(c *gin.Context) {
	var body tlsmanager.ConfigInput
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, err)
		return
	}
	if err := s.tlsMgr.Apply(body); err != nil {
		respondError(c, err)
		return
	}
	st, err := s.tlsMgr.Status()
	if err != nil {
		respondError(c, err)
		return
	}
	respondOK(c, st)
}

// disableTLS 关闭 HTTPS，恢复纯 HTTP 访问。HTTP 监听始终保留，
// 保证误操作后仍能从面板里改回来。
func (s *Server) disableTLS(c *gin.Context) {
	if err := s.tlsMgr.Disable(); err != nil {
		respondError(c, err)
		return
	}
	st, err := s.tlsMgr.Status()
	if err != nil {
		respondError(c, err)
		return
	}
	respondOK(c, st)
}
