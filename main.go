package main

import (
	"context"
	"fmt"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
)

func main() {
	// 1. 加载本地 OCI 配置文件
	configProvider := common.DefaultConfigProvider()
	tenancyID, err := configProvider.TenancyOCID()
	if err != nil {
		fmt.Println("❌ 读取 OCI 配置失败:", err)
		return
	}

	// 2. 创建一个“计算客户端”，专门用来管理服务器（实例）
	computeClient, err := core.NewComputeClientWithConfigurationProvider(configProvider)
	if err != nil {
		fmt.Println("❌ 创建 OCI 计算客户端失败:", err)
		return
	}

	fmt.Println("✅ 身份验证通过！正在向 Oracle 请求获取您的服务器列表，请稍候...")

	// 3. 构建请求：告诉 Oracle 我们要查哪个区间（Compartment）的机器
	// 注意：这里默认查询你的根区间 (Tenancy)
	req := core.ListInstancesRequest{
		CompartmentId: common.String(tenancyID),
	}

	// 4. 正式发送网络请求
	resp, err := computeClient.ListInstances(context.Background(), req)
	if err != nil {
		fmt.Println("❌ 获取实例列表失败:", err)
		return
	}

	// 5. 解析并打印获取到的机器列表
	instances := resp.Items
	if len(instances) == 0 {
		fmt.Println("⚠️ 请求成功，但在当前的根区间 (Root Compartment) 下没有找到任何服务器。")
		fmt.Println("提示：如果你在网页控制台能看到机器，说明机器可能建立在子区间里。")
	} else {
		fmt.Printf("🎉 成功获取！共找到 %d 台服务器：\n", len(instances))
		fmt.Println("=====================================")
		for i, instance := range instances {
			// 获取机器名称，如果没有名字就显示"未命名"
			name := "未命名"
			if instance.DisplayName != nil {
				name = *instance.DisplayName
			}
			// 获取机器的当前状态 (比如 RUNNING 或者 STOPPED)
			state := "未知状态"
			if instance.LifecycleState != "" {
				state = string(instance.LifecycleState)
			}
			
			fmt.Printf("[%d] 机器名称: %s | 运行状态: %s\n", i+1, name, state)
		}
		fmt.Println("=====================================")
	}
}
