package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/yourname/oci-manager/internal/oci"
)

// capacityRequest 是容量探测的请求体。
// availability_domains 留空表示自动扫描该区域下的全部可用域。
type capacityRequest struct {
	Region              string   `json:"region"`
	Shape               string   `json:"shape"`
	OCPUs               float32  `json:"ocpus"`
	MemoryGBs           float32  `json:"memory_gbs"`
	AvailabilityDomains []string `json:"availability_domains"`
}

type capacityResponse struct {
	Region        string               `json:"region"`
	Shape         string               `json:"shape"`
	OCPUs         float32              `json:"ocpus"`
	MemoryGBs     float32              `json:"memory_gbs"`
	CompartmentID string               `json:"compartment_id"`
	Items         []oci.CapacityResult `json:"items"`
	CheckedAt     string               `json:"checked_at"`
	// Unsupported 为 true 表示该账户无法使用容量报告接口，前端应提示改用直接抢购。
	Unsupported bool   `json:"unsupported"`
	Hint        string `json:"hint,omitempty"`
}

// checkCapacity 探测指定账号、区域下若干可用域的机型库存。
//
// 使用 OCI Compute Capacity Report 只读接口，不创建实例、不占用配额。
func (s *Server) checkCapacity(c *gin.Context) {
	// 一次探测会对每个可用域发一次 OCI 只读请求，必须限流避免页面刷新放大调用量。
	if !s.rateLimit(c, "capacity", 8, time.Minute) {
		return
	}
	var body capacityRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, err)
		return
	}

	account, err := s.store.GetAccount(c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	cs, err := s.pool.For(account.ID, body.Region)
	if err != nil {
		respondError(c, err)
		return
	}

	shape := strings.TrimSpace(body.Shape)
	if shape == "" {
		shape = "VM.Standard.A1.Flex"
	}
	ocpus := body.OCPUs
	memory := body.MemoryGBs
	if ocpus <= 0 {
		ocpus = 4
	}
	if ocpus > 64 {
		ocpus = 64
	}
	if memory <= 0 {
		memory = 24
	}
	if memory > 512 {
		memory = 512
	}
	region := body.Region
	if region == "" {
		region = account.Region
	}

	// 容量报告 API 要求以根区间（Tenancy OCID）为作用域，这里始终使用租户 OCID。
	rootCompartment := account.TenancyOCID

	ctx, cancel := context.WithTimeout(c.Request.Context(), 90*time.Second)
	defer cancel()

	ads := body.AvailabilityDomains
	if len(ads) == 0 {
		ads, err = oci.ListAvailabilityDomains(ctx, cs, rootCompartment)
		if err != nil {
			respondError(c, fmt.Errorf("获取可用域失败: %s", oci.DescribeError(err)))
			return
		}
	}
	// 每个 AD 一次 OCI 调用，服务端必须有上限。
	if len(ads) > 10 {
		ads = ads[:10]
	}

	out := capacityResponse{
		Region:        region,
		Shape:         shape,
		OCPUs:         ocpus,
		MemoryGBs:     memory,
		CompartmentID: rootCompartment,
		Items:         make([]oci.CapacityResult, 0, len(ads)),
		CheckedAt:     time.Now().UTC().Format(time.RFC3339),
	}

	unsupportedCount := 0
	for _, ad := range ads {
		ad = strings.TrimSpace(ad)
		if ad == "" {
			continue
		}
		items, err := oci.CheckCapacity(ctx, cs, oci.CapacityQuery{
			CompartmentID:      rootCompartment,
			AvailabilityDomain: ad,
			Shape:              shape,
			OCPUs:              ocpus,
			MemoryGBs:          memory,
		})
		if err != nil {
			if oci.IsCapacityReportUnsupported(err) {
				unsupportedCount++
			}
			out.Items = append(out.Items, oci.CapacityResult{
				AvailabilityDomain: ad,
				Shape:              shape,
				OCPUs:              ocpus,
				MemoryGBs:          memory,
				AvailabilityStatus: oci.CapacityUnknown,
				Error:              sanitizeError(err.Error()),
			})
			continue
		}
		out.Items = append(out.Items, items...)
	}

	if unsupportedCount > 0 && unsupportedCount == len(ads) && len(ads) > 0 {
		out.Unsupported = true
		out.Hint = "该账户无权使用 OCI 容量报告接口（需要 tenancy 级读取权限，或该区域尚未开放）。" +
			"可关闭「抢购前容量预检」，改为直接尝试创建实例。"
	}
	respondOK(c, out)
}

// precheckSettings 返回抢购前的容量预检开关。
func (s *Server) getPrecheck(c *gin.Context) {
	value, err := s.store.GetSetting(settingCapacityPrecheck)
	if err != nil {
		respondError(c, err)
		return
	}
	enabled := value != "0" // 默认开启
	respondOK(c, gin.H{"enabled": enabled})
}

// savePrecheck 修改抢购前的容量预检开关。
func (s *Server) savePrecheck(c *gin.Context) {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, err)
		return
	}
	value := "1"
	if !body.Enabled {
		value = "0"
	}
	if err := s.store.SetSetting(settingCapacityPrecheck, value); err != nil {
		respondError(c, err)
		return
	}
	respondOK(c, gin.H{"enabled": body.Enabled})
}

const settingCapacityPrecheck = "capacity_precheck"
