package api

import (
	"github.com/gin-gonic/gin"

	"github.com/yourname/oci-manager/internal/notify"
)

// telegramSettings 返回 Telegram 配置。Bot Token 只回传是否已配置，不回传明文。
func (s *Server) telegramSettings(c *gin.Context) {
	cfg, err := s.notify.Config()
	if err != nil {
		respondError(c, err)
		return
	}
	respondOK(c, cfg)
}

// saveTelegramSettings 保存 Telegram 配置。
func (s *Server) saveTelegramSettings(c *gin.Context) {
	var body notify.ConfigInput
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, err)
		return
	}
	if err := s.notify.Save(body); err != nil {
		respondError(c, err)
		return
	}
	cfg, err := s.notify.Config()
	if err != nil {
		respondError(c, err)
		return
	}
	respondOK(c, cfg)
}

// testTelegram 发送一条测试消息。
func (s *Server) testTelegram(c *gin.Context) {
	if err := s.notify.Test(); err != nil {
		respondError(c, err)
		return
	}
	respondOK(c, gin.H{"status": "ok"})
}
