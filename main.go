package main

import (
	"fmt"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
)

func main() {
	// 使用默认配置提供者 (默认读取 ~/.oci/config)
	configProvider := common.DefaultConfigProvider()

	// 尝试创建一个 OCI 核心计算客户端
	_, err := core.NewComputeClientWithConfigurationProvider(configProvider)
	if err != nil {
		fmt.Println("❌ 创建 OCI 客户端失败:", err)
		return
	}

	// 验证配置，尝试获取 Tenancy OCID
	tenancyID, err := configProvider.TenancyOCID()
	if err != nil {
		fmt.Println("❌ 读取 OCI 配置失败，请检查 ~/.oci/config 文件:", err)
		return
	}

	fmt.Println("✅ 成功加载 OCI 配置！")
	fmt.Println("📌 当前的 Tenancy OCID 是:", tenancyID)
	fmt.Println("🚀 基础通信通道已打通，可以进行下一步开发。")
}
