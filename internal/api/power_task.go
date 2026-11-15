package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/yourname/oci-manager/internal/store"
)

// powerTaskRequest 是定时任务的写入模型。
type powerTaskRequest struct {
	AccountID     string   `json:"account_id"`
	DisplayName   string   `json:"display_name"`
	Action        string   `json:"action"`
	TargetMode    string   `json:"target_mode"`
	InstanceIDs   []string `json:"instance_ids"`
	CompartmentID string   `json:"compartment_id"`
	DaysOfWeek    []int    `json:"days_of_week"`
	Hour          int      `json:"hour"`
	Minute        int      `json:"minute"`
	Enabled       bool     `json:"enabled"`
}

func (r powerTaskRequest) toInput() store.PowerTaskInput {
	return store.PowerTaskInput{
		DisplayName:   strings.TrimSpace(r.DisplayName),
		Action:        strings.TrimSpace(r.Action),
		TargetMode:    strings.TrimSpace(r.TargetMode),
		InstanceIDs:   r.InstanceIDs,
		CompartmentID: strings.TrimSpace(r.CompartmentID),
		DaysOfWeek:    r.DaysOfWeek,
		Hour:          r.Hour,
		Minute:        r.Minute,
		Enabled:       r.Enabled,
	}
}

// powerTaskSummary 生成可读摘要，写进审计 detail / 实时日志流。
func powerTaskSummary(in store.PowerTaskInput) string {
	action := map[string]string{
		store.PowerActionStart: "开机",
		store.PowerActionStop:  "关机",
		store.PowerActionReset: "重启",
	}[in.Action]
	if action == "" {
		action = in.Action
	}
	mode := "全部实例"
	if in.TargetMode == store.PowerTargetSelected {
		mode = fmt.Sprintf("%d 台指定实例", len(in.InstanceIDs))
	}
	days := "每天"
	if len(in.DaysOfWeek) > 0 {
		weekNames := []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}
		parts := make([]string, 0, len(in.DaysOfWeek))
		seen := map[int]bool{}
		for _, d := range store.ParsePowerTaskDOW(store.PowerTaskDOW(in.DaysOfWeek)) {
			if d >= 0 && d <= 6 && !seen[d] {
				seen[d] = true
				parts = append(parts, weekNames[d])
			}
		}
		days = strings.Join(parts, "、")
	}
	return fmt.Sprintf("%s · %s · %02d:%02d · %s", action, mode, in.Hour, in.Minute, days)
}

func (s *Server) listPowerTasks(c *gin.Context) {
	tasks, err := s.store.ListPowerTasks(c.Query("account_id"))
	if err != nil {
		respondError(c, err)
		return
	}
	respondOK(c, tasks)
}

func (s *Server) getPowerTask(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		respondError(c, err)
		return
	}
	task, err := s.store.GetPowerTask(id)
	if err != nil {
		respondError(c, err)
		return
	}
	respondOK(c, task)
}

func (s *Server) createPowerTask(c *gin.Context) {
	var req powerTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, err)
		return
	}
	if strings.TrimSpace(req.AccountID) == "" {
		respondError(c, fmt.Errorf("account_id 不能为空"))
		return
	}
	task, err := s.store.CreatePowerTask(strings.TrimSpace(req.AccountID), req.toInput())
	if err != nil {
		respondError(c, err)
		return
	}
	s.recordAudit(c, task.AccountID, "compute", "power_task_create", task.DisplayName,
		"ok", powerTaskSummary(req.toInput()))
	respondCreated(c, task)
}

func (s *Server) updatePowerTask(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		respondError(c, err)
		return
	}
	var req powerTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, err)
		return
	}
	task, err := s.store.UpdatePowerTask(id, req.toInput())
	if err != nil {
		respondError(c, err)
		return
	}
	s.recordAudit(c, task.AccountID, "compute", "power_task_update", task.DisplayName,
		"ok", powerTaskSummary(req.toInput()))
	respondOK(c, task)
}

func (s *Server) deletePowerTask(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		respondError(c, err)
		return
	}
	// 先取出摘要供审计留痕；任务不存在时仍保持 DELETE 的幂等语义（204）。
	task, getErr := s.store.GetPowerTask(id)
	if err := s.store.DeletePowerTask(id); err != nil {
		respondError(c, err)
		return
	}
	if getErr == nil {
		s.recordAudit(c, task.AccountID, "compute", "power_task_delete", task.DisplayName,
			"ok", "已删除定时任务")
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) togglePowerTask(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		respondError(c, err)
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, err)
		return
	}
	task, err := s.store.GetPowerTask(id)
	if err != nil {
		respondError(c, err)
		return
	}
	var instanceIDs []string
	if strings.TrimSpace(task.InstanceIDs) != "" {
		instanceIDs = strings.Split(task.InstanceIDs, ",")
	}
	input := store.PowerTaskInput{
		DisplayName:   task.DisplayName,
		Action:        task.Action,
		TargetMode:    task.TargetMode,
		InstanceIDs:   instanceIDs,
		CompartmentID: task.CompartmentID,
		DaysOfWeek:    store.ParsePowerTaskDOW(task.DaysOfWeek),
		Hour:          task.Hour,
		Minute:        task.Minute,
		Enabled:       req.Enabled,
	}
	updated, err := s.store.UpdatePowerTask(id, input)
	if err != nil {
		respondError(c, err)
		return
	}
	state := "已停用"
	if req.Enabled {
		state = "已启用"
	}
	s.recordAudit(c, updated.AccountID, "compute", "power_task_toggle", updated.DisplayName,
		"ok", fmt.Sprintf("%s定时任务（%s）", state, powerTaskSummary(input)))
	respondOK(c, updated)
}
