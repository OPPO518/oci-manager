package api

import (
	"context"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourname/oci-manager/internal/oci"
	"github.com/yourname/oci-manager/internal/store"
)

// overviewData 是单账号资源与成本的聚合结果，供 /overview 与 /dashboard 复用。
type overviewData struct {
	accountID        string
	region           string
	tenancyOCID      string
	compartmentCount int
	instanceCount    int
	runningCount     int
	totalOCPUs       float32
	totalMemoryGBs   float32
	monthCost        float32
	currency         string
	compartmentError string
	costError        string
}

// accountOverview 计算单个账号的资源与成本概览。它会真实调用 OCI（演示账号在
// 客户端池里被短路为模拟数据），供主页仪表盘跨账号聚合复用。
func (s *Server) accountOverview(ctx context.Context, account *store.Account) (*overviewData, error) {
	cs, err := s.pool.For(account.ID, account.Region)
	if err != nil {
		return nil, err
	}
	data := &overviewData{accountID: account.ID, region: cs.Region}

	compartments, cerr := oci.ListCompartments(ctx, cs, account.TenancyOCID)
	if cerr != nil {
		data.compartmentError = cerr.Error()
	} else {
		data.compartmentCount = len(compartments)
	}

	instances := make([]oci.Instance, 0)
	for _, comp := range compartments {
		list, lerr := oci.ListInstances(ctx, cs, comp.ID)
		if lerr != nil {
			continue
		}
		instances = append(instances, list...)
	}
	data.instanceCount = len(instances)
	for _, inst := range instances {
		if inst.State == "RUNNING" {
			data.runningCount++
			data.totalOCPUs += inst.OCPUs
			data.totalMemoryGBs += inst.MemoryGBs
		}
	}

	monthStart := time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.UTC)
	points, qerr := oci.QueryCost(ctx, cs, oci.UsageQuery{
		TenancyID:   account.TenancyOCID,
		Start:       monthStart,
		End:         time.Now().UTC(),
		Granularity: "monthly",
		GroupBy:     []string{"service"},
	})
	if qerr != nil {
		data.costError = qerr.Error()
	} else {
		for _, p := range points {
			data.monthCost += p.Cost
			if p.Currency != "" {
				data.currency = p.Currency
			}
		}
	}
	data.tenancyOCID = account.TenancyOCID
	return data, nil
}

// dashboard 跨所有账号聚合资源、成本与抢购任务，是主页仪表盘的数据源。
func (s *Server) dashboard(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 120*time.Second)
	defer cancel()

	accounts, err := s.store.ListAccounts()
	if err != nil {
		respondError(c, err)
		return
	}

	var (
		instTotal, instRunning   int
		ocpus, memory, monthCost float32
		currency                 string
		regionCounts             = map[string]int{}
		accountNames             = map[string]string{}
		errs                     []string
	)
	for _, acc := range accounts {
		accountNames[acc.ID] = acc.Name
		ov, e := s.accountOverview(ctx, &acc)
		if e != nil {
			errs = append(errs, acc.Name+": "+e.Error())
			continue
		}
		instTotal += ov.instanceCount
		instRunning += ov.runningCount
		ocpus += ov.totalOCPUs
		memory += ov.totalMemoryGBs
		monthCost += ov.monthCost
		if ov.currency != "" {
			currency = ov.currency
		}
		if ov.region != "" {
			regionCounts[ov.region] += ov.instanceCount
		}
		if ov.compartmentError != "" {
			errs = append(errs, acc.Name+": "+ov.compartmentError)
		}
		if ov.costError != "" {
			errs = append(errs, acc.Name+": "+ov.costError)
		}
	}

	tasks, _ := s.store.ListGrabTasks("")
	var gtTotal, gtRunning, gtSuccess int
	for _, t := range tasks {
		gtTotal++
		switch t.Status {
		case store.StatusRunning:
			gtRunning++
		case store.StatusSuccess:
			gtSuccess++
		}
	}

	logs, _ := s.store.ListAuditLogs(15)

	type regionRank struct {
		Region string `json:"region"`
		Count  int    `json:"count"`
	}
	ranks := make([]regionRank, 0, len(regionCounts))
	for r, n := range regionCounts {
		ranks = append(ranks, regionRank{Region: r, Count: n})
	}
	sort.Slice(ranks, func(i, j int) bool { return ranks[i].Count > ranks[j].Count })

	type activity struct {
		ID             int64  `json:"id"`
		CreatedAt      string `json:"created_at"`
		Account        string `json:"account"`
		Category       string `json:"category"`
		Operation      string `json:"operation"`
		Result         string `json:"result"`
		Target         string `json:"target"`
		CategoryLabel  string `json:"category_label"`
		OperationLabel string `json:"operation_label"`
		ResultLabel    string `json:"result_label"`
	}
	activities := make([]activity, 0, len(logs))
	for _, l := range logs {
		activities = append(activities, activity{
			ID: l.ID, CreatedAt: l.CreatedAt,
			Account: accountNames[l.AccountID], Category: l.Category,
			Operation: l.Operation, Result: l.Result, Target: l.Target,
			CategoryLabel:  store.AuditCategoryLabel(l.Category),
			OperationLabel: store.AuditOperationLabel(l.Operation),
			ResultLabel:    store.AuditResultLabel(l.Result),
		})
	}

	respondOK(c, gin.H{
		"accounts_total":     len(accounts),
		"instances_total":    instTotal,
		"running_count":      instRunning,
		"stopped_count":      instTotal - instRunning,
		"total_ocpus":        ocpus,
		"total_memory_gbs":   memory,
		"month_cost":         monthCost,
		"currency":           currency,
		"grab_tasks_total":   gtTotal,
		"grab_tasks_running": gtRunning,
		"grab_tasks_success": gtSuccess,
		"region_ranking":     ranks,
		"activities":         activities,
		"errors":             errs,
	})
}
