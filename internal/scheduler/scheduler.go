// Package scheduler 实现定时开关机任务的调度。
//
// 调度精度到分钟：每 20 秒轮询一次，命中 (hour, minute, days_of_week) 且
// 该分钟窗口尚未触发过的任务会被执行一次。每个任务按账号独立获取客户端，
// 演示账号短路到内存模拟，绝不发出真实 OCI 请求。
package scheduler

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/yourname/oci-manager/internal/logbus"
	"github.com/yourname/oci-manager/internal/oci"
	"github.com/yourname/oci-manager/internal/store"
)

// Manager 持有调度所需的依赖并运行轮询循环。
type Manager struct {
	st   *store.Store
	pool *oci.Pool
}

// New 创建调度管理器。
func New(st *store.Store, pool *oci.Pool) *Manager {
	return &Manager{st: st, pool: pool}
}

// Start 启动后台轮询循环，直到 ctx 取消。
func (m *Manager) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		// 启动后先补齐一次，避免错过的整点任务（仅在服务刚重启且窗口未触发过时）。
		m.tick(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.tick(ctx)
			}
		}
	}()
}

func (m *Manager) tick(ctx context.Context) {
	now := time.Now()
	tasks, err := m.st.ListAllEnabledPowerTasks()
	if err != nil {
		log.Printf("读取定时任务失败: %v", err)
		return
	}
	for i := range tasks {
		t := &tasks[i]
		if !matchesWindow(t, now) {
			continue
		}
		if alreadyRanThisMinute(t, now) {
			continue
		}
		// 拷贝一份，避免闭包复用循环变量。
		task := *t
		go m.execute(ctx, &task, now)
	}
}

// matchesWindow 判断当前时间是否命中任务的 (hour, minute, days_of_week)。
func matchesWindow(t *store.PowerTask, now time.Time) bool {
	if now.Hour() != t.Hour || now.Minute() != t.Minute {
		return false
	}
	if t.DaysOfWeek == "*" || t.DaysOfWeek == "" {
		return true
	}
	want := now.Weekday()
	for _, d := range store.ParsePowerTaskDOW(t.DaysOfWeek) {
		if time.Weekday(d) == want {
			return true
		}
	}
	return false
}

// alreadyRanThisMinute 防止同一分钟窗口内重复触发（轮询周期 < 1 分钟）。
func alreadyRanThisMinute(t *store.PowerTask, now time.Time) bool {
	if t.LastRun == nil {
		return false
	}
	return t.LastRun.Year() == now.Year() &&
		t.LastRun.Month() == now.Month() &&
		t.LastRun.Day() == now.Day() &&
		t.LastRun.Hour() == now.Hour() &&
		t.LastRun.Minute() == now.Minute()
}

func (m *Manager) execute(ctx context.Context, t *store.PowerTask, when time.Time) {
	account, err := m.st.GetAccount(t.AccountID)
	if err != nil {
		_ = m.st.MarkPowerTaskRun(t.ID, "失败：读取账号失败 "+err.Error())
		logbus.Default.Publish(logbus.LevelError, logbus.SourceScheduler, t.AccountID,
			fmt.Sprintf("定时任务「%s」读取账号失败：%s", t.DisplayName, err.Error()))
		return
	}

	cs, err := m.pool.For(account.ID, account.Region)
	if err != nil {
		_ = m.st.MarkPowerTaskRun(t.ID, "失败：获取客户端失败 "+err.Error())
		logbus.Default.Publish(logbus.LevelError, logbus.SourceScheduler, t.AccountID,
			fmt.Sprintf("定时任务「%s」获取客户端失败：%s", t.DisplayName, err.Error()))
		return
	}

	targets, err := m.resolveTargets(ctx, cs, account, t)
	if err != nil {
		_ = m.st.MarkPowerTaskRun(t.ID, "失败："+err.Error())
		logbus.Default.Publish(logbus.LevelError, logbus.SourceScheduler, t.AccountID,
			fmt.Sprintf("定时任务「%s」解析目标失败：%s", t.DisplayName, err.Error()))
		return
	}
	if len(targets) == 0 {
		_ = m.st.MarkPowerTaskRun(t.ID, "跳过：没有匹配的实例")
		logbus.Default.Publish(logbus.LevelWarn, logbus.SourceScheduler, t.AccountID,
			fmt.Sprintf("定时任务「%s」未匹配到实例，已跳过", t.DisplayName))
		return
	}

	var succeeded, failed int
	var errs []string
	for _, inst := range targets {
		if err := oci.InstanceAction(ctx, cs, inst.ID, t.Action); err != nil {
			failed++
			errs = append(errs, fmt.Sprintf("%s: %s", inst.DisplayName, err.Error()))
			continue
		}
		succeeded++
	}

	actionLabel := map[string]string{
		store.PowerActionStart: "启动",
		store.PowerActionStop:  "停止",
		store.PowerActionReset: "重启",
	}[t.Action]
	result := fmt.Sprintf("%s %d 台，失败 %d 台", actionLabel, succeeded, failed)
	if len(errs) > 0 {
		result += "；错误：" + strings.Join(errs, "；")
	}
	_ = m.st.MarkPowerTaskRun(t.ID, result)
	level := logbus.LevelSuccess
	if failed > 0 && succeeded == 0 {
		level = logbus.LevelError
	} else if failed > 0 {
		level = logbus.LevelWarn
	}
	logbus.Default.Publish(level, logbus.SourceScheduler, t.AccountID,
		fmt.Sprintf("定时任务「%s」执行%s：成功 %d / 失败 %d 台", t.DisplayName, actionLabel, succeeded, failed))
}

// resolveTargets 返回本次要操作的实例列表。
func (m *Manager) resolveTargets(ctx context.Context, cs *oci.ClientSet, account *store.Account, t *store.PowerTask) ([]oci.Instance, error) {
	var instances []oci.Instance
	if t.TargetMode == store.PowerTargetAll {
		comps, err := oci.ListCompartments(ctx, cs, account.TenancyOCID)
		if err != nil {
			return nil, err
		}
		for _, comp := range comps {
			list, lerr := oci.ListInstances(ctx, cs, comp.ID)
			if lerr != nil {
				return nil, lerr
			}
			instances = append(instances, list...)
		}
		return instances, nil
	}

	// selected：按 instance_ids 精确匹配，跨区间查找。
	want := map[string]struct{}{}
	for _, id := range strings.Split(t.InstanceIDs, ",") {
		id = strings.TrimSpace(id)
		if id != "" {
			want[id] = struct{}{}
		}
	}
	if len(want) == 0 {
		return nil, nil
	}
	comps, err := oci.ListCompartments(ctx, cs, account.TenancyOCID)
	if err != nil {
		return nil, err
	}
	for _, comp := range comps {
		list, lerr := oci.ListInstances(ctx, cs, comp.ID)
		if lerr != nil {
			return nil, lerr
		}
		for _, inst := range list {
			if _, ok := want[inst.ID]; ok {
				instances = append(instances, inst)
			}
		}
	}
	return instances, nil
}
