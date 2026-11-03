package api

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/yourname/oci-manager/internal/oci"
	"github.com/yourname/oci-manager/internal/store"
)

// requestTimeout 是单个 OCI 调用的默认超时。
const requestTimeout = 60 * time.Second

// accountAndClients 解析路径中的账号，并取对应区域的客户端。
// region 为空表示使用账号的默认区域。
func (s *Server) accountAndClients(c *gin.Context) (*store.Account, *oci.ClientSet, bool) {
	account, err := s.store.GetAccount(c.Param("id"))
	if err != nil {
		respondError(c, err)
		return nil, nil, false
	}
	cs, err := s.pool.For(account.ID, c.Query("region"))
	if err != nil {
		respondError(c, err)
		return nil, nil, false
	}
	return account, cs, true
}

// compartmentOf 取区间 OCID，未指定时回落到根区间（租户本身）。
func compartmentOf(c *gin.Context, account *store.Account) string {
	if v := strings.TrimSpace(c.Query("compartment_id")); v != "" {
		return v
	}
	return account.TenancyOCID
}

func (s *Server) listRegions(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()

	account, cs, ok := s.accountAndClients(c)
	if !ok {
		return
	}
	regions, err := oci.ListRegions(ctx, cs, account.TenancyOCID)
	if err != nil {
		respondError(c, err)
		return
	}
	respondOK(c, regions)
}

func (s *Server) listCompartments(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()

	account, cs, ok := s.accountAndClients(c)
	if !ok {
		return
	}
	compartments, err := oci.ListCompartments(ctx, cs, account.TenancyOCID)
	if err != nil {
		respondError(c, err)
		return
	}
	respondOK(c, compartments)
}

func (s *Server) listAvailabilityDomains(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()

	account, cs, ok := s.accountAndClients(c)
	if !ok {
		return
	}
	ads, err := oci.ListAvailabilityDomains(ctx, cs, compartmentOf(c, account))
	if err != nil {
		respondError(c, err)
		return
	}
	respondOK(c, ads)
}

func (s *Server) listSubnets(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()

	account, cs, ok := s.accountAndClients(c)
	if !ok {
		return
	}
	subnets, err := oci.ListSubnets(ctx, cs, compartmentOf(c, account))
	if err != nil {
		respondError(c, err)
		return
	}
	respondOK(c, subnets)
}

func (s *Server) listImages(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()

	account, cs, ok := s.accountAndClients(c)
	if !ok {
		return
	}
	images, err := oci.ListImages(ctx, cs, compartmentOf(c, account))
	if err != nil {
		respondError(c, err)
		return
	}
	respondOK(c, images)
}

func (s *Server) listInstances(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()

	account, cs, ok := s.accountAndClients(c)
	if !ok {
		return
	}
	instances, err := oci.ListInstances(ctx, cs, compartmentOf(c, account))
	if err != nil {
		respondError(c, err)
		return
	}
	respondOK(c, instances)
}

func (s *Server) instanceAction(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()

	var body struct {
		Action string `json:"action"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, err)
		return
	}
	_, cs, ok := s.accountAndClients(c)
	if !ok {
		return
	}
	if err := oci.InstanceAction(ctx, cs, c.Param("instanceId"), body.Action); err != nil {
		respondError(c, err)
		return
	}
	respondOK(c, gin.H{"status": "ok"})
}

func (s *Server) updateInstanceShape(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()

	var body struct {
		OCPUs     float32 `json:"ocpus"`
		MemoryGBs float32 `json:"memory_gbs"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, err)
		return
	}
	_, cs, ok := s.accountAndClients(c)
	if !ok {
		return
	}
	if err := oci.UpdateInstanceShape(ctx, cs, c.Param("instanceId"), body.OCPUs, body.MemoryGBs); err != nil {
		respondError(c, err)
		return
	}
	respondOK(c, gin.H{"status": "ok"})
}

func (s *Server) terminateInstance(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()

	preserve := c.DefaultQuery("preserve_boot_volume", "true") == "true"
	_, cs, ok := s.accountAndClients(c)
	if !ok {
		return
	}
	if err := oci.TerminateInstance(ctx, cs, c.Param("instanceId"), preserve); err != nil {
		respondError(c, err)
		return
	}
	respondOK(c, gin.H{"status": "ok"})
}

