// Package grabber 实现 ARM 实例抢购任务的调度。
//
// Oracle Cloud 的 Always Free ARM 资源（Ampere A1）长期处于超售状态，
// 创建实例时经常返回 "Out of host capacity"。本包通过定时重试来抢占
// 偶发释放的资源：只有容量不足才继续重试，其他错误一律终止任务，
// 避免因为配置错误（例如子网或镜像填错）而无限循环。
package grabber

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/yourname/oci-manager/internal/oci"
	"github.com/yourname/oci-manager/internal/store"
)

// Manager 管理抢购任务的生命周期。
type Manager struct {
	store *store.Store
	pool  *oci.Pool

	mu      sync.Mutex
	running map[string]*taskHandle
}

type taskHandle struct {
	cancel context.CancelFunc
}

// NewManager 创建调度器。
func NewManager(st *store.Store, pool *oci.Pool) *Manager {
	return &Manager{
		store:   st,
		pool:    pool,
		running: make(map[string]*taskHandle),
	}
}

// RecoverOrphans 在服务启动时清理上次运行遗留的 running 任务。
// 进程重启后对应的 goroutine 已不存在，必须把状态纠正过来，
// 否则前端会一直显示"运行中"。
func (m *Manager) RecoverOrphans() error {
	tasks, err := m.store.ListGrabTasks("")
	if err != nil {
		return err
	}
	for _, t := range tasks {
		if t.Status == store.StatusRunning {
			_ = m.store.UpdateGrabTaskStatus(t.ID, store.StatusStopped, "服务重启，任务已中断")
			_ = m.store.AppendLog(t.ID, "warn", "服务重启导致任务中断，可手动重新启动")
		}
	}
	return nil
}

// Start 启动一个抢购任务。已在运行中的任务会返回错误。
func (m *Manager) Start(taskID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.running[taskID]; ok {
		return fmt.Errorf("任务正在运行中")
	}
	task, err := m.store.GetGrabTask(taskID)
	if err != nil {
		return err
	}
	if task.Status == store.StatusSuccess {
		return fmt.Errorf("任务已成功完成，如需再建请新建任务")
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.running[taskID] = &taskHandle{cancel: cancel}

	if err := m.store.UpdateGrabTaskStatus(taskID, store.StatusRunning, "任务已启动"); err != nil {
		cancel()
		delete(m.running, taskID)
		return err
	}
	_ = m.store.AppendLog(taskID, "info", fmt.Sprintf("任务启动：%s / %s / %d OCPU / %d GB 内存",
		task.Shape, task.ADName, task.OCPUs, task.MemoryGBs))

	go func() {
		defer func() {
			m.mu.Lock()
			delete(m.running, taskID)
			m.mu.Unlock()
		}()
		m.loop(ctx, taskID)
	}()
	return nil
}

// Stop 停止一个运行中的任务。
func (m *Manager) Stop(taskID string) error {
	m.mu.Lock()
	handle, ok := m.running[taskID]
	m.mu.Unlock()
	if !ok {
		// 任务可能处于空闲或已结束状态，这里做幂等处理
		if task, err := m.store.GetGrabTask(taskID); err == nil && task.Status == store.StatusRunning {
			_ = m.store.UpdateGrabTaskStatus(taskID, store.StatusStopped, "任务已停止")
		}
		return nil
	}
	handle.cancel()
	_ = m.store.UpdateGrabTaskStatus(taskID, store.StatusStopped, "任务已停止")
	_ = m.store.AppendLog(taskID, "info", "收到停止指令")
	return nil
}

// IsRunning 返回任务是否正在运行。
func (m *Manager) IsRunning(taskID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.running[taskID]
	return ok
}

func (m *Manager) loop(ctx context.Context, taskID string) {
	for {
		task, err := m.store.GetGrabTask(taskID)
		if err != nil {
			return
		}

		attempt := task.Attempts + 1
		clients, err := m.pool.For(task.AccountID, task.Region)
		if err != nil {
			_ = m.store.UpdateGrabTaskStatus(taskID, store.StatusFailed, err.Error())
			_ = m.store.AppendLog(taskID, "error", "获取账号客户端失败："+err.Error())
			return
		}

		_ = m.store.AppendLog(taskID, "info", fmt.Sprintf("第 %d 次尝试创建实例", attempt))
		created, launchErr := m.launchAll(ctx, clients, task)

		if launchErr == nil {
			_, _ = m.store.RecordAttempt(taskID, true,
				fmt.Sprintf("成功创建 %d 台实例", len(created)), created)
			_ = m.store.AppendLog(taskID, "success",
				fmt.Sprintf("抢购成功，实例 ID：%v", created))
			return
		}

		if !oci.IsCapacityError(launchErr) {
			// 非容量问题（配额、权限、参数错误等）继续重试没有意义
			_, _ = m.store.RecordAttempt(taskID, false, launchErr.Error(), nil)
			_ = m.store.UpdateGrabTaskStatus(taskID, store.StatusFailed, launchErr.Error())
			_ = m.store.AppendLog(taskID, "error", "创建失败："+launchErr.Error())
			return
		}

		_, _ = m.store.RecordAttempt(taskID, false, "资源不足，等待下次重试", nil)
		_ = m.store.AppendLog(taskID, "warn", "可用域资源不足（Out of host capacity），稍后重试")

		if task.MaxAttempts > 0 && attempt >= task.MaxAttempts {
			_ = m.store.UpdateGrabTaskStatus(taskID, store.StatusFailed,
				fmt.Sprintf("已达最大尝试次数 %d，仍未抢到资源", task.MaxAttempts))
			return
		}

		interval := task.IntervalSec
		if interval <= 0 {
			interval = 60
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(interval) * time.Second):
		}
	}
}

// launchAll 按任务要求创建指定数量的实例，全部成功才返回 nil。
// 已创建成功的实例会保留，不会回滚。
func (m *Manager) launchAll(ctx context.Context, clients *oci.ClientSet, task *store.GrabTask) ([]string, error) {
	created := make([]string, 0, task.InstanceCount)
	for i := 0; i < task.InstanceCount; i++ {
		name := task.DisplayName
		if task.InstanceCount > 1 {
			name = fmt.Sprintf("%s-%d", task.DisplayName, i+1)
		}
		instance, err := oci.LaunchInstance(ctx, clients, oci.LaunchParams{
			DisplayName:        name,
			CompartmentID:      task.CompartmentID,
			AvailabilityDomain: task.ADName,
			Shape:              task.Shape,
			OCPUs:              float32(task.OCPUs),
			MemoryGBs:          float32(task.MemoryGBs),
			ImageID:            task.ImageID,
			SubnetID:           task.SubnetID,
			SSHPublicKey:       task.SSHPublicKey,
			BootVolumeGBs:      int64(task.BootVolumeGBs),
			AssignPublicIP:     true,
		})
		if err != nil {
			return created, err
		}
		created = append(created, instance.ID)
		_ = m.store.AppendLog(task.ID, "info", "实例创建成功："+instance.ID)
	}
	return created, nil
}
