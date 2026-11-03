package store

import (
	"database/sql"
	"strings"
	"time"
)

// 抢购任务状态取值。
const (
	StatusIdle     = "idle"     // 未启动
	StatusRunning  = "running"  // 正在轮询重试
	StatusSuccess  = "success"  // 已成功创建实例
	StatusFailed   = "failed"   // 停止并失败
	StatusStopped  = "stopped"  // 用户手动停止
)

// GrabTask 描述一个 ARM 实例抢购任务。
type GrabTask struct {
	ID             string     `json:"id"`
	AccountID      string     `json:"account_id"`
	DisplayName    string     `json:"display_name"`
	Region         string     `json:"region"`
	CompartmentID  string     `json:"compartment_id"`
	ADName         string     `json:"ad_name"`
	Shape          string     `json:"shape"`
	OCPUs          int        `json:"ocpus"`
	MemoryGBs      int        `json:"memory_gbs"`
	ImageID        string     `json:"image_id"`
	SubnetID       string     `json:"subnet_id"`
	SSHPublicKey   string     `json:"ssh_public_key"`
	BootVolumeGBs  int        `json:"boot_volume_gbs"`
	InstanceCount  int        `json:"instance_count"`
	IntervalSec    int        `json:"interval_sec"`
	MaxAttempts    int        `json:"max_attempts"`
	Status         string     `json:"status"`
	Message        string     `json:"message"`
	Attempts       int        `json:"attempts"`
	InstanceIDs    []string   `json:"instance_ids"`
	LastAttemptAt  *time.Time `json:"last_attempt_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// GrabTaskInput 用于创建抢购任务。
type GrabTaskInput struct {
	AccountID     string
	DisplayName   string
	Region        string
	CompartmentID string
	ADName        string
	Shape         string
	OCPUs         int
	MemoryGBs     int
	ImageID       string
	SubnetID      string
	SSHPublicKey  string
	BootVolumeGBs int
	InstanceCount int
	IntervalSec   int
	MaxAttempts   int
}

// GrabTaskLog 是任务运行日志的一行。
type GrabTaskLog struct {
	ID        int64     `json:"id"`
	TaskID    string    `json:"task_id"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

const grabTaskColumns = `id, account_id, display_name, region, compartment_id, ad_name, shape,
	ocpus, memory_gbs, image_id, subnet_id, ssh_public_key, boot_volume_gbs,
	instance_count, interval_sec, max_attempts, status, message, attempts,
	instance_ids, last_attempt_at, created_at, updated_at`

func scanGrabTask(row interface{ Scan(...any) error }) (*GrabTask, error) {
	var (
		t           GrabTask
		instanceIDs string
		lastAttempt sql.NullString
		createdAt   string
		updatedAt   string
	)
	err := row.Scan(&t.ID, &t.AccountID, &t.DisplayName, &t.Region, &t.CompartmentID,
		&t.ADName, &t.Shape, &t.OCPUs, &t.MemoryGBs, &t.ImageID, &t.SubnetID,
		&t.SSHPublicKey, &t.BootVolumeGBs, &t.InstanceCount, &t.IntervalSec,
		&t.MaxAttempts, &t.Status, &t.Message, &t.Attempts, &instanceIDs,
		&lastAttempt, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	t.InstanceIDs = splitIDs(instanceIDs)
	if lastAttempt.Valid {
		ts := parseTime(lastAttempt.String)
		t.LastAttemptAt = &ts
	}
	t.CreatedAt = parseTime(createdAt)
	t.UpdatedAt = parseTime(updatedAt)
	return &t, nil
}

func splitIDs(v string) []string {
	if strings.TrimSpace(v) == "" {
		return []string{}
	}
	return strings.Split(v, ",")
}

// CreateGrabTask 创建抢购任务。
func (s *Store) CreateGrabTask(in GrabTaskInput) (*GrabTask, error) {
	if strings.TrimSpace(in.DisplayName) == "" {
		return nil, errTaskField("任务名称不能为空")
	}
	if strings.TrimSpace(in.CompartmentID) == "" {
		return nil, errTaskField("区间 OCID 不能为空")
	}
	if strings.TrimSpace(in.SubnetID) == "" {
		return nil, errTaskField("子网 OCID 不能为空")
	}
	if in.OCPUs <= 0 {
		return nil, errTaskField("OCPU 数量必须大于 0")
	}
	if in.MemoryGBs <= 0 {
		return nil, errTaskField("内存容量必须大于 0")
	}
	if in.InstanceCount <= 0 {
		in.InstanceCount = 1
	}
	if in.IntervalSec <= 0 {
		in.IntervalSec = 60
	}
	if in.Shape == "" {
		in.Shape = "VM.Standard.A1.Flex"
	}
	if in.BootVolumeGBs <= 0 {
		in.BootVolumeGBs = 50
	}

	id, err := newID()
	if err != nil {
		return nil, err
	}
	ts := now()
	_, err = s.db.Exec(`INSERT INTO grab_tasks (`+grabTaskColumns+`) VALUES
		(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		id, in.AccountID, in.DisplayName, in.Region, in.CompartmentID, in.ADName,
		in.Shape, in.OCPUs, in.MemoryGBs, in.ImageID, in.SubnetID, in.SSHPublicKey,
		in.BootVolumeGBs, in.InstanceCount, in.IntervalSec, in.MaxAttempts,
		StatusIdle, "", 0, "", nil, ts, ts)
	if err != nil {
		return nil, err
	}
	return s.GetGrabTask(id)
}

// GetGrabTask 按 ID 取任务。
func (s *Store) GetGrabTask(id string) (*GrabTask, error) {
	row := s.db.QueryRow(`SELECT `+grabTaskColumns+` FROM grab_tasks WHERE id = ?`, id)
	t, err := scanGrabTask(row)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return t, err
}

// ListGrabTasks 列出任务；accountID 为空时返回全部。
func (s *Store) ListGrabTasks(accountID string) ([]GrabTask, error) {
	query := `SELECT ` + grabTaskColumns + ` FROM grab_tasks`
	args := []any{}
	if accountID != "" {
		query += ` WHERE account_id = ?`
		args = append(args, accountID)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := make([]GrabTask, 0)
	for rows.Next() {
		t, err := scanGrabTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, *t)
	}
	return tasks, rows.Err()
}

// UpdateGrabTaskStatus 更新任务状态与提示信息。
func (s *Store) UpdateGrabTaskStatus(id, status, message string) error {
	res, err := s.db.Exec(`UPDATE grab_tasks SET status=?, message=?, updated_at=? WHERE id=?`,
		status, message, now(), id)
	return ensureAffected(res, err)
}

// RecordAttempt 记录一次尝试：累加计数、写入最后尝试时间、可选更新状态。
// 返回更新后的任务。
func (s *Store) RecordAttempt(id string, succeeded bool, message string, instanceIDs []string) (*GrabTask, error) {
	status := ""
	if succeeded {
		status = StatusSuccess
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`UPDATE grab_tasks SET attempts = attempts + 1, last_attempt_at = ?, updated_at = ?
		WHERE id = ?`, now(), now(), id); err != nil {
		return nil, err
	}
	if status != "" || instanceIDs != nil {
		if _, err := tx.Exec(`UPDATE grab_tasks SET status = ?, message = ?, instance_ids = ? WHERE id = ?`,
			status, message, strings.Join(instanceIDs, ","), id); err != nil {
			return nil, err
		}
	} else if message != "" {
		if _, err := tx.Exec(`UPDATE grab_tasks SET message = ? WHERE id = ?`, message, id); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetGrabTask(id)
}

// DeleteGrabTask 删除任务及其日志。
func (s *Store) DeleteGrabTask(id string) error {
	if _, err := s.db.Exec(`DELETE FROM grab_task_logs WHERE task_id = ?`, id); err != nil {
		return err
	}
	res, err := s.db.Exec(`DELETE FROM grab_tasks WHERE id = ?`, id)
	return ensureAffected(res, err)
}

// AppendLog 写入一行任务日志。
func (s *Store) AppendLog(taskID, level, message string) error {
	_, err := s.db.Exec(`INSERT INTO grab_task_logs (task_id, level, message, created_at) VALUES (?,?,?,?)`,
		taskID, level, message, now())
	return err
}

// ListLogs 返回任务日志，afterID 用于增量拉取。limit 为 0 时使用默认值 200。
func (s *Store) ListLogs(taskID string, afterID, limit int) ([]GrabTaskLog, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.Query(`SELECT id, task_id, level, message, created_at FROM grab_task_logs
		WHERE task_id = ? AND id > ? ORDER BY id ASC LIMIT ?`, taskID, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	logs := make([]GrabTaskLog, 0)
	for rows.Next() {
		var l GrabTaskLog
		var createdAt string
		if err := rows.Scan(&l.ID, &l.TaskID, &l.Level, &l.Message, &createdAt); err != nil {
			return nil, err
		}
		l.CreatedAt = parseTime(createdAt)
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

func errTaskField(msg string) error { return &fieldError{msg} }

type fieldError struct{ msg string }

func (e *fieldError) Error() string { return e.msg }