func (s *Server) queryCost(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()

	account, cs, ok := s.accountAndClients(c)
	if !ok {
		return
	}
	start, end, err := parseRange(c.Query("start"), c.Query("end"))
	if err != nil {
		respondError(c, err)
		return
	}
	groupBy := []string{}
	if raw := strings.TrimSpace(c.Query("group_by")); raw != "" {
		for _, g := range strings.Split(raw, ",") {
			if g = strings.TrimSpace(g); g != "" {
				groupBy = append(groupBy, g)
			}
		}
	}
	if len(groupBy) == 0 {
		groupBy = []string{"service"}
	}

	points, err := oci.QueryCost(ctx, cs, oci.UsageQuery{
		TenancyID:   account.TenancyOCID,
		Start:       start,
		End:         end,
		Granularity: c.DefaultQuery("granularity", "daily"),
		GroupBy:     groupBy,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	respondOK(c, points)
}

// overview 汇总账号的资源与成本，用于总览页。
// 单个子项失败不阻断整体返回，前端据此灰度展示。
func (s *Server) overview(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 90*time.Second)
	defer cancel()

	account, cs, ok := s.accountAndClients(c)
	if !ok {
		return
	}

	result := gin.H{
		"account_id": account.ID,
		"region":     cs.Region,
	}

	compartments, err := oci.ListCompartments(ctx, cs, account.TenancyOCID)
	if err != nil {
		result["compartment_error"] = err.Error()
		compartments = nil
	}

	instances := make([]oci.Instance, 0)
	for _, comp := range compartments {
		list, err := oci.ListInstances(ctx, cs, comp.ID)
		if err != nil {
			continue
		}
		instances = append(instances, list...)
	}

	running := 0
	totalOCPUs := float32(0)
	totalMemory := float32(0)
	for _, inst := range instances {
		if inst.State == "RUNNING" {
			running++
			totalOCPUs += inst.OCPUs
			totalMemory += inst.MemoryGBs
		}
	}

	monthStart := time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.UTC)
	points, costErr := oci.QueryCost(ctx, cs, oci.UsageQuery{
		TenancyID:   account.TenancyOCID,
		Start:       monthStart,
		End:         time.Now().UTC(),
		Granularity: "monthly",
		GroupBy:     []string{"service"},
	})
	var monthCost float32
	currency := ""
	if costErr == nil {
		for _, p := range points {
			monthCost += p.Cost
			if p.Currency != "" {
				currency = p.Currency
			}
		}
	} else {
		result["cost_error"] = costErr.Error()
	}

	result["tenancy_ocid"] = account.TenancyOCID
	result["compartment_count"] = len(compartments)
	result["instance_count"] = len(instances)
	result["running_count"] = running
	result["total_ocpus"] = totalOCPUs
	result["total_memory_gbs"] = totalMemory
	result["month_cost"] = monthCost
	result["currency"] = currency
	respondOK(c, result)
}

// parseRange 解析查询区间，缺失时默认返回最近 30 天。
func parseRange(startStr, endStr string) (time.Time, time.Time, error) {
	now := time.Now().UTC()
	end := now
	start := now.AddDate(0, 0, -30)

	if strings.TrimSpace(endStr) != "" {
		parsed, err := time.Parse("2006-01-02", strings.TrimSpace(endStr))
		if err != nil {
			return start, end, errDate("结束日期格式应为 YYYY-MM-DD")
		}
		end = parsed
	}
	if strings.TrimSpace(startStr) != "" {
		parsed, err := time.Parse("2006-01-02", strings.TrimSpace(startStr))
		if err != nil {
			return start, end, errDate("开始日期格式应为 YYYY-MM-DD")
		}
		start = parsed
	}
	if end.Before(start) {
		return start, end, errDate("结束日期不能早于开始日期")
	}
	return start, end, nil
}

type dateError struct{ msg string }

func (e *dateError) Error() string { return e.msg }

func errDate(msg string) error { return &dateError{msg} }
