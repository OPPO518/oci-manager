package oci

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/usageapi"
)

// UsagePoint 是一条用量/成本记录。
type UsagePoint struct {
	Service      string    `json:"service"`
	ResourceName string    `json:"resource_name"`
	Region       string    `json:"region"`
	Shape        string    `json:"shape"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
	Cost         float32   `json:"cost"`
	Quantity     float32   `json:"quantity"`
	Currency     string    `json:"currency"`
}

// UsageQuery 描述一次用量查询。
type UsageQuery struct {
	TenancyID string
	// Start 与 End 为查询区间，End 会被调整为次日零点以覆盖当天。
	Start    time.Time
	End      time.Time
	// Granularity 取 hourly / daily / monthly / total。
	Granularity string
	// GroupBy 为聚合维度，例如 []string{"service"} 或 []string{"service", "region"}。
	GroupBy []string
}

// QueryCost 查询指定区间的成本，按维度聚合。
func QueryCost(ctx context.Context, cs *ClientSet, q UsageQuery) ([]UsagePoint, error) {
	granularity, err := parseGranularity(q.Granularity)
	if err != nil {
		return nil, err
	}
	start := common.SDKTime{Time: q.Start.UTC()}
	// OCI 的结束时间是开区间，加一天以包含最后一天的数据
	end := common.SDKTime{Time: q.End.UTC().AddDate(0, 0, 1)}

	isAggregate := true
	details := usageapi.RequestSummarizedUsagesDetails{
		TenantId:         &q.TenancyID,
		TimeUsageStarted: &start,
		TimeUsageEnded:   &end,
		Granularity:      granularity,
		QueryType:        usageapi.RequestSummarizedUsagesDetailsQueryTypeCost,
		IsAggregateByTime: &isAggregate,
	}
	if len(q.GroupBy) > 0 {
		details.GroupBy = q.GroupBy
	}

	var (
		points []UsagePoint
		page   *string
	)
	for {
		req := usageapi.RequestSummarizedUsagesRequest{
			RequestSummarizedUsagesDetails: details,
			Page:                           page,
			Limit:                          intPtr(500),
		}
		resp, err := cs.Usage.RequestSummarizedUsages(ctx, req)
		if err != nil {
			return nil, wrap("查询用量失败", err)
		}
		for _, item := range resp.Items {
			points = append(points, convertUsage(item))
		}
		if resp.OpcNextPage == nil || *resp.OpcNextPage == "" {
			break
		}
		page = resp.OpcNextPage
	}
	return points, nil
}

func convertUsage(item usageapi.UsageSummary) UsagePoint {
	p := UsagePoint{}
	if item.Service != nil {
		p.Service = *item.Service
	}
	if item.ResourceName != nil {
		p.ResourceName = *item.ResourceName
	}
	if item.Region != nil {
		p.Region = *item.Region
	}
	if item.Shape != nil {
		p.Shape = *item.Shape
	}
	if item.ComputedAmount != nil {
		p.Cost = *item.ComputedAmount
	}
	if item.ComputedQuantity != nil {
		p.Quantity = *item.ComputedQuantity
	}
	if item.Currency != nil {
		p.Currency = *item.Currency
	}
	if item.TimeUsageStarted != nil {
		p.StartTime = item.TimeUsageStarted.Time
	}
	if item.TimeUsageEnded != nil {
		p.EndTime = item.TimeUsageEnded.Time
	}
	return p
}

func parseGranularity(v string) (usageapi.RequestSummarizedUsagesDetailsGranularityEnum, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "daily":
		return usageapi.RequestSummarizedUsagesDetailsGranularityDaily, nil
	case "hourly":
		return usageapi.RequestSummarizedUsagesDetailsGranularityHourly, nil
	case "monthly":
		return usageapi.RequestSummarizedUsagesDetailsGranularityMonthly, nil
	case "total":
		return usageapi.RequestSummarizedUsagesDetailsGranularityTotal, nil
	default:
		return "", fmt.Errorf("不支持的时间粒度 %q，可选值：hourly/daily/monthly/total", v)
	}
}
