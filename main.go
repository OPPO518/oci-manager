package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
)

// 这个函数就是我们之前跑通的核心逻辑，现在把它打包成一个被网页调用的接口
func getInstancesHandler(w http.ResponseWriter, r *http.Request) {
	configProvider := common.DefaultConfigProvider()
	tenancyID, err := configProvider.TenancyOCID()
	if err != nil {
		http.Error(w, `{"error": "读取 OCI 配置失败"}`, http.StatusInternalServerError)
		return
	}

	computeClient, err := core.NewComputeClientWithConfigurationProvider(configProvider)
	if err != nil {
		http.Error(w, `{"error": "创建计算客户端失败"}`, http.StatusInternalServerError)
		return
	}

	// 代理劫持逻辑
	proxyStr := "socks5://127.0.0.1:10801"
	proxyURL, _ := url.Parse(proxyStr)
	customHttpClient := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
		Timeout:   15 * time.Second,
	}
	computeClient.HTTPClient = customHttpClient

	req := core.ListInstancesRequest{
		CompartmentId: common.String(tenancyID),
	}

	resp, err := computeClient.ListInstances(context.Background(), req)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "请求失败: %v"}`, err), http.StatusInternalServerError)
		return
	}

	// 告诉前端我们返回的是 JSON 格式的数据
	w.Header().Set("Content-Type", "application/json")
	
	// 如果没有机器，返回空列表而不是报错
	if len(resp.Items) == 0 {
		w.Write([]byte(`{"message": "请求成功，但当前账号下没有服务器", "instances": []}`))
		return
	}

	// 将 OCI 的返回结果打包成 JSON 发给网页
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":   "请求成功",
		"instances": resp.Items,
	})
}

func main() {
	// 1. 设置一个 API 接口，网页点按钮就会触发这个接口
	http.HandleFunc("/api/instances", getInstancesHandler)

	// 2. 将 web 文件夹里的静态网页暴露出去
	http.Handle("/", http.FileServer(http.Dir("./web")))

	fmt.Println("🚀 Web 服务器已启动！")
	fmt.Println("👉 请在浏览器中访问: http://你的VPS公网IP:8080")
	
	// 3. 在 8080 端口启动服务器
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		fmt.Println("❌ 服务器启动失败:", err)
	}
}
