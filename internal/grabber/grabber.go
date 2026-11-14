// Package grabber 实现 ARM 实例抢购任务的调度。
//
// Oracle Cloud 的 Always Free ARM 资源（Ampere A1）长期处于超售状态，
// 创建实例时经常返回 "Out of host capacity"。本包通过定时重试来抢占
// 偶发释放的资源：只有容量不足才继续重试，其他错误一律终止任务，
// 避免因为配置错误（例如子网或镜像填错）而无限循环。
//
// 风控策略（重要）：
//   - 每轮先用只读的 Compute Capacity Report 预判，没货就不发任何写请求；
//   - OCI 限流(429)、5xx、超时一律「退避暂停」，绝不降级成高频直接创建；
//   - 所有 OCI 调用经过全局节流，参数在服务端强制 clamp，避免多个任务叠加。
package grabber

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/yourname/oci-manager/internal/oci"
	"github.com/yourname/oci-manager/internal/store"
)

// 服务端强制的调用频率与数量下限。前端的 min 属性可以绕过，必须在这里兜底。
const (
	minIntervalSec         = 30
	maxIntervalSec         = 3600
	defaultIntervalSec     = 60
	fallbackMinIntervalSec = 120 // 降级为直接创建后，间隔不能低于 2 分钟
	maxInstancesPerTask    = 10
	minAPIGap              = 2 * time.Second // 全局 OCI 调用最小间隔
	maxBackoff             = 15 * time.Minute
)

// Notifier 用于在抢购任务状态变化时推送通知。
type Notifier interface {
	// GrabEventEnabled 返回是否开启了抢购事件通知。
	GrabEventEnabled() bool
	// Send 推送一条消息。
	Send(text string) error
}

// precheckDecision 是容量预检的结论。
type precheckDecision int

const (
	decisionLaunch   precheckDecision = iota // 有货/预检关闭：可以创建
	decisionSkip                             // 确认无货：等下一轮，不发写请求
	decisionBackoff                          // 限流/网络/服务端错误：指数退避，绝不直创
	decisionFallback                         // 接口确实不可用：降级直创，但拉长间隔
)

// Manager 管理抢购任务的生命周期。
type Manager struct {
	store  *store.Store
	pool   *oci.Pool
	notify Notifier

	mu      sync.Mutex
	running map[string]*taskHandle

	rateMu      sync.Mutex
	lastAPICall time.Time
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

// reserveSlot 给所有 OCI 调用做全局节流，避免多个任务同时轮询形成请求脉冲。
// 采用「预约时间点」而不是简单 sleep，防止并发 goroutine 一起通过检查。
func (m *Manager) reserveSlot(ctx context.Context) error {
	for {
		m.rateMu.Lock()
		now := time.Now()
		next := m.lastAPICall.Add(minAPIGap)
		if !now.Before(next) {
			m.lastAPICall = now
			m.rateMu.Unlock()
			return nil
		}
		m.lastAPICall = next
		wait := next.Sub(now)
		m.rateMu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
			return nil
		}
	}
}

// taskIntervalSec 把轮询间隔收敛到安全区间。数据库中历史任务即使写着 1 秒，
// 运行时也不会低于 30 秒。
func taskIntervalSec(t *store.GrabTask) int {
	v := t.IntervalSec
	if v <= 0 {
		v = defaultIntervalSec
	}
	if v < minIntervalSec {
		v = minIntervalSec
	}
	if v > maxIntervalSec {
		v = maxIntervalSec
	}
	return v
}

// backoffDuration 指数退避 + 抖动，避免多任务同步重试。
func backoffDuration(failures int) time.Duration {
	d := 30 * time.Second
	for i := 1; i < failures && d < maxBackoff; i++ {
		d *= 2
	}
	if d > maxBackoff {
		d = maxBackoff
	}
	return d + time.Duration(rand.Int63n(int64(d/5)+1))
}

