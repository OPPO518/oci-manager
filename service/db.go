package service

import (
	"database/sql"
	"fmt"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite" // 引入纯 Go 版 SQLite 驱动，防报错神器
)

// DB 是全局的数据库连接池，其他地方都可以直接用它
var DB *sql.DB

// InitDB 负责数据库的开机自检和初始化
func InitDB() error {
	var err error
	// 1. 连接或自动创建 data.db 单文件数据库
	DB, err = sql.Open("sqlite", "./data.db")
	if err != nil {
		return fmt.Errorf("打开数据库失败: %v", err)
	}

	// 2. 自动建表
	if err = createTables(); err != nil {
		return fmt.Errorf("创建数据表失败: %v", err)
	}

	// 3. 注入默认管理员账号
	if err = initDefaultAdmin(); err != nil {
		return fmt.Errorf("初始化管理员失败: %v", err)
	}

	return nil
}

func createTables() error {
	// 创建管理员大门表
	adminTable := `
	CREATE TABLE IF NOT EXISTS admin_users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE,
		password_hash TEXT
	);`

	// 创建 OCI 资产表
	ociTable := `
	CREATE TABLE IF NOT EXISTS oci_accounts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		account_name TEXT UNIQUE,
		encrypted_config TEXT,
		proxy_url TEXT
	);`

	if _, err := DB.Exec(adminTable); err != nil {
		return err
	}
	if _, err := DB.Exec(ociTable); err != nil {
		return err
	}
	return nil
}

func initDefaultAdmin() error {
	// 检查表里是不是空荡荡的（没有管理员）
	var count int
	err := DB.QueryRow("SELECT COUNT(*) FROM admin_users").Scan(&count)
	if err != nil {
		return err
	}

	// 如果连一个管理员都没有，说明是首次运行，咱们给它弄个默认的
	if count == 0 {
		// bcrypt 登场：把明文密码 "admin123" 搅碎成乱码
		hash, err := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
		if err != nil {
			return err
		}

		// 把账号和搅碎后的乱码存进数据库
		_, err = DB.Exec("INSERT INTO admin_users (username, password_hash) VALUES (?, ?)", "admin", string(hash))
		if err != nil {
			return err
		}
		
		fmt.Println("==================================================")
		fmt.Println("⚠️ 系统检测到首次运行，已自动生成默认管理员！")
		fmt.Println("👉 账号: admin")
		fmt.Println("👉 密码: admin123")
		fmt.Println("⚠️ 强烈建议平台搭建完成后，立即在 Web 界面修改密码！")
		fmt.Println("==================================================")
	}
	return nil
}
