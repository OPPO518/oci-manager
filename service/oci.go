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

// 提取我们在前端需要展示的核心字段，丢弃甲骨文冗余的 SDK 底层数据
type CachedInstance struct {
	Id             string `json:"id"`
	DisplayName    string `json:"displayName"`
	Region         string `json:"region"`
	LifecycleState string `json:"lifecycleState"`
	Shape          string `json:"shape"`
}

// buildClient 保持不变（为 VNC 和 API 提供底层安全代理通道）
func buildClient(accountID int) (core.ComputeClient, identity.IdentityClient, string, error) {
	var encryptedConfig, proxyStr string
	err := DB.QueryRow("SELECT encrypted_config, proxy_url FROM oci_accounts WHERE id = ?", accountID).Scan(&encryptedConfig, &proxyStr)
	if err != nil { return core.ComputeClient{}, identity.IdentityClient{}, "", fmt.Errorf("找不到该账号或已被删除") }

	decryptedJSON, err := Decrypt(encryptedConfig)
	if err != nil { return core.ComputeClient{}, identity.IdentityClient{}, "", fmt.Errorf("凭证解密失败") }

	var creds OCICredentials
	if err := json.Unmarshal([]byte(decryptedJSON), &creds); err != nil {
		return core.ComputeClient{}, identity.IdentityClient{}, "", fmt.Errorf("内部凭证损坏")
	}

	configProvider := common.NewRawConfigurationProvider(
		creds.Tenancy, creds.User, creds.Region, creds.Fingerprint, creds.PrivateKey, nil,
	)

	computeClient, err := core.NewComputeClientWithConfigurationProvider(configProvider)
	if err != nil { return core.ComputeClient{}, identity.IdentityClient{}, "", err }
	
	identityClient, err := identity.NewIdentityClientWithConfigurationProvider(configProvider)
	if err != nil { return core.ComputeClient{}, identity.IdentityClient{}, "", err }

	proxyURL, _ := url.Parse(proxyStr)
	httpClient := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
		Timeout:   15 * time.Second,
	}
	computeClient.HTTPClient = httpClient
	identityClient.HTTPClient = httpClient

	return computeClient, identityClient, creds.Tenancy, nil
}

// 🚀 新增：秒读本地数据库缓存（0 延迟）
func GetCachedInstances(accountID int) ([]CachedInstance, error) {
	var cacheStr string
	err := DB.QueryRow("SELECT cached_instances FROM oci_accounts WHERE id = ?", accountID).Scan(&cacheStr)
	if err != nil || cacheStr == "" {
		return []CachedInstance{}, nil // 没有缓存时返回空数组
	}

	var instances []CachedInstance
	json.Unmarshal([]byte(cacheStr), &instances)
	return instances, nil
}

// 🚀 改造：深度同步云端数据，并偷偷覆写到本地数据库
func SyncInstances(accountID int) ([]CachedInstance, error) {
	computeClient, identityClient, tenancyID, err := buildClient(accountID)
	if err != nil { return nil, err }

	reqComp := identity.ListCompartmentsRequest{
		CompartmentId:          common.String(tenancyID),
		CompartmentIdInSubtree: common.Bool(true),
		AccessLevel:            identity.ListCompartmentsAccessLevelAccessible,
	}
	compResp, err := identityClient.ListCompartments(context.Background(), reqComp)
	if err != nil { return nil, fmt.Errorf("获取区间列表失败: %v", err) }

	compartments := []string{tenancyID}
	for _, c := range compResp.Items {
		if c.LifecycleState == identity.CompartmentLifecycleStateActive {
			compartments = append(compartments, *c.Id)
		}
	}

	var allInstances []CachedInstance
	for _, compID := range compartments {
		reqInst := core.ListInstancesRequest{CompartmentId: common.String(compID)}
		respInst, err := computeClient.ListInstances(context.Background(), reqInst)
		if err == nil && len(respInst.Items) > 0 {
			for _, inst := range respInst.Items {
				if inst.LifecycleState != core.InstanceLifecycleStateTerminated {
					// 将复杂的 SDK 结构体转换为精简的缓存结构体
					allInstances = append(allInstances, CachedInstance{
						Id:             *inst.Id,
						DisplayName:    *inst.DisplayName,
						Region:         *inst.Region,
						LifecycleState: string(inst.LifecycleState),
						Shape:          *inst.Shape,
					})
				}
			}
		}
	}

	// 偷偷将最新结果序列化为 JSON 字符串，存入当前账号的抽屉里
	cacheBytes, _ := json.Marshal(allInstances)
	_, _ = DB.Exec("UPDATE oci_accounts SET cached_instances = ? WHERE id = ?", string(cacheBytes), accountID)

	return allInstances, nil
}

// InstanceAction (发送电源指令，保持不变)
func InstanceAction(accountID int, instanceID string, action core.InstanceActionActionEnum) error {
	computeClient, _, _, err := buildClient(accountID)
	if err != nil { return err }

	req := core.InstanceActionRequest{
		InstanceId: common.String(instanceID),
		Action:     action,
	}
	_, err = computeClient.InstanceAction(context.Background(), req)
	return err
}
