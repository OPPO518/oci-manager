package oci

import (
	"context"

	"github.com/oracle/oci-go-sdk/v65/identity"
)

// Region 描述账号已订阅的一个区域。
type Region struct {
	Key          string `json:"key"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	IsHomeRegion bool   `json:"is_home_region"`
}

// Compartment 描述一个区间（compartment）。
type Compartment struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// Inactive 表示区间状态非 ACTIVE。
	Inactive bool `json:"inactive"`
}

// ListRegions 列出账号已订阅的全部区域。
func ListRegions(ctx context.Context, cs *ClientSet, tenancyID string) ([]Region, error) {
	resp, err := cs.Identity.ListRegionSubscriptions(ctx, identity.ListRegionSubscriptionsRequest{
		TenancyId: &tenancyID,
	})
	if err != nil {
		return nil, wrap("列出订阅区域失败", err)
	}

	regions := make([]Region, 0, len(resp.Items))
	for _, item := range resp.Items {
		r := Region{}
		if item.RegionKey != nil {
			r.Key = *item.RegionKey
		}
		if item.RegionName != nil {
			r.Name = *item.RegionName
		}
		if item.IsHomeRegion != nil {
			r.IsHomeRegion = *item.IsHomeRegion
		}
		r.Status = string(item.Status)
		regions = append(regions, r)
	}
	return regions, nil
}

// ListAvailabilityDomains 列出指定区间下的可用域名称。
func ListAvailabilityDomains(ctx context.Context, cs *ClientSet, compartmentID string) ([]string, error) {
	resp, err := cs.Identity.ListAvailabilityDomains(ctx, identity.ListAvailabilityDomainsRequest{
		CompartmentId: &compartmentID,
	})
	if err != nil {
		return nil, wrap("列出可用域失败", err)
	}
	names := make([]string, 0, len(resp.Items))
	for _, ad := range resp.Items {
		if ad.Name != nil {
			names = append(names, *ad.Name)
		}
	}
	return names, nil
}

// ListCompartments 列出根区间及其全部子区间。
func ListCompartments(ctx context.Context, cs *ClientSet, tenancyID string) ([]Compartment, error) {
	inSubtree := true
	accessLevel := identity.ListCompartmentsAccessLevelAccessible

	var (
		compartments []Compartment
		page         *string
	)
	for {
		resp, err := cs.Identity.ListCompartments(ctx, identity.ListCompartmentsRequest{
			CompartmentId:          &tenancyID,
			CompartmentIdInSubtree: &inSubtree,
			AccessLevel:            accessLevel,
			Page:                   page,
		})
		if err != nil {
			return nil, wrap("列出区间失败", err)
		}
		for _, c := range resp.Items {
			item := Compartment{}
			if c.Id != nil {
				item.ID = *c.Id
			}
			if c.Name != nil {
				item.Name = *c.Name
			}
			if c.Description != nil {
				item.Description = *c.Description
			}
			item.Inactive = c.LifecycleState != identity.CompartmentLifecycleStateActive
			compartments = append(compartments, item)
		}
		if resp.OpcNextPage == nil || *resp.OpcNextPage == "" {
			break
		}
		page = resp.OpcNextPage
	}
	return compartments, nil
}

// GetTenancyName 查询租户外显名称，用于连通性测试时确认凭证有效。
func GetTenancyName(ctx context.Context, cs *ClientSet, tenancyID string) (string, error) {
	resp, err := cs.Identity.GetTenancy(ctx, identity.GetTenancyRequest{TenancyId: &tenancyID})
	if err != nil {
		return "", wrap("查询租户信息失败", err)
	}
	if resp.Name != nil {
		return *resp.Name, nil
	}
	return "", nil
}
