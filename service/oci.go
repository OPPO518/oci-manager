package service

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
)

// GetInstances 查询 OCI 服务器列表的逻辑统一封装在这里
func GetInstances(proxyAddr string) ([]core.Instance, error) {
	configProvider := common.DefaultConfigProvider()
	
	// 改进 1：严谨捕获配置读取错误
	tenancyID, err := configProvider.TenancyOCID()
	if err != nil {
		return nil, fmt.Errorf("读取 OCI 凭证失败: %v", err)
	}

	computeClient, err := core.NewComputeClientWithConfigurationProvider(configProvider)
	if err != nil {
		return nil, fmt.Errorf("创建 OCI 计算客户端失败: %v", err)
	}

	// 改进 2：严谨捕获代理地址解析错误
	proxyURL, err := url.Parse(proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("代理地址格式解析失败: %v", err)
	}

	// 配置自定义 HTTP 客户端，挂载代理
	computeClient.HTTPClient = &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
		Timeout:   15 * time.Second,
	}

	req := core.ListInstancesRequest{CompartmentId: common.String(tenancyID)}
	resp, err := computeClient.ListInstances(context.Background(), req)
	if err != nil {
		return nil, fmt.Errorf("请求 Oracle 接口被拒绝或超时: %v", err)
	}

	return resp.Items, nil
}
