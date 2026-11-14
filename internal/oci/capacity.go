package oci

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/oracle/oci-go-sdk/v65/core"
)

// 容量状态常量，直接对应 OCI Compute Capacity Report 的返回值。
const (
	CapacityAvailable            = "AVAILABLE"
	CapacityOutOfHostCapacity    = "OUT_OF_HOST_CAPACITY"
	CapacityHardwareNotSupported = "HARDWARE_NOT_SUPPORTED"
	CapacityUnknown              = "UNKNOWN"
)

// CapacityQuery 是一次容量探测的查询条件。
//
// CompartmentID 必须是根区间（即 Tenancy OCID），OCI 文档明确要求容量报告
// 始终以根区间为作用域；传子区间会得到无意义的结果或直接报错。
// AvailabilityDomain 一次只能查一个，多个可用域由调用方循环发起。
type CapacityQuery struct {
	CompartmentID      string
	AvailabilityDomain string
	Shape              string
	OCPUs              float32
	MemoryGBs          float32
	FaultDomain        string // 留空表示按该可用域下全部故障域汇总
}

// CapacityResult 是单个「可用域 / 故障域」组合的探测结果。
type CapacityResult struct {
	AvailabilityDomain string  `json:"availability_domain"`
	FaultDomain        string  `json:"fault_domain"`
	Shape              string  `json:"shape"`
	OCPUs              float32 `json:"ocpus"`
	MemoryGBs          float32 `json:"memory_gbs"`
	AvailabilityStatus string  `json:"availability_status"`
	Available          bool    `json:"available"`
	// AvailableCount 仅对 DRCC 客户与白名单租户返回，普通账户通常为 0。
	// 因此判断有货一律以 AvailabilityStatus 为准，不要用这个字段。
	AvailableCount *int64 `json:"available_count"`
	TimeCreated    string `json:"time_created"`
	Error          string `json:"error,omitempty"`
}

