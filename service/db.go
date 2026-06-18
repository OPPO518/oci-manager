package service

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

var DB *sql.DB

// InitDB 初始化并自动升级 SQLite 数据库
func InitDB() error {
	var err error
	// 连接本地 SQLite 数据库文件
	DB, err = sql.Open("sqlite3", "./oci_manager.db")
	if err != nil {
		return fmt.Errorf("数据库连接失败: %v", err)
	}

	// 1. 创建或校验核心租户表 (保留旧的根基)
	createAccountsTable := `
	CREATE TABLE IF NOT EXISTS oci_accounts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		proxy_url TEXT,
		encrypted_config TEXT NOT NULL
	);`
	if _, err := DB.Exec(createAccountsTable); err != nil {
		return fmt.Errorf("创建 oci_accounts 表失败: %v", err)
	}

	// 2. 创系统全局安全配置表 (用于存放 TG Token, TOTP 密钥, 初始化标记等)
	createSettingsTable := `
	CREATE TABLE IF NOT EXISTS system_settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);`
	if _, err := DB.Exec(createSettingsTable); err != nil {
		return fmt.Errorf("创建 system_settings 表失败: %v", err)
	}

	// 3. 🚀 Schema V2 无损平滑升级机制：动态扩建所有业务与缓存字段
	// 采用忽略错误的 Exec 模式，如果列不存在则添加，已存在则静默跳过
	upgradeQueries := []string{
		// --- 租户展示与业务维度 ---
		"ALTER TABLE oci_accounts ADD COLUMN tenant_name TEXT DEFAULT '';",           // 租户名
		"ALTER TABLE oci_accounts ADD COLUMN cost REAL DEFAULT 0.0;",                 // 账号成本
		"ALTER TABLE oci_accounts ADD COLUMN alive_days INTEGER DEFAULT 0;",          // 存活天数
		"ALTER TABLE oci_accounts ADD COLUMN main_region TEXT DEFAULT '';",           // 主区域
		"ALTER TABLE oci_accounts ADD COLUMN is_multi_region BOOLEAN DEFAULT 0;",     // 是否多区
		"ALTER TABLE oci_accounts ADD COLUMN account_type TEXT DEFAULT 'Free Tier';", // 账号类型
		"ALTER TABLE oci_accounts ADD COLUMN status TEXT DEFAULT 'Active';",          // 账号状态
		"ALTER TABLE oci_accounts ADD COLUMN created_at DATETIME DEFAULT CURRENT_TIMESTAMP;", // 录入时间
		
		// --- 抢机任务队列 ---
		"ALTER TABLE oci_accounts ADD COLUMN provision_tasks TEXT DEFAULT '[]';",     // 正在进行的开机任务
		
		// --- 惰性按需缓存抽屉 (JSON 文本) ---
		"ALTER TABLE oci_accounts ADD COLUMN cached_instances TEXT DEFAULT '[]';",    // VPS 资产快照
		"ALTER TABLE oci_accounts ADD COLUMN cached_networks TEXT DEFAULT '[]';",     // VCN 网络资产快照
		"ALTER TABLE oci_accounts ADD COLUMN cached_limits TEXT DEFAULT '{}';",       // API 配额限制快照
	}

	log.Println("--- [oci-go DB] 正在校验并执行数据库 Schema V2 平滑升级 ---")
	for _, query := range upgradeQueries {
		_, _ = DB.Exec(query) 
	}
	log.Println("✅ [oci-go DB] 升级完成！所有缓存抽屉与高维业务字段已部署就绪。")

	return nil
}
