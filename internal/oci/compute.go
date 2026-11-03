package oci

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/oracle/oci-go-sdk/v65/core"
)

// Instance 是面向前端的实例视图，屏蔽 SDK 的指针字段。
type Instance struct {
	ID                 string     `json:"id"`
	DisplayName        string     `json:"display_name"`
	State              string     `json:"state"`
	Shape              string     `json:"shape"`
	OCPUs              float32    `json:"ocpus"`
	MemoryGBs          float32    `json:"memory_gbs"`
	AvailabilityDomain string     `json:"availability_domain"`
	CompartmentID      string     `json:"compartment_id"`
	Region             string     `json:"region"`
	ImageID            string     `json:"image_id"`
	TimeCreated        *time.Time `json:"time_created"`
}

// LaunchParams 是创建实例的参数集合。
type LaunchParams struct {
	DisplayName        string
	CompartmentID      string
	AvailabilityDomain string
	Shape              string
	OCPUs              float32
	MemoryGBs          float32
	ImageID            string
	SubnetID           string
	SSHPublicKey       string
	BootVolumeGBs      int64
	AssignPublicIP     bool
}

// ListInstances 列出区间下的全部计算实例。
func ListInstances(ctx context.Context, cs *ClientSet, compartmentID string) ([]Instance, error) {
	var (
		instances []Instance
		page      *string
	)
	for {
		resp, err := cs.Compute.ListInstances(ctx, core.ListInstancesRequest{
			CompartmentId: &compartmentID,
			Page:          page,
			Limit:         intPtr(500),
		})
		if err != nil {
			return nil, wrap("列出实例失败", err)
		}
		for _, item := range resp.Items {
			instances = append(instances, convertInstance(item))
		}
		if resp.OpcNextPage == nil || *resp.OpcNextPage == "" {
			break
		}
		page = resp.OpcNextPage
	}
	return instances, nil
}

func convertInstance(item core.Instance) Instance {
	out := Instance{State: string(item.LifecycleState)}
	if item.Id != nil {
		out.ID = *item.Id
	}
	if item.DisplayName != nil {
		out.DisplayName = *item.DisplayName
	}
	if item.Shape != nil {
		out.Shape = *item.Shape
	}
	if item.AvailabilityDomain != nil {
		out.AvailabilityDomain = *item.AvailabilityDomain
	}
	if item.CompartmentId != nil {
		out.CompartmentID = *item.CompartmentId
	}
	if item.Region != nil {
		out.Region = *item.Region
	}
	if item.ImageId != nil {
		out.ImageID = *item.ImageId
	}
	if item.ShapeConfig != nil {
		if item.ShapeConfig.Ocpus != nil {
			out.OCPUs = *item.ShapeConfig.Ocpus
		}
		if item.ShapeConfig.MemoryInGBs != nil {
			out.MemoryGBs = *item.ShapeConfig.MemoryInGBs
		}
	}
	if item.TimeCreated != nil {
		t := item.TimeCreated.Time
		out.TimeCreated = &t
	}
	return out
}

// InstanceAction 对实例执行电源类操作。
// action 取值为 start / stop / softstop / reset / softreset。
func InstanceAction(ctx context.Context, cs *ClientSet, instanceID, action string) error {
	parsed, err := parseInstanceAction(action)
	if err != nil {
		return err
	}
	_, err = cs.Compute.InstanceAction(ctx, core.InstanceActionRequest{
		InstanceId: &instanceID,
		Action:     parsed,
	})
	if err != nil {
		return wrap(fmt.Sprintf("执行实例操作 %s 失败", action), err)
	}
	return nil
}

func parseInstanceAction(action string) (core.InstanceActionActionEnum, error) {
	switch strings.ToUpper(strings.TrimSpace(action)) {
	case "START":
		return core.InstanceActionActionStart, nil
	case "STOP":
		return core.InstanceActionActionStop, nil
	case "SOFTSTOP":
		return core.InstanceActionActionSoftstop, nil
	case "RESET":
		return core.InstanceActionActionReset, nil
	case "SOFTRESET":
		return core.InstanceActionActionSoftreset, nil
	default:
		return "", fmt.Errorf("不支持的实例操作 %q，可选值：start/stop/softstop/reset/softreset", action)
	}
}

// UpdateInstanceShape 调整实例的 OCPU 与内存规格。
// 仅对 Flex 系列规格（如 VM.Standard.A1.Flex）有效。
func UpdateInstanceShape(ctx context.Context, cs *ClientSet, instanceID string, ocpus, memoryGBs float32) error {
	if ocpus <= 0 || memoryGBs <= 0 {
		return fmt.Errorf("OCPU 与内存容量必须大于 0")
	}
	_, err := cs.Compute.UpdateInstance(ctx, core.UpdateInstanceRequest{
		InstanceId: &instanceID,
		UpdateInstanceDetails: core.UpdateInstanceDetails{
			ShapeConfig: &core.UpdateInstanceShapeConfigDetails{
				Ocpus:       &ocpus,
				MemoryInGBs: &memoryGBs,
			},
		},
	})
	if err != nil {
		return wrap("调整实例规格失败", err)
	}
	return nil
}

// TerminateInstance 终止实例。preserveBootVolume 为 true 时保留启动卷。
func TerminateInstance(ctx context.Context, cs *ClientSet, instanceID string, preserveBootVolume bool) error {
	_, err := cs.Compute.TerminateInstance(ctx, core.TerminateInstanceRequest{
		InstanceId:        &instanceID,
		PreserveBootVolume: &preserveBootVolume,
	})
	if err != nil {
		return wrap("终止实例失败", err)
	}
	return nil
}

// LaunchInstance 创建一台实例。返回新实例的 ID。
// 当可用域资源不足时返回的错误可被 IsCapacityError 识别。
func LaunchInstance(ctx context.Context, cs *ClientSet, p LaunchParams) (*Instance, error) {
	if p.AvailabilityDomain == "" || p.CompartmentID == "" {
		return nil, fmt.Errorf("可用域与区间 OCID 不能为空")
	}
	if p.Shape == "" {
		p.Shape = "VM.Standard.A1.Flex"
	}
	if p.BootVolumeGBs <= 0 {
		p.BootVolumeGBs = 50
	}

	details := core.LaunchInstanceDetails{
		AvailabilityDomain: &p.AvailabilityDomain,
		CompartmentId:      &p.CompartmentID,
		Shape:              &p.Shape,
		ShapeConfig: &core.LaunchInstanceShapeConfigDetails{
			Ocpus:       &p.OCPUs,
			MemoryInGBs: &p.MemoryGBs,
		},
		SourceDetails: core.InstanceSourceViaImageDetails{
			ImageId:             &p.ImageID,
			BootVolumeSizeInGBs: &p.BootVolumeGBs,
		},
	}
	if p.DisplayName != "" {
		details.DisplayName = &p.DisplayName
	}
	if p.SubnetID != "" {
		assignPublicIP := p.AssignPublicIP
		details.CreateVnicDetails = &core.CreateVnicDetails{
			SubnetId:       &p.SubnetID,
			AssignPublicIp: &assignPublicIP,
		}
	}
	if p.SSHPublicKey != "" {
		details.Metadata = map[string]string{"ssh_authorized_keys": p.SSHPublicKey}
	}

	resp, err := cs.Compute.LaunchInstance(ctx, core.LaunchInstanceRequest{LaunchInstanceDetails: details})
	if err != nil {
		return nil, wrap("创建实例失败", err)
	}
	instance := convertInstance(resp.Instance)
	return &instance, nil
}

func intPtr(v int) *int { return &v }
