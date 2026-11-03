// Command server 是 OCI 个人账户管理器的服务入口。
package main

import (
	"context"
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
	"github.com/yourname/oci-manager/internal/config"
	"github.com/yourname/oci-manager/internal/grabber"
	"github.com/yourname/oci-manager/internal/oci"
	"github.com/yourname/oci-manager/internal/secret"
	"github.com/yourname/oci-manager/internal/store"
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
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("创建数据目录失败: %w", err)
		}
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

	pool := oci.NewPool(db)
	tasks := grabber.NewManager(db, pool)
	if err := tasks.RecoverOrphans(); err != nil {
		log.Printf("清理遗留任务失败: %v", err)
	}

	srv, err := api.New(cfg, db, pool, tasks)
	if err != nil {
		return err
	}

	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 15 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("OCI 管理器已启动，监听 %s", cfg.Addr)
		if cfg.WarnDevelopmentKey {
			log.Printf("警告：未设置环境变量 OCI_MANAGER_ENCRYPTION_KEY，正在使用默认密钥，请勿用于生产环境")
		}
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
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
	return httpSrv.Shutdown(ctx)
}
