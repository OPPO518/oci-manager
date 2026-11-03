// Package store 负责账号凭证、代理配置与抢购任务的持久化。
// 使用 SQLite（modernc.org/sqlite，纯 Go 实现，无需 CGO）。
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/yourname/oci-manager/internal/secret"
)

// Store 封装数据库句柄与加密器。
type Store struct {
	db     *sql.DB
	sealer *secret.Sealer
}

// Open 打开（必要时创建）数据库并建表。
func Open(path string, sealer *secret.Sealer) (*Store, error) {
	// 容器里 /data 由卷挂载提供，但目录可能不存在或属主不对。
	// 这里主动创建，避免 SQLite 报 unable to open database file (14)。
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("创建数据库目录 %s 失败: %w", dir, err)
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	// SQLite 只支持单写者，限制连接池避免 "database is locked"
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(time.Hour)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("连接数据库失败（路径 %s，请确认该目录存在且进程有写权限）: %w", path, err)
	}

	s := &Store{db: db, sealer: sealer}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

// Close 关闭数据库连接。
func (s *Store) Close() error { return s.db.Close() }

// DB 暴露底层句柄，供测试使用。
func (s *Store) DB() *sql.DB { return s.db }

var (
	// ErrNotFound 表示记录不存在。
	ErrNotFound = errors.New("记录不存在")
)

func (s *Store) migrate() error {
	statements := []string{
		`PRAGMA journal_mode = WAL;`,
		`PRAGMA foreign_keys = ON;`,
		`CREATE TABLE IF NOT EXISTS accounts (
			id             TEXT PRIMARY KEY,
			name           TEXT NOT NULL,
			tenancy_ocid   TEXT NOT NULL,
			user_ocid      TEXT NOT NULL,
			fingerprint    TEXT NOT NULL,
			region         TEXT NOT NULL,
			private_key    TEXT NOT NULL,
			key_passphrase TEXT NOT NULL DEFAULT '',
			proxy_url      TEXT NOT NULL DEFAULT '',
			created_at     DATETIME NOT NULL,
			updated_at     DATETIME NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS grab_tasks (
			id              TEXT PRIMARY KEY,
			account_id      TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
			display_name    TEXT NOT NULL,
			region          TEXT NOT NULL,
			compartment_id  TEXT NOT NULL,
			ad_name         TEXT NOT NULL,
			shape           TEXT NOT NULL DEFAULT 'VM.Standard.A1.Flex',
			ocpus           INTEGER NOT NULL DEFAULT 4,
			memory_gbs      INTEGER NOT NULL DEFAULT 24,
			image_id        TEXT NOT NULL,
			subnet_id       TEXT NOT NULL,
			ssh_public_key  TEXT NOT NULL,
			boot_volume_gbs INTEGER NOT NULL DEFAULT 50,
			instance_count  INTEGER NOT NULL DEFAULT 1,
			interval_sec    INTEGER NOT NULL DEFAULT 60,
			max_attempts    INTEGER NOT NULL DEFAULT 0,
			status          TEXT NOT NULL DEFAULT 'idle',
			message         TEXT NOT NULL DEFAULT '',
			attempts        INTEGER NOT NULL DEFAULT 0,
			instance_ids    TEXT NOT NULL DEFAULT '',
			last_attempt_at DATETIME,
			created_at      DATETIME NOT NULL,
			updated_at      DATETIME NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_grab_tasks_account ON grab_tasks(account_id);`,
		`CREATE TABLE IF NOT EXISTS grab_task_logs (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id    TEXT NOT NULL,
			level      TEXT NOT NULL DEFAULT 'info',
			message    TEXT NOT NULL,
			created_at DATETIME NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_grab_task_logs_task ON grab_task_logs(task_id, id);`,

		// 键值形式的运行时设置。敏感项（如 Telegram Bot Token）由 Sealer 加密后存放。
		`CREATE TABLE IF NOT EXISTS settings (
			key        TEXT PRIMARY KEY,
			value      TEXT NOT NULL DEFAULT '',
			updated_at DATETIME NOT NULL
		);`,

		// 管理员账号。整个系统只允许一个，用 CHECK 约束保证。
		`CREATE TABLE IF NOT EXISTS admins (
			id            INTEGER PRIMARY KEY CHECK (id = 1),
			username      TEXT NOT NULL,
			password_hash TEXT NOT NULL,
			created_at    DATETIME NOT NULL,
			updated_at    DATETIME NOT NULL
		);`,

		`CREATE TABLE IF NOT EXISTS sessions (
			token        TEXT PRIMARY KEY,
			username     TEXT NOT NULL,
			created_at   DATETIME NOT NULL,
			expires_at   DATETIME NOT NULL,
			last_seen_at DATETIME NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);`,

		// 登录挑战：两步验证时暂存验证码哈希，验证码本身不落库。
		`CREATE TABLE IF NOT EXISTS login_challenges (
			token      TEXT PRIMARY KEY,
			username   TEXT NOT NULL,
			code_hash  TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			expires_at DATETIME NOT NULL,
			attempts   INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE INDEX IF NOT EXISTS idx_login_challenges_expires ON login_challenges(expires_at);`,
	}
	for _, stmt := range statements {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("建表失败: %w", err)
		}
	}
	return nil
}

// now 统一时间来源，便于测试替换。
func now() time.Time { return time.Now().UTC() }
