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
	"github.com/oracle/oci-go-sdk/v65/identity"
)

// buildClient 是我们内部的一个“造轮子”函数，提取公共部分，避免每次调用都写一遍解密和代理逻辑
func buildClient(accountID int) (core.ComputeClient, identity.IdentityClient, string, error) {
	var encryptedConfig, proxyStr string
	err := DB.QueryRow("SELECT encrypted_config, proxy_url FROM oci_accounts WHERE id = ?", accountID).Scan(&encryptedConfig, &proxyStr)
	if err != nil {
		return core.ComputeClient{}, identity.IdentityClient{}, "", fmt.Errorf("找不到该账号或已被删除")
	}

	decryptedJSON, err := Decrypt(encryptedConfig)
	if err != nil {
		return core.ComputeClient{}, identity.IdentityClient{}, "", fmt.Errorf("凭证解密失败")
	}

	var creds OCICredentials
	if err := json.Unmarshal([]byte(decryptedJSON), &creds); err != nil {
		return core.ComputeClient{}, identity.IdentityClient{}, "", fmt.Errorf("内部凭证损坏")
	}

	configProvider := common.NewRawConfigurationProvider(
		creds.Tenancy, creds.User, creds.Region, creds.Fingerprint, creds.PrivateKey, nil,
	)

	// 创建计算客户端（用来管机器）
	computeClient, err := core.NewComputeClientWithConfigurationProvider(configProvider)
	if err != nil {
		return core.ComputeClient{}, identity.IdentityClient{}, "", err
	}
	
	// 创建身份客户端（用来查区间）
	identityClient, err := identity.NewIdentityClientWithConfigurationProvider(configProvider)
	if err != nil {
		return core.ComputeClient{}, identity.IdentityClient{}, "", err
	}

	// 强制挂载代理通道
	proxyURL, _ := url.Parse(proxyStr)
	httpClient := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
		Timeout:   15 * time.Second,
	}
	computeClient.HTTPClient = httpClient
	identityClient.HTTPClient = httpClient

	return computeClient, identityClient, creds.Tenancy, nil
}

// GetInstances 核心升级：自动遍历所有子区间
func GetInstances(accountID int) ([]core.Instance, error) {
	computeClient, identityClient, tenancyID, err := buildClient(accountID)
	if err != nil {
		return nil, err
	}

	// 1. 利用 identity 客户端拉取所有子区间列表
	reqComp := identity.ListCompartmentsRequest{
		CompartmentId:          common.String(tenancyID),
		CompartmentIdInSubtree: common.Bool(true),
		AccessLevel:            identity.ListCompartmentsAccessLevelAccessible,
	}
	compResp, err := identityClient.ListCompartments(context.Background(), reqComp)
	if err != nil {
		return nil, fmt.Errorf("获取区间列表失败: %v", err)
	}

	// 2. 把根区间和所有活跃的子区间整理到一个数组里
	compartments := []string{tenancyID}
	for _, c := range compResp.Items {
		if c.LifecycleState == identity.CompartmentLifecycleStateActive {
			compartments = append(compartments, *c.Id)
		}
	}

	// 3. 带着雷达扫荡所有区间，揪出机器
	var allInstances []core.Instance
	for _, compID := range compartments {
		reqInst := core.ListInstancesRequest{CompartmentId: common.String(compID)}
		respInst, err := computeClient.ListInstances(context.Background(), reqInst)
		if err == nil && len(respInst.Items) > 0 {
			for _, inst := range respInst.Items {
				// 过滤掉已经永久销毁 (TERMINATED) 的机器尸体，保持列表干净
				if inst.LifecycleState != core.InstanceLifecycleStateTerminated {
					allInstances = append(allInstances, inst)
				}
			}
		}
	}
	return allInstances, nil
}

// InstanceAction 全新武器：发送电源指令
func InstanceAction(accountID int, instanceID string, action core.InstanceActionActionEnum) error {
	computeClient, _, _, err := buildClient(accountID)
	if err != nil {
		return err
	}

	req := core.InstanceActionRequest{
		InstanceId: common.String(instanceID),
		Action:     action,
	}
	_, err = computeClient.InstanceAction(context.Background(), req)
	return err
}
