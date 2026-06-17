package service

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

// GetInstances 现在需要接收一个具体的“账号 ID”，去动态连不同的机器
func GetInstances(accountID int) ([]core.Instance, error) {
	// 1. 从数据库查出这个账号的加密凭证和专属代理
	var encryptedConfig, proxyStr string
	err := DB.QueryRow("SELECT encrypted_config, proxy_url FROM oci_accounts WHERE id = ?", accountID).Scan(&encryptedConfig, &proxyStr)
	if err != nil {
		return nil, fmt.Errorf("找不到该账号或已被删除: %v", err)
	}

	// 2. 用主密钥在内存中瞬间解密
	decryptedJSON, err := Decrypt(encryptedConfig)
	if err != nil {
		return nil, fmt.Errorf("凭证解密失败，系统密钥可能已被篡改: %v", err)
	}

	var creds OCICredentials
	if err := json.Unmarshal([]byte(decryptedJSON), &creds); err != nil {
		return nil, fmt.Errorf("内部凭证格式损坏: %v", err)
	}

	// 3. 🚀 核心升级：不再读取本地文件，直接用解密后的参数在内存里组装 OCI 通行证
	configProvider := common.NewRawConfigurationProvider(
		creds.Tenancy,
		creds.User,
		creds.Region,
		creds.Fingerprint,
		creds.PrivateKey,
		nil, // 私钥密码（通常没有，填 nil）
	)

	computeClient, err := core.NewComputeClientWithConfigurationProvider(configProvider)
	if err != nil {
		return nil, fmt.Errorf("创建 OCI 计算客户端失败: %v", err)
	}

	// 4. 配置专属的代理通道
	proxyURL, err := url.Parse(proxyStr)
	if err != nil {
		return nil, fmt.Errorf("代理地址格式解析失败: %v", err)
	}
	computeClient.HTTPClient = &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
		Timeout:   15 * time.Second,
	}

	// 5. 正式发起请求
	req := core.ListInstancesRequest{CompartmentId: common.String(creds.Tenancy)}
	resp, err := computeClient.ListInstances(context.Background(), req)
	if err != nil {
		return nil, fmt.Errorf("请求 Oracle 接口被拒绝或超时: %v", err)
	}

	return resp.Items, nil
}
