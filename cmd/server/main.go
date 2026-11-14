// Command server 是 OCI 个人账户管理器的服务入口。
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/yourname/oci-manager/internal/api"
	"github.com/yourname/oci-manager/internal/auth"
	"github.com/yourname/oci-manager/internal/config"
	"github.com/yourname/oci-manager/internal/grabber"
	"github.com/yourname/oci-manager/internal/notify"
	"github.com/yourname/oci-manager/internal/oci"
	"github.com/yourname/oci-manager/internal/secret"
	"github.com/yourname/oci-manager/internal/store"
	"github.com/yourname/oci-manager/internal/tlsmanager"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("启动失败: %v", err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if dir := filepath.Dir(cfg.DBPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("创建数据目录失败: %w", err)
		}
		_ = os.Chmod(dir, 0o700)
	}

	sealer, err := secret.NewSealer(cfg.EncryptionKey)
	if err != nil {
		return fmt.Errorf("初始化加密器失败: %w", err)
	}

	db, err := store.Open(cfg.DBPath, sealer)
	if err != nil {
		return err
	}
	defer db.Close()

	notifier := notify.New(db)
	pool := oci.NewPool(db)
	tasks := grabber.NewManager(db, pool, notifier)
	if err := tasks.RecoverOrphans(); err != nil {
		log.Printf("清理遗留任务失败: %v", err)
	}

	authSvc := auth.New(db, notifier)
	go cleanupLoop(authSvc)

	tlsMgr := tlsmanager.New(db, filepath.Join(filepath.Dir(cfg.DBPath), "certs"), cfg.HTTPSAddr)
	tlsMgr.Load()
	tlsMgr.StartRenewLoop()

	srv, err := api.New(cfg, db, pool, tasks, authSvc, notifier, tlsMgr)
	if err != nil {
		return err
	}

	handler := srv.Handler()
	// 证书签发后的明文 HTTP 拒绝统一由 internal/api 的 rejectPlainHTTP 中间件处理：
	// 业务请求一律 403，仅容器内 /api/health 放行（Docker 健康检查不依赖宿主机
	// HTTP 端口映射）。这里不再做 308 跳转；判定基于真实 TLS 连接，不信任客户端
	// 可伪造的 X-Forwarded-Proto。反代终结 TLS 或排障时可设
	// OCI_MANAGER_ALLOW_HTTP=true 作为逃生阀。
	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 15 * time.Second,
	}

	// HTTPS 监听始终启动（容器内 8443）。尚未配置证书时握手会失败，但容器内
	// HTTP 8080 始终可用，用户仍能进入后台完成配置；证书热加载后立即生效。
	// 对外是否暴露 HTTP / HTTPS 端口由 install.sh 生成的 compose override 决定。
	httpsSrv := &http.Server{
		Addr:              cfg.HTTPSAddr,
		Handler:           handler,
		ReadHeaderTimeout: 15 * time.Second,
	}
	tlsListener, err := tls.Listen("tcp", cfg.HTTPSAddr, tlsMgr.TLSConfig())
	if err != nil {
		return fmt.Errorf("启动 HTTPS 监听 %s 失败: %w", cfg.HTTPSAddr, err)
	}

	errCh := make(chan error, 2)
	go func() {
		log.Printf("OCI 管理器已启动，HTTP 监听 %s，HTTPS 监听 %s（加密密钥指纹 %s）",
			cfg.Addr, cfg.HTTPSAddr, cfg.KeyFingerprintHex())
		if !tlsMgr.HasCertificate() {
			log.Printf("尚未启用 HTTPS：请先不要录入真实 OCI 凭证；在「设置 → HTTPS 证书」配置证书后请重跑一键脚本关闭 HTTP 访问")
		}
		if st, err := authSvc.Status(); err == nil && !st.Initialized {
			log.Printf("尚未设置管理员账号，请打开面板完成初始化设置")
		}
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	go func() {
		if err := httpsSrv.Serve(tlsListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case <-quit:
		log.Printf("收到退出信号，正在关闭服务")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = httpsSrv.Shutdown(ctx)
	return httpSrv.Shutdown(ctx)
}

// cleanupLoop 定期清理过期的会话与登录挑战。
func cleanupLoop(svc *auth.Service) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		if err := svc.Cleanup(); err != nil {
			log.Printf("清理过期会话失败: %v", err)
		}
	}
}
