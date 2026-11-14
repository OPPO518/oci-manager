// Package api 提供管理器的 HTTP 接口与前端静态资源服务。
package api

import (
	"errors"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/yourname/oci-manager/internal/auth"
	"github.com/yourname/oci-manager/internal/config"
	"github.com/yourname/oci-manager/internal/grabber"
	"github.com/yourname/oci-manager/internal/notify"
	"github.com/yourname/oci-manager/internal/oci"
	"github.com/yourname/oci-manager/internal/store"
	"github.com/yourname/oci-manager/internal/tlsmanager"
	"github.com/yourname/oci-manager/internal/web"
)

// Server 聚合了接口层需要的全部依赖。
type Server struct {
	cfg      *config.Config
	store    *store.Store
	pool     *oci.Pool
	grabber  *grabber.Manager
	auth     *auth.Service
	notify   *notify.Service
	tlsMgr   *tlsmanager.Manager
	engine   *gin.Engine
	staticFS http.FileSystem
	limiter  *ipRateLimiter
	iamVault *iamVault
}

// New 构造 HTTP 服务。
func New(
	cfg *config.Config,
	st *store.Store,
	pool *oci.Pool,
	gm *grabber.Manager,
	authSvc *auth.Service,
	notifySvc *notify.Service,
	tlsMgr *tlsmanager.Manager,
) (*Server, error) {
	gin.SetMode(gin.ReleaseMode)

	s := &Server{
		cfg: cfg, store: st, pool: pool, grabber: gm,
		auth: authSvc, notify: notifySvc, tlsMgr: tlsMgr,
		iamVault: newIAMVault(),
	}

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
	// 不信任任何 X-Forwarded-For：否则攻击者可伪造来源 IP，绕过登录锁定与限流。
	_ = s.engine.SetTrustedProxies(nil)
	s.engine.Use(gin.Logger(), gin.Recovery(), s.securityHeaders())
	s.limiter = newIPRateLimiter()
	s.registerRoutes()
	return s, nil
}

// Handler 返回可挂载的 http.Handler。
func (s *Server) Handler() http.Handler { return s.engine }

func (s *Server) registerRoutes() {
	// 公开接口：健康检查与鉴权相关
	public := s.engine.Group("/api")
	{
		public.GET("/health", s.health)
		// Docker healthcheck / wget --spider 发的是 HEAD，Gin 不会自动把 HEAD 映射到 GET，
		// 不单独注册就会落到 NoRoute 返回 404，容器被误标为 unhealthy。
		public.HEAD("/health", s.health)
		public.POST("/auth/setup", s.authSetup)
		public.POST("/auth/login", s.authLogin)
		public.POST("/auth/otp", s.authVerifyOTP)
	}
	// 状态查询单独注册在受保护组之外，因为未登录时也要能拿到初始化状态
	s.engine.GET("/api/auth/status", s.authStatus)

	// 其余接口需要有效会话
	api := s.engine.Group("/api", s.requireAuth)
	{
		api.POST("/auth/logout", s.authLogout)
		api.POST("/auth/password", s.authChangePassword)

		api.GET("/settings/telegram", s.telegramSettings)
		api.PUT("/settings/telegram", s.saveTelegramSettings)
		api.POST("/settings/telegram/test", s.testTelegram)
		api.GET("/settings/precheck", s.getPrecheck)
		api.PUT("/settings/precheck", s.savePrecheck)
		api.GET("/settings/tls", s.getTLS)
		api.PUT("/settings/tls", s.saveTLS)
		api.POST("/settings/tls/disable", s.disableTLS)

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
		api.POST("/accounts/:id/capacity", s.checkCapacity)

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

		// 演示模式、通用验证码、审计日志与保护策略
		api.GET("/settings/demo", s.getDemo)
		api.PUT("/settings/demo", s.saveDemo)
		api.POST("/otp/request", s.requestOTP)
		api.GET("/audit-logs", s.listAuditLogs)
		api.GET("/settings/ip-rotation", s.getIPRotation)
		api.PUT("/settings/ip-rotation", s.saveIPRotation)

		// IAM：双钥匙 + 三道闸门
		api.GET("/settings/iam", s.getIAMSettings)
		api.PUT("/settings/iam", s.saveIAMSettings)
		api.POST("/settings/iam/credential", s.setIAMCredential)
		api.DELETE("/settings/iam/credential", s.deleteIAMCredential)

		// 资源编辑与网络
		api.GET("/accounts/:id/instances/:instanceId/detail", s.instanceDetail)
		api.PUT("/accounts/:id/instances/:instanceId/display-name", s.renameInstance)
		api.GET("/accounts/:id/boot-volumes", s.listBootVolumes)
		api.PUT("/accounts/:id/boot-volumes/:volumeId", s.updateBootVolume)
		api.GET("/accounts/:id/vnics", s.listVnics)
		api.PUT("/accounts/:id/vnics/:vnicId", s.updateVnic)
		api.GET("/accounts/:id/security-lists", s.listSecurityLists)
		api.PUT("/accounts/:id/security-lists/:listId", s.updateSecurityList)
		api.GET("/accounts/:id/public-ips", s.listPublicIPs)
		api.POST("/accounts/:id/public-ips/rotate", s.rotatePublicIP)

		// 身份与权限（IAM）
		api.GET("/accounts/:id/iam/overview", s.iamOverview)
		api.POST("/accounts/:id/iam/users", s.iamCreateUser)
		api.PUT("/accounts/:id/iam/users/:userId", s.iamUpdateUser)
		api.DELETE("/accounts/:id/iam/users/:userId", s.iamDeleteUser)
		api.POST("/accounts/:id/iam/users/:userId/password", s.iamResetPassword)
		api.POST("/accounts/:id/iam/users/:userId/api-keys", s.iamCreateAPIKey)
		api.DELETE("/accounts/:id/iam/users/:userId/api-keys/:fingerprint", s.iamDeleteAPIKey)
		api.POST("/accounts/:id/iam/groups/:groupId/members", s.iamAddMember)
		api.DELETE("/accounts/:id/iam/groups/:groupId/members/:userId", s.iamRemoveMember)
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
	c.JSON(status, gin.H{"error": sanitizeError(err.Error())})
}

// isAuthError 粗略判断凭证类错误，便于前端给出明确提示。
func isAuthError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "authentication") ||
		strings.Contains(msg, "not authenticated") ||
		strings.Contains(msg, "invalid") && strings.Contains(msg, "key") ||
		strings.Contains(msg, "401")
}
