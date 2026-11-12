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
	"log"
	"strings"
	"sync"
	"time"

	"github.com/yourname/oci-manager/internal/oci"
	"github.com/yourname/oci-manager/internal/store"
)

// Notifier 用于在抢购任务状态变化时推送通知。
type Notifier interface {
	// GrabEventEnabled 返回是否开启了抢购事件通知。
	GrabEventEnabled() bool
	// Send 推送一条消息。
	Send(text string) error
}

// Manager 管理抢购任务的生命周期。
type Manager struct {
	store  *store.Store
	pool   *oci.Pool
	notify Notifier

	mu      sync.Mutex
	running map[string]*taskHandle
}

type taskHandle struct {
	cancel context.CancelFunc
}

// NewManager 创建调度器。notify 可为 nil，表示不推送通知。
func NewManager(st *store.Store, pool *oci.Pool, notify Notifier) *Manager {
	return &Manager{
		store:   st,
		pool:    pool,
		notify:  notify,
		running: make(map[string]*taskHandle),
	}
}

// notifyGrab 推送抢购事件。未配置通知或发送失败都不影响任务本身。
func (m *Manager) notifyGrab(text string) {
	if m.notify == nil || !m.notify.GrabEventEnabled() {
		return
	}
	if err := m.notify.Send(text); err != nil {
		log.Printf("推送抢购通知失败: %v", err)
	}
}

// taskSummary 生成任务摘要，用于通知正文。
func taskSummary(t *store.GrabTask) string {
	var sb strings.Builder
	sb.WriteString("任务：")
	sb.WriteString(t.DisplayName)
	sb.WriteString("\n区域：")
	sb.WriteString(t.Region)
	sb.WriteString("\n规格：")
	fmt.Fprintf(&sb, "%s（%d OCPU / %d GB）", t.Shape, t.OCPUs, t.MemoryGBs)
	sb.WriteString("\n可用域：")
	sb.WriteString(t.ADName)
	return sb.String()
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
	// precheckAvailable 记录容量报告接口是否可用。首次发现不可用时降级为
	// 直接创建，并关闭后续预检，避免每轮都白跑一次注定失败的请求。
	precheckAvailable := true

	for {
		task, err := m.store.GetGrabTask(taskID)
		if err != nil {
			return
		}

		clients, err := m.pool.For(task.AccountID, task.Region)
		if err != nil {
			_ = m.store.UpdateGrabTaskStatus(taskID, store.StatusFailed, err.Error())
			_ = m.store.AppendLog(taskID, "error", "获取账号客户端失败："+err.Error())
			m.notifyGrab("❌ 抢购任务已终止\n\n" + taskSummary(task) +
				"\n原因：获取账号客户端失败：" + err.Error())
			return
		}

		// ---- 抢购前容量预检 ----
		// 用只读的 Compute Capacity Report 判断是否有货，只有有货时才发起
		// LaunchInstance。这样绝大多数轮询不会产生写调用，显著降低风控风险。
		if precheckAvailable {
			launch, note := m.capacityPrecheck(ctx, clients, task)
			_ = m.store.AppendLog(taskID, "info", note)
			if !launch {
				interval := task.IntervalSec
				if interval <= 0 {
					interval = 60
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Duration(interval) * time.Second):
				}
				continue
			}
			if strings.Contains(note, "降级") || strings.Contains(note, "预检失败") {
				precheckAvailable = false
			}
		}

		attempt := task.Attempts + 1
		_ = m.store.AppendLog(taskID, "info", fmt.Sprintf("第 %d 次尝试创建实例", attempt))
		created, launchErr := m.launchAll(ctx, clients, task)

		if launchErr == nil {
			_, _ = m.store.RecordAttempt(taskID, true,
				fmt.Sprintf("成功创建 %d 台实例", len(created)), created)
			_ = m.store.AppendLog(taskID, "success",
				fmt.Sprintf("抢购成功，实例 ID：%v", created))
			m.notifyGrab("🎉 ARM 抢购成功\n\n" + taskSummary(task) +
				fmt.Sprintf("\n实例 ID：%s\n第 %d 次尝试", strings.Join(created, "\n"), attempt))
			return
		}

		if !oci.IsCapacityError(launchErr) {
			// 非容量问题（配额、权限、参数错误等）继续重试没有意义
			_, _ = m.store.RecordAttempt(taskID, false, launchErr.Error(), nil)
			_ = m.store.UpdateGrabTaskStatus(taskID, store.StatusFailed, launchErr.Error())
			_ = m.store.AppendLog(taskID, "error", "创建失败："+launchErr.Error())
			m.notifyGrab("❌ 抢购任务已终止\n\n" + taskSummary(task) +
				"\n原因：" + launchErr.Error())
			return
		}

		_, _ = m.store.RecordAttempt(taskID, false, "资源不足，等待下次重试", nil)
		_ = m.store.AppendLog(taskID, "warn", "可用域资源不足（Out of host capacity），稍后重试")

		if task.MaxAttempts > 0 && attempt >= task.MaxAttempts {
			_ = m.store.UpdateGrabTaskStatus(taskID, store.StatusFailed,
				fmt.Sprintf("已达最大尝试次数 %d，仍未抢到资源", task.MaxAttempts))
			m.notifyGrab("⏹ 抢购任务已结束\n\n" + taskSummary(task) +
				fmt.Sprintf("\n已达最大尝试次数 %d，仍未抢到资源", task.MaxAttempts))
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

// capacityPrecheck 在真正创建实例之前做一次只读容量探测。
//
// 返回 shouldLaunch=true 表示可以发起 LaunchInstance；false 表示本轮确认无货，
// 调用方应直接等待下一轮，不要产生写调用。note 会写进任务日志，方便用户
// 看清「为什么这次没有尝试创建」。
//
// 容量报告接口对部分个人账户不可用（权限或区域限制），此时自动降级为
// 原来的直接创建逻辑，保证抢购功能不会因此失效。
func (m *Manager) capacityPrecheck(ctx context.Context, clients *oci.ClientSet, task *store.GrabTask) (bool, string) {
	value, err := m.store.GetSetting("capacity_precheck")
	if err == nil && value == "0" {
		return true, "容量预检已关闭，直接尝试创建实例"
	}

	account, err := m.store.GetAccount(task.AccountID)
	if err != nil {
		return true, "容量预检跳过（读取账号失败），直接尝试创建实例"
	}

	probeCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	items, err := oci.CheckCapacity(probeCtx, clients, oci.CapacityQuery{
		// 容量报告要求以根区间（Tenancy OCID）为作用域。
		CompartmentID:      account.TenancyOCID,
		AvailabilityDomain: task.ADName,
		Shape:              task.Shape,
		OCPUs:              float32(task.OCPUs),
		MemoryGBs:          float32(task.MemoryGBs),
	})
	if err != nil {
		if oci.IsCapacityReportUnsupported(err) {
			return true, "容量报告接口不可用（账户无权限或区域未开放），已降级为直接创建"
		}
		return true, "容量预检失败（" + err.Error() + "），本次改为直接创建"
	}

	if oci.AnyAvailable(items) {
		return true, fmt.Sprintf("容量预检：%s / %s 有库存，发起创建请求",
			task.ADName, task.Shape)
	}
	status := oci.CapacityUnknown
	if len(items) > 0 {
		status = items[0].AvailabilityStatus
	}
	return false, fmt.Sprintf("容量预检：%s / %s %s，跳过本次创建请求（未调用 LaunchInstance）",
		task.ADName, task.Shape, oci.StatusLabel(status))
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
