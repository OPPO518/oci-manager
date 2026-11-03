package oci

import (
	"context"

	"github.com/oracle/oci-go-sdk/v65/core"
)

// Subnet 描述一个子网，用于创建实例时选择网络。
type Subnet struct {
	ID                 string `json:"id"`
	DisplayName        string `json:"display_name"`
	CIDRBlock          string `json:"cidr_block"`
	VcnID              string `json:"vcn_id"`
	AvailabilityDomain string `json:"availability_domain"`
	Region             string `json:"region"`
}

// Image 描述一个可用的镜像。
type Image struct {
	ID                      string `json:"id"`
	DisplayName             string `json:"display_name"`
	OperatingSystem         string `json:"operating_system"`
	OperatingSystemVersion  string `json:"operating_system_version"`
}

// ListSubnets 列出区间下的全部子网。
func ListSubnets(ctx context.Context, cs *ClientSet, compartmentID string) ([]Subnet, error) {
	resp, err := cs.Network.ListSubnets(ctx, core.ListSubnetsRequest{CompartmentId: &compartmentID})
	if err != nil {
		return nil, wrap("列出子网失败", err)
	}
	subnets := make([]Subnet, 0, len(resp.Items))
	for _, s := range resp.Items {
		item := Subnet{}
		if s.Id != nil {
			item.ID = *s.Id
		}
		if s.DisplayName != nil {
			item.DisplayName = *s.DisplayName
		}
		if s.CidrBlock != nil {
			item.CIDRBlock = *s.CidrBlock
		}
		if s.VcnId != nil {
			item.VcnID = *s.VcnId
		}
		if s.AvailabilityDomain != nil {
			item.AvailabilityDomain = *s.AvailabilityDomain
		}
		subnets = append(subnets, item)
	}
	return subnets, nil
}

// ListImages 列出区间下可用的镜像，按操作系统排序交给前端筛选。
func ListImages(ctx context.Context, cs *ClientSet, compartmentID string) ([]Image, error) {
	resp, err := cs.Compute.ListImages(ctx, core.ListImagesRequest{CompartmentId: &compartmentID})
	if err != nil {
		return nil, wrap("列出镜像失败", err)
	}
	images := make([]Image, 0, len(resp.Items))
	for _, img := range resp.Items {
		item := Image{}
		if img.Id != nil {
			item.ID = *img.Id
		}
		if img.DisplayName != nil {
			item.DisplayName = *img.DisplayName
		}
		if img.OperatingSystem != nil {
			item.OperatingSystem = *img.OperatingSystem
		}
		if img.OperatingSystemVersion != nil {
			item.OperatingSystemVersion = *img.OperatingSystemVersion
		}
		images = append(images, item)
	}
	return images, nil
}
