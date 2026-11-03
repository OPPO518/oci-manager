package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// GetSetting 读取设置项，不存在时返回空字符串。
func (s *Store) GetSetting(key string) (string, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("读取设置 %s 失败: %w", key, err)
	}
	return value, nil
}

// SetSetting 写入设置项。
func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("写入设置 %s 失败: %w", key, err)
	}
	return nil
}

// GetSecretSetting 读取加密设置项并返回明文。
func (s *Store) GetSecretSetting(key string) (string, error) {
	encoded, err := s.GetSetting(key)
	if err != nil || encoded == "" {
		return "", err
	}
	plain, err := s.sealer.Decrypt(encoded)
	if err != nil {
		return "", fmt.Errorf("解密设置 %s 失败: %w", key, err)
	}
	return plain, nil
}

// SetSecretSetting 加密后写入设置项。传空字符串表示清除该项。
func (s *Store) SetSecretSetting(key, value string) error {
	if value == "" {
		return s.SetSetting(key, "")
	}
	encoded, err := s.sealer.Encrypt(value)
	if err != nil {
		return fmt.Errorf("加密设置 %s 失败: %w", key, err)
	}
	return s.SetSetting(key, encoded)
}
