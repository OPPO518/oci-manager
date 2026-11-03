// Package api 提供管理器的 HTTP 接口与前端静态资源服务。
package api

import (
	"errors"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/yourname/oci-manager/internal/config"
	"github.com/yourname/oci-manager/internal/grabber"
	"github.com/yourname/oci-manager/internal/oci"
	"github.com/yourname/oci-manager/internal/store"
	"github.com/yourname/oci-manager/internal/web"
)

// Server 聚合了接口层需要的全部依赖。
type Server struct {
	cfg      *config.Config
	store    *store.Store
	pool     *oci.Pool
	grabber  *grabber.Manager
	engine   *gin.Engine
	staticFS http.FileSystem
}

// New 构造 HTTP 服务。
func New(cfg *config.Config, st *store.Store, pool *oci.Pool, gm *grabber.Manager) (*Server, error) {
	gin.SetMode(gin.ReleaseMode)

	s := &Server{cfg: cfg, store: st, pool: pool, grabber: gm}

	if cfg.WebDir != "" {
		s.staticFS = http.Dir(cfg.WebDir)
	} else {
		sub, err := fs.Sub(web.DistFS, "dist")
		if err != nil {
			return nil, err
		}
		s.staticFS = http.FS(sub)
	}

	s.engine = gin.New()
	s.engine.Use(gin.Logger(), gin.Recovery())
	s.registerRoutes()
	return s, nil
}

// Handler 返回可挂载的 http.Handler。
func (s *Server) Handler() http.Handler { return s.engine }

func (s *Server) registerRoutes() {
	api := s.engine.Group("/api")
	{
		api.GET("/health", s.health)

		api.GET("/accounts", s.listAccounts)
		api.POST("/accounts", s.createAccount)
		api.GET("/accounts/:id", s.getAccount)
		api.PUT("/accounts/:id", s.updateAccount)
		api.DELETE("/accounts/:id", s.deleteAccount)
		api.POST("/accounts/:id/test", s.testAccount)

		api.GET("/accounts/:id/regions", s.listRegions)
		api.GET("/accounts/:id/compartments", s.listCompartments)
		api.GET("/accounts/:id/availability-domains", s.listAvailabilityDomains)
		api.GET("/accounts/:id/subnets", s.listSubnets)
		api.GET("/accounts/:id/images", s.listImages)

		api.GET("/accounts/:id/instances", s.listInstances)
		api.POST("/accounts/:id/instances/:instanceId/action", s.instanceAction)
		api.PUT("/accounts/:id/instances/:instanceId/shape", s.updateInstanceShape)
		api.DELETE("/accounts/:id/instances/:instanceId", s.terminateInstance)

		api.GET("/accounts/:id/cost", s.queryCost)
		api.GET("/accounts/:id/overview", s.overview)

		api.GET("/grab-tasks", s.listGrabTasks)
		api.POST("/grab-tasks", s.createGrabTask)
		api.GET("/grab-tasks/:id", s.getGrabTask)
		api.DELETE("/grab-tasks/:id", s.deleteGrabTask)
		api.POST("/grab-tasks/:id/start", s.startGrabTask)
		api.POST("/grab-tasks/:id/stop", s.stopGrabTask)
		api.GET("/grab-tasks/:id/logs", s.grabTaskLogs)
	}

	// 其余路径交给前端单页应用
	s.engine.NoRoute(s.serveSPA)
}

func (s *Server) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// serveSPA 服务前端静态资源，未命中的路径回落到 index.html。
func (s *Server) serveSPA(c *gin.Context) {
	if strings.HasPrefix(c.Request.URL.Path, "/api") {
		c.JSON(http.StatusNotFound, gin.H{"error": "接口不存在"})
		return
	}
	fileServer := http.FileServer(s.staticFS)

	path := strings.TrimPrefix(c.Request.URL.Path, "/")
	if path != "" {
		if f, err := s.staticFS.Open(path); err == nil {
			_ = f.Close()
			fileServer.ServeHTTP(c.Writer, c.Request)
			return
		}
	}
	c.Request.URL.Path = "/"
	fileServer.ServeHTTP(c.Writer, c.Request)
}

// clients 取指定账号在指定区域下的客户端，并统一处理账号不存在的情况。
func (s *Server) clients(c *gin.Context, region string) (*oci.ClientSet, bool) {
	accountID := c.Param("id")
	cs, err := s.pool.For(accountID, region)
	if err != nil {
		respondError(c, err)
		return nil, false
	}
	return cs, true
}

func respondOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, data)
}

func respondCreated(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, data)
}

// respondError 把错误映射为合适的 HTTP 状态码。
func respondError(c *gin.Context, err error) {
	status := http.StatusBadRequest
	switch {
	case errors.Is(err, store.ErrNotFound):
		status = http.StatusNotFound
	case isAuthError(err):
		status = http.StatusUnauthorized
	default:
		status = http.StatusBadRequest
	}
	c.JSON(status, gin.H{"error": err.Error()})
}

// isAuthError 粗略判断凭证类错误，便于前端给出明确提示。
func isAuthError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "authentication") ||
		strings.Contains(msg, "not authenticated") ||
		strings.Contains(msg, "invalid") && strings.Contains(msg, "key") ||
		strings.Contains(msg, "401")
}