// CheckCapacity 调用 OCI Compute Capacity Report API 查询单个可用域的机型余量。
//
// 这是只读接口，不会创建实例、不占用配额，也不会产生 LaunchInstance 那种
// 高风险写调用。用它做抢购前的预检，可以把大量注定失败的创建请求过滤掉，
// 从而显著降低触发风控的概率。
func CheckCapacity(ctx context.Context, cs *ClientSet, q CapacityQuery) ([]CapacityResult, error) {
	if cs.IsDemo() {
		if strings.TrimSpace(q.Shape) == "" {
			q.Shape = "VM.Standard.A1.Flex"
		}
		return cs.Demo.checkCapacity(q), nil
	}
	shape := strings.TrimSpace(q.Shape)
	if shape == "" {
		shape = "VM.Standard.A1.Flex"
	}
	compartment := q.CompartmentID
	ad := q.AvailabilityDomain

	shapeAvailability := core.CreateCapacityReportShapeAvailabilityDetails{
		InstanceShape: &shape,
	}
	if q.FaultDomain != "" {
		fd := q.FaultDomain
		shapeAvailability.FaultDomain = &fd
	}
	// Flex 机型必须带上规格，否则 OCI 无法判断具体容量。
	if q.OCPUs > 0 || q.MemoryGBs > 0 {
		ocpus := q.OCPUs
		memory := q.MemoryGBs
		shapeAvailability.InstanceShapeConfig = &core.CapacityReportInstanceShapeConfig{
			Ocpus:       &ocpus,
			MemoryInGBs: &memory,
		}
	}

	resp, err := cs.Compute.CreateComputeCapacityReport(ctx, core.CreateComputeCapacityReportRequest{
		CreateComputeCapacityReportDetails: core.CreateComputeCapacityReportDetails{
			CompartmentId:      &compartment,
			AvailabilityDomain: &ad,
			ShapeAvailabilities: []core.CreateCapacityReportShapeAvailabilityDetails{
				shapeAvailability,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("容量报告查询失败: %s", DescribeError(err))
	}

	created := ""
	if resp.TimeCreated != nil {
		created = resp.TimeCreated.Time.UTC().Format(time.RFC3339)
	}

	items := make([]CapacityResult, 0, len(resp.ShapeAvailabilities))
	for _, sa := range resp.ShapeAvailabilities {
		item := CapacityResult{
			AvailabilityDomain: ad,
			Shape:              shape,
			OCPUs:              q.OCPUs,
			MemoryGBs:          q.MemoryGBs,
			AvailabilityStatus: CapacityUnknown,
			TimeCreated:        created,
		}
		if sa.FaultDomain != nil {
			item.FaultDomain = *sa.FaultDomain
		}
		if sa.InstanceShape != nil && *sa.InstanceShape != "" {
			item.Shape = *sa.InstanceShape
		}
		if sa.InstanceShapeConfig != nil {
			if sa.InstanceShapeConfig.Ocpus != nil {
				item.OCPUs = *sa.InstanceShapeConfig.Ocpus
			}
			if sa.InstanceShapeConfig.MemoryInGBs != nil {
				item.MemoryGBs = *sa.InstanceShapeConfig.MemoryInGBs
			}
		}
		status := string(sa.AvailabilityStatus)
		if status == "" {
			// 少数情况下 OCI 只返回 availableCount 而不带 status。
			// 普通账户拿不到 count，这里仍以 UNKNOWN 处理，避免误判为有货。
			status = CapacityUnknown
		}
		item.AvailabilityStatus = status
		item.Available = status == CapacityAvailable
		item.AvailableCount = sa.AvailableCount
		items = append(items, item)
	}
	if len(items) == 0 {
		items = append(items, CapacityResult{
			AvailabilityDomain: ad,
			Shape:              shape,
			OCPUs:              q.OCPUs,
			MemoryGBs:          q.MemoryGBs,
			AvailabilityStatus: CapacityUnknown,
			TimeCreated:        created,
		})
	}
	return items, nil
}

// IsCapacityReportUnsupported 判断错误是否说明该账户无法使用容量报告接口。
// 常见原因：租户未被授权、区域不支持、旧版本 API 端点不可用。
// 遇到这类错误时抢购流程应自动降级为直接创建，而不是终止任务。
func IsCapacityReportUnsupported(err error) bool {
	if err == nil {
		return false
	}
	// 限流/5xx/超时绝不能算「接口不支持」，否则会退化成每轮直接创建。
	if IsRateLimited(err) || IsTransientError(err) {
		return false
	}
	msg := strings.ToLower(err.Error())
	// markers 必须窄：过去 "not found"、"invalid parameter"、"compartment"
	// 会把参数错误和网络错误误判为不支持，从而关掉预检、放大写请求。
	markers := []string{
		"not authorized", "notauthorized", "authorization failed",
		"403", "404", "501", "notfound",
		"not supported", "notsupported",
		// SDK 的 service code 是驼峰连写，例如 UnknownOperation / NotSupported，
		// 必须同时匹配连写形式，否则会被误判成暂时错误而一直退避。
		"unknown operation", "unknownoperation",
		"capacity report",
	}
	for _, m := range markers {
		if strings.Contains(msg, m) {
			return true
		}
	}
	return false
}

// AnyAvailable 判断一组结果里是否存在「有货」的项。
func AnyAvailable(items []CapacityResult) bool {
	for _, it := range items {
		if it.Available {
			return true
		}
	}
	return false
}

// StatusLabel 把 OCI 状态翻译成中文，供日志与通知使用。
func StatusLabel(status string) string {
	switch status {
	case CapacityAvailable:
		return "有库存"
	case CapacityOutOfHostCapacity:
		return "无库存（主机容量不足）"
	case CapacityHardwareNotSupported:
		return "该区域尚未部署对应硬件"
	default:
		return "状态未知"
	}
}
