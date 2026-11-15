package store

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// 电源任务动作。与 OCI InstanceAction 的取值保持一致。
const (
	PowerActionStart    = "start"
	PowerActionStop     = "stop"
	PowerActionReset    = "reset"
	PowerTargetAll      = "all"
	PowerTargetSelected = "selected"
)

// PowerTask 是一条定时开关机任务。
//
// 调度语义：当服务器本地时间命中 days_of_week（"*" 表示每天）且等于
// hour:minute 时触发一次。调度器会保证每个 (hour,minute,days_of_week) 窗口
// 至少执行一次，执行结果写入 last_run / last_result。
type PowerTask struct {
	ID            int64      `json:"id"`
	AccountID     string     `json:"account_id"`
	DisplayName   string     `json:"display_name"`
	Action        string     `json:"action"`
	TargetMode    string     `json:"target_mode"`
	InstanceIDs   string     `json:"instance_ids"`
	CompartmentID string     `json:"compartment_id"`
	DaysOfWeek    string     `json:"days_of_week"`
	Hour          int        `json:"hour"`
	Minute        int        `json:"minute"`
	Enabled       bool       `json:"enabled"`
	LastRun       *time.Time `json:"last_run"`
	LastResult    string     `json:"last_result"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// PowerTaskInput 是创建/更新任务的入参。
type PowerTaskInput struct {
	DisplayName   string
	Action        string
	TargetMode    string
	InstanceIDs   []string
	CompartmentID string
	DaysOfWeek    []int // 0=周日 ... 6=周六；空表示每天(*)
	Hour          int
	Minute        int
	Enabled       bool
}

// PowerTaskDOW 把周几切片序列化成存储用的字符串。
func PowerTaskDOW(days []int) string {
	if len(days) == 0 {
		return "*"
	}
	sort.Ints(days)
	parts := make([]string, 0, len(days))
	for _, d := range days {
		parts = append(parts, fmt.Sprintf("%d", d))
	}
	return strings.Join(parts, ",")
}

// ParsePowerTaskDOW 解析存储字符串为周几切片；"*" 或空返回 nil。
func ParsePowerTaskDOW(s string) []int {
	s = strings.TrimSpace(s)
	if s == "" || s == "*" {
		return nil
	}
	out := make([]int, 0)
	for _, p := range strings.Split(s, ",") {
		var d int
		if _, err := fmt.Sscanf(strings.TrimSpace(p), "%d", &d); err == nil && d >= 0 && d <= 6 {
			out = append(out, d)
		}
	}
	return out
}

// CreatePowerTask 写入一条新任务。
func (s *Store) CreatePowerTask(accountID string, in PowerTaskInput) (*PowerTask, error) {
	if in.Action != PowerActionStart && in.Action != PowerActionStop && in.Action != PowerActionReset {
		return nil, fmt.Errorf("动作仅支持 start/stop/reset")
	}
	if in.TargetMode != PowerTargetAll && in.TargetMode != PowerTargetSelected {
		return nil, fmt.Errorf("目标模式仅支持 all/selected")
	}
	if in.Hour < 0 || in.Hour > 23 {
		return nil, fmt.Errorf("小时需在 0-23 之间")
	}
	if in.Minute < 0 || in.Minute > 59 {
		return nil, fmt.Errorf("分钟需在 0-59 之间")
	}
	ts := now()
	dow := PowerTaskDOW(in.DaysOfWeek)
	res, err := s.db.Exec(
		`INSERT INTO power_tasks
		 (account_id, display_name, action, target_mode, instance_ids, compartment_id,
		  days_of_week, hour, minute, enabled, last_run, last_result, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		accountID, in.DisplayName, in.Action, in.TargetMode, strings.Join(in.InstanceIDs, ","),
		in.CompartmentID, dow, in.Hour, in.Minute, boolToInt(in.Enabled), nil, "",
		ts.Format(time.RFC3339), ts.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("创建定时任务失败: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.GetPowerTask(id)
}

// GetPowerTask 按主键读取。
func (s *Store) GetPowerTask(id int64) (*PowerTask, error) {
	row := s.db.QueryRow(
		`SELECT id, account_id, display_name, action, target_mode, instance_ids, compartment_id,
		        days_of_week, hour, minute, enabled, last_run, last_result, created_at, updated_at
		 FROM power_tasks WHERE id = ?`, id,
	)
	return scanPowerTask(row)
}

// ListPowerTasks 列出某账号的任务；accountID 为空表示全部。
func (s *Store) ListPowerTasks(accountID string) ([]PowerTask, error) {
	var rows *sql.Rows
	var err error
	query := `SELECT id, account_id, display_name, action, target_mode, instance_ids, compartment_id,
			days_of_week, hour, minute, enabled, last_run, last_result, created_at, updated_at
			FROM power_tasks`
	if accountID == "" {
		rows, err = s.db.Query(query + " ORDER BY created_at DESC")
	} else {
		rows, err = s.db.Query(query+" WHERE account_id = ? ORDER BY created_at DESC", accountID)
	}
	if err != nil {
		return nil, fmt.Errorf("读取定时任务失败: %w", err)
	}
	defer rows.Close()
	out := make([]PowerTask, 0)
	for rows.Next() {
		t, e := scanPowerTaskFromRows(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// ListAllEnabledPowerTasks 返回所有启用中的任务，供调度器枚举。
func (s *Store) ListAllEnabledPowerTasks() ([]PowerTask, error) {
	rows, err := s.db.Query(
		`SELECT id, account_id, display_name, action, target_mode, instance_ids, compartment_id,
			days_of_week, hour, minute, enabled, last_run, last_result, created_at, updated_at
		FROM power_tasks WHERE enabled = 1`)
	if err != nil {
		return nil, fmt.Errorf("读取定时任务失败: %w", err)
	}
	defer rows.Close()
	out := make([]PowerTask, 0)
	for rows.Next() {
		t, e := scanPowerTaskFromRows(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// UpdatePowerTask 更新一条任务（按 id 全量更新可写字段）。
func (s *Store) UpdatePowerTask(id int64, in PowerTaskInput) (*PowerTask, error) {
	if in.Action != PowerActionStart && in.Action != PowerActionStop && in.Action != PowerActionReset {
		return nil, fmt.Errorf("动作仅支持 start/stop/reset")
	}
	if in.TargetMode != PowerTargetAll && in.TargetMode != PowerTargetSelected {
		return nil, fmt.Errorf("目标模式仅支持 all/selected")
	}
	if in.Hour < 0 || in.Hour > 23 {
		return nil, fmt.Errorf("小时需在 0-23 之间")
	}
	if in.Minute < 0 || in.Minute > 59 {
		return nil, fmt.Errorf("分钟需在 0-59 之间")
	}
	ts := now()
	dow := PowerTaskDOW(in.DaysOfWeek)
	_, err := s.db.Exec(
		`UPDATE power_tasks SET display_name=?, action=?, target_mode=?, instance_ids=?, compartment_id=?,
		 days_of_week=?, hour=?, minute=?, enabled=?, updated_at=? WHERE id=?`,
		in.DisplayName, in.Action, in.TargetMode, strings.Join(in.InstanceIDs, ","),
		in.CompartmentID, dow, in.Hour, in.Minute, boolToInt(in.Enabled), ts.Format(time.RFC3339), id,
	)
	if err != nil {
		return nil, fmt.Errorf("更新定时任务失败: %w", err)
	}
	return s.GetPowerTask(id)
}

// DeletePowerTask 删除一条任务。
func (s *Store) DeletePowerTask(id int64) error {
	_, err := s.db.Exec(`DELETE FROM power_tasks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("删除定时任务失败: %w", err)
	}
	return nil
}

// MarkPowerTaskRun 写入本次触发时间与结果摘要。
func (s *Store) MarkPowerTaskRun(id int64, result string) error {
	ts := now()
	_, err := s.db.Exec(
		`UPDATE power_tasks SET last_run=?, last_result=?, updated_at=? WHERE id=?`,
		ts.Format(time.RFC3339), result, ts.Format(time.RFC3339), id,
	)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// scanPowerTask 从单行读取。
func scanPowerTask(row *sql.Row) (*PowerTask, error) {
	var (
		t           PowerTask
		enabled     int
		lastRun     sql.NullString
		createdAt   string
		updatedAt   string
		instanceIDs string
	)
	err := row.Scan(
		&t.ID, &t.AccountID, &t.DisplayName, &t.Action, &t.TargetMode, &instanceIDs,
		&t.CompartmentID, &t.DaysOfWeek, &t.Hour, &t.Minute, &enabled, &lastRun,
		&t.LastResult, &createdAt, &updatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	t.Enabled = enabled == 1
	if lastRun.Valid && lastRun.String != "" {
		lr := parseTime(lastRun.String)
		t.LastRun = &lr
	}
	t.InstanceIDs = instanceIDs
	t.CreatedAt = parseTime(createdAt)
	t.UpdatedAt = parseTime(updatedAt)
	return &t, nil
}

// scanPowerTaskFromRows 从多行读取（复用同一列序）。
func scanPowerTaskFromRows(rows *sql.Rows) (*PowerTask, error) {
	var (
		t           PowerTask
		enabled     int
		lastRun     sql.NullString
		createdAt   string
		updatedAt   string
		instanceIDs string
	)
	err := rows.Scan(
		&t.ID, &t.AccountID, &t.DisplayName, &t.Action, &t.TargetMode, &instanceIDs,
		&t.CompartmentID, &t.DaysOfWeek, &t.Hour, &t.Minute, &enabled, &lastRun,
		&t.LastResult, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}
	t.Enabled = enabled == 1
	if lastRun.Valid && lastRun.String != "" {
		lr := parseTime(lastRun.String)
		t.LastRun = &lr
	}
	t.InstanceIDs = instanceIDs
	t.CreatedAt = parseTime(createdAt)
	t.UpdatedAt = parseTime(updatedAt)
	return &t, nil
}
