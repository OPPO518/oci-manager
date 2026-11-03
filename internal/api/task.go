package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/yourname/oci-manager/internal/store"
)

// grabTaskRequest 是抢购任务的写入模型。
type grabTaskRequest struct {
	AccountID     string `json:"account_id"`
	DisplayName   string `json:"display_name"`
	Region        string `json:"region"`
	CompartmentID string `json:"compartment_id"`
	ADName        string `json:"ad_name"`
	Shape         string `json:"shape"`
	OCPUs         int    `json:"ocpus"`
	MemoryGBs     int    `json:"memory_gbs"`
	ImageID       string `json:"image_id"`
	SubnetID      string `json:"subnet_id"`
	SSHPublicKey  string `json:"ssh_public_key"`
	BootVolumeGBs int    `json:"boot_volume_gbs"`
	InstanceCount int    `json:"instance_count"`
	IntervalSec   int    `json:"interval_sec"`
	MaxAttempts   int    `json:"max_attempts"`
}

func (r grabTaskRequest) toInput() store.GrabTaskInput {
	return store.GrabTaskInput{
		AccountID:     strings.TrimSpace(r.AccountID),
		DisplayName:   strings.TrimSpace(r.DisplayName),
		Region:        strings.TrimSpace(r.Region),
		CompartmentID: strings.TrimSpace(r.CompartmentID),
		ADName:        strings.TrimSpace(r.ADName),
		Shape:         strings.TrimSpace(r.Shape),
		OCPUs:         r.OCPUs,
		MemoryGBs:     r.MemoryGBs,
		ImageID:       strings.TrimSpace(r.ImageID),
		SubnetID:      strings.TrimSpace(r.SubnetID),
		SSHPublicKey:  strings.TrimSpace(r.SSHPublicKey),
		BootVolumeGBs: r.BootVolumeGBs,
		InstanceCount: r.InstanceCount,
		IntervalSec:   r.IntervalSec,
		MaxAttempts:   r.MaxAttempts,
	}
}

func (s *Server) listGrabTasks(c *gin.Context) {
	tasks, err := s.store.ListGrabTasks(c.Query("account_id"))
	if err != nil {
		respondError(c, err)
		return
	}
	// 补上运行状态，前端据此决定启停按钮
	for i := range tasks {
		if s.grabber.IsRunning(tasks[i].ID) {
			tasks[i].Status = store.StatusRunning
		}
	}
	respondOK(c, tasks)
}

func (s *Server) getGrabTask(c *gin.Context) {
	task, err := s.store.GetGrabTask(c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	if s.grabber.IsRunning(task.ID) {
		task.Status = store.StatusRunning
	}
	respondOK(c, task)
}

func (s *Server) createGrabTask(c *gin.Context) {
	var req grabTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, err)
		return
	}
	task, err := s.store.CreateGrabTask(req.toInput())
	if err != nil {
		respondError(c, err)
		return
	}
	respondCreated(c, task)
}

func (s *Server) deleteGrabTask(c *gin.Context) {
	id := c.Param("id")
	if err := s.grabber.Stop(id); err != nil {
		respondError(c, err)
		return
	}
	if err := s.store.DeleteGrabTask(id); err != nil {
		respondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) startGrabTask(c *gin.Context) {
	if err := s.grabber.Start(c.Param("id")); err != nil {
		respondError(c, err)
		return
	}
	respondOK(c, gin.H{"status": "running"})
}

func (s *Server) stopGrabTask(c *gin.Context) {
	if err := s.grabber.Stop(c.Param("id")); err != nil {
		respondError(c, err)
		return
	}
	respondOK(c, gin.H{"status": "stopped"})
}

// grabTaskLogs 返回任务日志，支持 after 增量拉取。
func (s *Server) grabTaskLogs(c *gin.Context) {
	after, _ := strconv.Atoi(c.DefaultQuery("after", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "200"))
	logs, err := s.store.ListLogs(c.Param("id"), after, limit)
	if err != nil {
		respondError(c, err)
		return
	}
	respondOK(c, logs)
}
