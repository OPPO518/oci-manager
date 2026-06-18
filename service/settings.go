package service

import (
	"fmt"
)

// GetSetting 获取系统设置
func GetSetting(key string) (string, error) {
	var val string
	err := DB.QueryRow("SELECT value FROM system_settings WHERE key = ?", key).Scan(&val)
	if err != nil {
		return "", err
	}
	return val, nil
}

// SetSetting 写入系统设置
func SetSetting(key, value string) error {
	_, err := DB.Exec("INSERT OR REPLACE INTO system_settings (key, value) VALUES (?, ?)", key, value)
	return err
}

// IsSystemInitialized 判断系统是否已经完成初始化
func IsSystemInitialized() bool {
	val, err := GetSetting("initialized")
	return err == nil && val == "true"
}