func sleepOrDone(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		d = time.Second
	}
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
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
	_ = m.store.AppendLog(taskID, "info", fmt.Sprintf("任务启动：%s / %s / %d OCPU / %d GB 内存（轮询间隔 %d 秒）",
		task.Shape, task.ADName, task.OCPUs, task.MemoryGBs, taskIntervalSec(task)))

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
	// precheckAvailable 为 false 表示容量报告接口确实不可用，已降级为直接创建。
	// 降级后仍保持长间隔，且不会因一次网络抖动就永久关闭预检。
	precheckAvailable := true
	transientFailures := 0

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

		interval := time.Duration(taskIntervalSec(task)) * time.Second
		if !precheckAvailable && interval < fallbackMinIntervalSec*time.Second {
			interval = fallbackMinIntervalSec * time.Second
		}

		// ---- 抢购前容量预检（只读，不创建实例）----
		if precheckAvailable {
			if err := m.reserveSlot(ctx); err != nil {
				return
			}
			decision, note := m.capacityPrecheck(ctx, clients, task)
			_ = m.store.AppendLog(taskID, "info", note)
			switch decision {
			case decisionSkip:
				transientFailures = 0
				if !sleepOrDone(ctx, interval) {
					return
				}
				continue
			case decisionBackoff:
				transientFailures++
				wait := backoffDuration(transientFailures)
				_ = m.store.AppendLog(taskID, "warn",
					fmt.Sprintf("预检暂时不可用，退避 %s 后重试（连续 %d 次，本轮未调用创建接口）", wait, transientFailures))
				if !sleepOrDone(ctx, wait) {
					return
				}
				continue
			case decisionFallback:
				precheckAvailable = false
				interval = fallbackMinIntervalSec * time.Second
				_ = m.store.AppendLog(taskID, "warn", "已降级为直接创建，轮询间隔自动拉长到 2 分钟以降低风控风险")
			}
			transientFailures = 0
		}

		attempt := task.Attempts + 1
		_ = m.store.AppendLog(taskID, "info", fmt.Sprintf("第 %d 次尝试创建实例", attempt))
		if err := m.reserveSlot(ctx); err != nil {
			return
		}
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

		// 限流/暂时错误必须退避暂停，绝不能当作「容量不足」继续高频重试。
		if oci.IsRateLimited(launchErr) || oci.IsTransientError(launchErr) {
			transientFailures++
			wait := backoffDuration(transientFailures)
			_ = m.store.AppendLog(taskID, "warn",
				fmt.Sprintf("OCI 限流或暂时不可用，退避 %s 后重试（连续 %d 次）", wait, transientFailures))
			if !sleepOrDone(ctx, wait) {
				return
			}
			continue
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

		if !sleepOrDone(ctx, interval) {
			return
		}
	}
}

// capacityPrecheck 在真正创建实例之前做一次只读容量探测。
//
// 关键：只有在「接口确实不被账户支持」时才降级直创。
// 429 / 5xx / 超时 / 网络抖动一律返回 decisionBackoff，
// 否则 OCI 一开始限流，系统反而会关掉预检、按最短间隔狂发 LaunchInstance。
func (m *Manager) capacityPrecheck(ctx context.Context, clients *oci.ClientSet, task *store.GrabTask) (precheckDecision, string) {
	value, err := m.store.GetSetting("capacity_precheck")
	if err == nil && value == "0" {
		return decisionLaunch, "容量预检已关闭，直接尝试创建实例"
	}

	account, err := m.store.GetAccount(task.AccountID)
	if err != nil {
		return decisionBackoff, "容量预检跳过（读取账号失败），本轮不创建实例"
	}

	probeCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	items, err := oci.CheckCapacity(probeCtx, clients, oci.CapacityQuery{
		CompartmentID:      account.TenancyOCID,
		AvailabilityDomain: task.ADName,
		Shape:              task.Shape,
		OCPUs:              float32(task.OCPUs),
		MemoryGBs:          float32(task.MemoryGBs),
	})
	if err != nil {
		if oci.IsRateLimited(err) || oci.IsTransientError(err) {
			return decisionBackoff, "容量预检遇到限流或暂时错误，本轮不创建实例，稍后指数退避重试"
		}
		if oci.IsCapacityReportUnsupported(err) {
			return decisionFallback, "容量报告接口不可用（账户无权限或区域未开放），已降级为直接创建"
		}
		return decisionBackoff, "容量预检失败（" + oci.DescribeError(err) + "），本轮不创建实例"
	}

	if oci.AnyAvailable(items) {
		return decisionLaunch, fmt.Sprintf("容量预检：%s / %s 有库存，发起创建请求",
			task.ADName, task.Shape)
	}
	status := oci.CapacityUnknown
	if len(items) > 0 {
		status = items[0].AvailabilityStatus
	}
	return decisionSkip, fmt.Sprintf("容量预检：%s / %s %s，跳过本次创建请求（未调用 LaunchInstance）",
		task.ADName, task.Shape, oci.StatusLabel(status))
}

// launchAll 按任务要求创建指定数量的实例，全部成功才返回 nil。
// 已创建成功的实例会保留，不会回滚。
func (m *Manager) launchAll(ctx context.Context, clients *oci.ClientSet, task *store.GrabTask) ([]string, error) {
	count := task.InstanceCount
	if count <= 0 {
		count = 1
	}
	if count > maxInstancesPerTask {
		count = maxInstancesPerTask
	}
	created := make([]string, 0, count)
	for i := 0; i < count; i++ {
		if i > 0 {
			if err := m.reserveSlot(ctx); err != nil {
				return created, err
			}
		}
		name := task.DisplayName
		if count > 1 {
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
