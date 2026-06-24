package service

import (
	"database/sql"
	"fmt"
)

// GetSetting 获取全局系统设置
func GetSetting(key string) (string, error) {
	var val string
	err := DB.QueryRow("SELECT value FROM system_settings WHERE key = ?", key).Scan(&val)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil // 没有找到记录时不报错，返回空字符串
		}
		return "", err
	}
	return val, nil
}

// SetSetting 写入或更新全局系统设置
func SetSetting(key, value string) error {
	_, err := DB.Exec("INSERT OR REPLACE INTO system_settings (key, value) VALUES (?, ?)", key, value)
	return err
}

// IsSystemInitialized 探针：系统是否已经完成了首次配置
func IsSystemInitialized() bool {
	val, err := GetSetting("sys_initialized")
	if err != nil {
		return false
	}
	return val == "true"
}
