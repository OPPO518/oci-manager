package store

import (
	"fmt"
	"time"

	"github.com/yourname/oci-manager/internal/logbus"
)

// AuditLog 是一条敏感操作记录。表只追加，接口层不提供删除。
type AuditLog struct {
	ID        int64  `json:"id"`
	CreatedAt string `json:"created_at"`
	AccountID string `json:"account_id"`
	Category  string `json:"category"`
	Operation string `json:"operation"`
	Target    string `json:"target"`
	Result    string `json:"result"`
	Detail    string `json:"detail"`
	ClientIP  string `json:"client_ip"`
}

// AppendAudit 记录一条审计日志。detail 里禁止放入私钥、一次性密码或 Cookie。
func (s *Store) AppendAudit(category, operation, target, result, detail, accountID, clientIP string) {
	if len(detail) > 1000 {
		detail = detail[:1000]
	}
	_, err := s.db.Exec(
		`INSERT INTO audit_logs (created_at, account_id, category, operation, target, result, detail, client_ip)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		time.Now().UTC().Format(time.RFC3339), accountID, category, operation, target, result, detail, clientIP,
	)
	// 审计失败不掩盖业务结果，但要留下可见线索。
	if err != nil {
		fmt.Printf("写入审计日志失败: %v\n", err)
	}

	// 实时日志流：审计事件同步推送到日志总线。
	var level logbus.Level
	switch result {
	case "success", "ok", "updated", "true":
		level = logbus.LevelSuccess
	case "failed", "error", "denied":
		level = logbus.LevelError
	case "blocked":
		level = logbus.LevelWarn
	default:
		level = logbus.LevelInfo
	}
	msg := fmt.Sprintf("[%s] %s %s%s",
		AuditCategoryLabel(category), AuditOperationLabel(operation), target, detailSuffix(detail))
	logbus.Default.Publish(level, logbus.SourceAudit, accountID, msg)
}

func detailSuffix(detail string) string {
	if detail == "" {
		return ""
	}
	if len(detail) > 120 {
		detail = detail[:120] + "…"
	}
	return " — " + detail
}

// ListAuditLogs 返回最近的审计记录，按时间倒序。limit 由调用方钳制。
func (s *Store) ListAuditLogs(limit int) ([]AuditLog, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := s.db.Query(
		`SELECT id, created_at, account_id, category, operation, target, result, detail, client_ip
		 FROM audit_logs ORDER BY id DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("读取审计日志失败: %w", err)
	}
	defer rows.Close()

	logs := make([]AuditLog, 0)
	for rows.Next() {
		var item AuditLog
		if err := rows.Scan(&item.ID, &item.CreatedAt, &item.AccountID, &item.Category,
			&item.Operation, &item.Target, &item.Result, &item.Detail, &item.ClientIP); err != nil {
			return nil, err
		}
		logs = append(logs, item)
	}
	return logs, rows.Err()
}
