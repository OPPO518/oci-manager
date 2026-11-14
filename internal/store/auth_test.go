package store

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"testing"
	"time"

	"github.com/yourname/oci-manager/internal/secret"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	sealer, err := secret.NewSealer(make([]byte, 32))
	if err != nil {
		t.Fatalf("构造加密器失败: %v", err)
	}
	s, err := Open(filepath.Join(dir, "test.db"), sealer)
	if err != nil {
		t.Fatalf("打开测试数据库失败: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// 会话令牌等同于管理员身份，绝不能以明文落库。
func TestSessionTokenIsHashed(t *testing.T) {
	s := newTestStore(t)
	token := "plain-session-token-1234567890"
	if err := s.CreateSession(token, "admin", time.Hour); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	var stored string
	if err := s.db.QueryRow(`SELECT token FROM sessions WHERE username = ?`, "admin").Scan(&stored); err != nil {
		t.Fatalf("读取会话记录: %v", err)
	}
	if stored == token {
		t.Fatal("会话令牌仍以明文写入数据库")
	}
	sum := sha256.Sum256([]byte(token))
	if want := hex.EncodeToString(sum[:]); stored != want {
		t.Fatalf("存储值不是令牌摘要，得到 %s，期望 %s", stored, want)
	}

	sess, ok, err := s.GetSession(token)
	if err != nil || !ok {
		t.Fatalf("用明文令牌应能查到会话: ok=%v err=%v", ok, err)
	}
	if sess.Username != "admin" {
		t.Fatalf("会话用户名错误: %s", sess.Username)
	}
	if err := s.DeleteSession(token); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, ok, _ = s.GetSession(token); ok {
		t.Fatal("删除后会话仍有效")
	}
}

// 改密码后必须能一次撤销所有设备上的会话。
func TestDeleteAllSessions(t *testing.T) {
	s := newTestStore(t)
	for _, tok := range []string{"token-a", "token-b", "token-c"} {
		if err := s.CreateSession(tok, "admin", time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.DeleteAllSessions(); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("撤销后仍有 %d 条会话", n)
	}
}

// 同一个验证码不能被并发请求重复消费。
func TestConsumeChallengeIsOneShot(t *testing.T) {
	s := newTestStore(t)
	if err := s.CreateChallenge("challenge-1", "admin", "hash", time.Hour); err != nil {
		t.Fatal(err)
	}
	ok, err := s.ConsumeChallenge("challenge-1")
	if err != nil || !ok {
		t.Fatalf("首次消费应成功: ok=%v err=%v", ok, err)
	}
	ok, err = s.ConsumeChallenge("challenge-1")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("同一挑战被消费了两次，存在并发重放风险")
	}
}
