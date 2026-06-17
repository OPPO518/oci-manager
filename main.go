package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
)

func main() {
	// 1. 加载配置
	configProvider := common.DefaultConfigProvider()
	tenancyID, err := configProvider.TenancyOCID()
	if err != nil {
		fmt.Println("❌ 读取 OCI 配置失败:", err)
		return
	}

	// 2. 创建计算客户端
	computeClient, err := core.NewComputeClientWithConfigurationProvider(configProvider)
	if err != nil {
		fmt.Println("❌ 创建 OCI 计算客户端失败:", err)
		return
	}

	// ==========================================
	// 🚀 核心新增：代理隔离劫持逻辑
	// ==========================================
	
	// 假设我们为这个账号分配了本地的 10801 代理端口 (例如 Xray / VLESS 的本地 socks5 入口)
	proxyStr := "socks5://127.0.0.1:10801"
	fmt.Printf("🛡️ 正在启用防关联隔离，当前账号强制使用代理: %s\n", proxyStr)
	
	proxyURL, err := url.Parse(proxyStr)
	if err != nil {
		fmt.Println("❌ 代理地址格式错误:", err)
		return
	}

	// 创建一个挂载了指定代理的自定义网络传输层
	customTransport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
	}
	// 创建自定义 HTTP 客户端，并设置 15 秒超时防止死等
	customHttpClient := &http.Client{
		Transport: customTransport,
		Timeout:   15 * time.Second,
	}

	// 最关键的一步：拔掉 Oracle SDK 默认的网线，插上我们带有代理的网线
	// 【已修复】直接赋值即可，Go 内置的 client 完全兼容
	computeClient.HTTPClient = customHttpClient

	// ==========================================

	fmt.Println("✅ 代理通道已挂载！正在通过隔离通道向 Oracle 发送请求...")

	req := core.ListInstancesRequest{
		CompartmentId: common.String(tenancyID),
	}

	// 发送请求
	resp, err := computeClient.ListInstances(context.Background(), req)
	if err != nil {
		// ⚠️ 注意这里的报错！
		fmt.Println("\n❌ 请求失败！详细原因如下:")
		fmt.Println(err)
		fmt.Println("\n💡 诊断提示: 如果上面报错显示 'connection refused' (拒绝连接)，这反而是件好事！")
		fmt.Println("这证明我们的代码成功劫持了流量，但因为你的 VPS 上目前还没有在 10801 端口运行 Xray 代理，所以数据发不出去。")
		return
	}

	fmt.Println("🎉 请求成功！由于本账号下没有机器，返回空列表是正确的。")
	_ = resp.Items
}
