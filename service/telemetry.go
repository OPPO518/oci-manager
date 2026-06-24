package service

import (
	"net/http"
	"runtime"

	"github.com/gin-gonic/gin"
)

// GetDashboardMetrics 汇聚主页监控大屏的所有核心数据
func GetDashboardMetrics(c *gin.Context) {
	// 1. 资产大盘统计 (0 延迟读取本地 SQLite 缓存)
	var accountCount, instanceCount, activeProxies int

	// 统计托管账号总数
	DB.QueryRow("SELECT COUNT(*) FROM oci_accounts").Scan(&accountCount)
	
	// 统计配置了专属 Xray SOCKS5 代理的活跃节点数
	DB.QueryRow("SELECT COUNT(*) FROM oci_accounts WHERE proxy_url != ''").Scan(&activeProxies)

	// 利用 SQLite 原生 JSON 函数，瞬间计算出所有抽屉里缓存的 VPS 实例总数
	// 注意：如果你的 SQLite 编译时没开启 JSON1 扩展，这里可能会报错，可以后续用 Go 循环解析代替
	err := DB.QueryRow("SELECT COALESCE(SUM(json_array_length(cached_instances)), 0) FROM oci_accounts WHERE cached_instances != '[]'").Scan(&instanceCount)
	if err != nil {
		instanceCount = 0 // 容错处理
	}

	// 2. 宿主机性能探针 (极简版内存与并发监控)
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// 3. 组装 Redwood 卡片所需的数据结构
	c.JSON(http.StatusOK, gin.H{
		"system_assets": gin.H{
			"total_accounts":  accountCount,
			"total_instances": instanceCount,
			"active_proxies":  activeProxies,
		},
		"host_metrics": gin.H{
			"go_routines": runtime.NumGoroutine(), // 监控开机并发任务的压力
			"mem_sys_mb":  memStats.Sys / 1024 / 1024, // 核心后端内存占用
			"os_arch":     runtime.GOOS + "/" + runtime.GOARCH,
		},
	})
}
