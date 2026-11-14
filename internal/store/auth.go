package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// sessionTokenHash 把会话令牌摘要后入库。
// 令牌等同于管理员身份，若明文落库，任何读到 data/ 目录或备份的人都能
// 直接设置 Cookie 接管面板，连密码和两步验证都不用破。
func sessionTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Admin 是系统唯一的管理员账号。
type Admin struct {
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// GetAdmin 读取管理员账号，第二个返回值表示是否存在。
func (s *Store) GetAdmin() (*Admin, bool, error) {
	var (
		a           Admin
		createdText string
		updatedText string
	)
	err := s.db.QueryRow(
		`SELECT username, password_hash, created_at, updated_at FROM admins WHERE id = 1`,
	).Scan(&a.Username, &a.PasswordHash, &createdText, &updatedText)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("读取管理员账号失败: %w", err)
	}
	a.CreatedAt = parseTime(createdText)
	a.UpdatedAt = parseTime(updatedText)
	return &a, true, nil
}

// CreateAdmin 创建管理员账号。已存在时返回错误。
func (s *Store) CreateAdmin(username, passwordHash string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(
		`INSERT INTO admins (id, username, password_hash, created_at, updated_at)
		 VALUES (1, ?, ?, ?, ?)`,
		username, passwordHash, now, now,
	)
	if err != nil {
		return fmt.Errorf("创建管理员账号失败: %w", err)
	}
	affected, err := res.RowsAffected()
	if err == nil && affected == 0 {
		return errors.New("管理员账号已存在")
	}
	return nil
}

// UpdateAdminPassword 更新管理员密码哈希。
func (s *Store) UpdateAdminPassword(passwordHash string) error {
	_, err := s.db.Exec(
		`UPDATE admins SET password_hash = ?, updated_at = ? WHERE id = 1`,
		passwordHash, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("更新密码失败: %w", err)
	}
	return nil
}

// Session 是一次登录会话。Token 恒为摘要值，绝不放明文。
type Session struct {
	Token      string    `json:"-"`
	Username   string    `json:"username"`
	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
}

// CreateSession 写入会话记录，只保存令牌摘要。
func (s *Store) CreateSession(token, username string, ttl time.Duration) error {
	now := time.Now().UTC()
	_, err := s.db.Exec(
		`INSERT INTO sessions (token, username, created_at, expires_at, last_seen_at)
		 VALUES (?, ?, ?, ?, ?)`,
		sessionTokenHash(token), username,
		now.Format(time.RFC3339),
		now.Add(ttl).Format(time.RFC3339),
		now.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("创建会话失败: %w", err)
	}
	return nil
}

// GetSession 读取会话，过期会话视为不存在。
func (s *Store) GetSession(token string) (*Session, bool, error) {
	var (
		sess        Session
		createdText string
		expiresText string
		seenText    string
	)
	err := s.db.QueryRow(
		`SELECT token, username, created_at, expires_at, last_seen_at FROM sessions WHERE token = ?`,
		sessionTokenHash(token),
	).Scan(&sess.Token, &sess.Username, &createdText, &expiresText, &seenText)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("读取会话失败: %w", err)
	}
	sess.CreatedAt = parseTime(createdText)
	sess.ExpiresAt = parseTime(expiresText)
	sess.LastSeenAt = parseTime(seenText)
	if !sess.ExpiresAt.After(time.Now().UTC()) {
		_ = s.DeleteSession(token)
		return nil, false, nil
	}
	return &sess, true, nil
}

// TouchSession 更新会话的最后访问时间（用于滑动续期）。
func (s *Store) TouchSession(token string, ttl time.Duration) error {
	now := time.Now().UTC()
	_, err := s.db.Exec(
		`UPDATE sessions SET last_seen_at = ?, expires_at = ? WHERE token = ?`,
		now.Format(time.RFC3339), now.Add(ttl).Format(time.RFC3339), sessionTokenHash(token),
	)
	return err
}

// DeleteSession 删除会话。
func (s *Store) DeleteSession(token string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token = ?`, sessionTokenHash(token))
	return err
}

// DeleteAllSessions 撤销全部会话。修改密码或怀疑泄露时调用，
// 保证旧设备即使持有 Cookie 也会被立刻踢下线。
func (s *Store) DeleteAllSessions() error {
	_, err := s.db.Exec(`DELETE FROM sessions`)
	return err
}

// DeleteExpiredSessions 清理过期会话。
func (s *Store) DeleteExpiredSessions() error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE expires_at <= ?`, time.Now().UTC().Format(time.RFC3339))
	return err
}

// Challenge 是两步验证过程中的一次性挑战。
type Challenge struct {
	Token     string    `json:"token"`
	Username  string    `json:"username"`
	CodeHash  string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Attempts  int       `json:"attempts"`
}

// CreateChallenge 写入登录挑战。
func (s *Store) CreateChallenge(token, username, codeHash string, ttl time.Duration) error {
	now := time.Now().UTC()
	_, err := s.db.Exec(
		`INSERT INTO login_challenges (token, username, code_hash, created_at, expires_at, attempts)
		 VALUES (?, ?, ?, ?, ?, 0)`,
		token, username, codeHash,
		now.Format(time.RFC3339), now.Add(ttl).Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("创建登录挑战失败: %w", err)
	}
	return nil
}

// GetChallenge 读取登录挑战，过期的视为不存在。
func (s *Store) GetChallenge(token string) (*Challenge, bool, error) {
	var (
		ch          Challenge
		createdText string
		expiresText string
	)
	err := s.db.QueryRow(
		`SELECT token, username, code_hash, created_at, expires_at, attempts
		 FROM login_challenges WHERE token = ?`, token,
	).Scan(&ch.Token, &ch.Username, &ch.CodeHash, &createdText, &expiresText, &ch.Attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("读取登录挑战失败: %w", err)
	}
	ch.CreatedAt = parseTime(createdText)
	ch.ExpiresAt = parseTime(expiresText)
	if !ch.ExpiresAt.After(time.Now().UTC()) {
		_ = s.DeleteChallenge(token)
		return nil, false, nil
	}
	return &ch, true, nil
}

// BumpChallengeAttempts 累加验证码尝试次数。
func (s *Store) BumpChallengeAttempts(token string) error {
	_, err := s.db.Exec(`UPDATE login_challenges SET attempts = attempts + 1 WHERE token = ?`, token)
	return err
}

// ConsumeChallenge 原子地消费挑战：只有成功删掉唯一一条记录才返回 true。
// 用于杜绝「同一验证码并发请求都通过 bcrypt 比较、换到多张会话」的竞态重放。
func (s *Store) ConsumeChallenge(token string) (bool, error) {
	res, err := s.db.Exec(`DELETE FROM login_challenges WHERE token = ?`, token)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

// DeleteChallenge 删除登录挑战。
func (s *Store) DeleteChallenge(token string) error {
	_, err := s.db.Exec(`DELETE FROM login_challenges WHERE token = ?`, token)
	return err
}

// DeleteExpiredChallenges 清理过期挑战。
func (s *Store) DeleteExpiredChallenges() error {
	_, err := s.db.Exec(
		`DELETE FROM login_challenges WHERE expires_at <= ?`,
		time.Now().UTC().Format(time.RFC3339),
	)
	return err
}
