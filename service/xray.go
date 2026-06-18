package service

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sync"
)

// ProxyConfig 接收前端解析过来的节点档案
type ProxyConfig struct {
	Protocol string            `json:"protocol"`
	Host     string            `json:"host"`
	Port     string            `json:"port"`
	ID       string            `json:"id"`
	Network  string            `json:"network"`
	Security string            `json:"security"`
	Raw      map[string]string `json:"raw"`
}

var (
	xrayMutex   sync.Mutex
	xrayProcess *exec.Cmd
	basePort    = 10800
	activeNodes = make(map[string]ProxyConfig) // 内存数据库，记录所有分配的插座
)

// StartXrayProcess 改为单进程多路复用模式
func StartXrayProcess(config ProxyConfig) (string, error) {
	xrayMutex.Lock()
	defer xrayMutex.Unlock()

	// 动态计算下一个空闲插座号
	localPort := basePort + len(activeNodes)
	portStr := fmt.Sprintf("%d", localPort)

	// 将新节点加入内存矩阵
	activeNodes[portStr] = config

	// 编译并平滑重载 Xray 矩阵
	if err := reloadXrayMatrix(); err != nil {
		delete(activeNodes, portStr) // 如果重载失败，安全回滚
		return "", err
	}

	return fmt.Sprintf("socks5://127.0.0.1:%s", portStr), nil
}

// reloadXrayMatrix 核心引擎：将所有节点编译为一份大型路由表，并平滑重启单进程
func reloadXrayMatrix() error {
	inbounds := []map[string]interface{}{}
	outbounds := []map[string]interface{}{}
	rules := []map[string]interface{}{}

	for port, config := range activeNodes {
		inTag := "in_" + port
		outTag := "out_" + port

		// 1. 生成独立的入站 SOCKS5 插座
		inbounds = append(inbounds, map[string]interface{}{
			"listen":   "127.0.0.1",
			"port":     json.Number(port),
			"protocol": "socks",
			"tag":      inTag,
			"settings": map[string]interface{}{"auth": "noauth", "udp": true},
		})

		// 2. 组装出站节点
		outbound := map[string]interface{}{
			"protocol": config.Protocol,
			"tag":      outTag,
		}

		streamSettings := map[string]interface{}{
			"network":  config.Network,
			"security": config.Security,
		}

		if config.Security == "reality" && config.Raw != nil {
			streamSettings["realitySettings"] = map[string]interface{}{
				"serverName":  config.Raw["sni"],
				"publicKey":   config.Raw["pbk"],
				"fingerprint": config.Raw["fp"],
				"shortId":     config.Raw["sid"],
				"spiderX":     config.Raw["spx"],
			}
		} else if config.Security == "tls" && config.Raw != nil {
			streamSettings["tlsSettings"] = map[string]interface{}{
				"serverName": config.Raw["sni"],
			}
		}

		// 端口防空容错补丁
		remotePort := config.Port
		if remotePort == "" { remotePort = "443" }

		if config.Protocol == "vless" {
			flow := ""
			if config.Raw != nil { flow = config.Raw["flow"] }
			outbound["settings"] = map[string]interface{}{
				"vnext": []map[string]interface{}{
					{
						"address": config.Host,
						"port":    json.Number(remotePort),
						"users": []map[string]interface{}{
							{"id": config.ID, "encryption": "none", "flow": flow},
						},
					},
				},
			}
		} else if config.Protocol == "vmess" {
			outbound["settings"] = map[string]interface{}{
				"vnext": []map[string]interface{}{
					{
						"address": config.Host,
						"port":    json.Number(remotePort),
						"users": []map[string]interface{}{
							{"id": config.ID, "alterId": 0},
						},
					},
				},
			}
		} else {
			return fmt.Errorf("不支持的协议类型: %s", config.Protocol)
		}
		
		outbound["streamSettings"] = streamSettings
		outbounds = append(outbounds, outbound)

		// 3. 核心壁垒：建立极其严苛的物理路由隔离规则 (In 绝对绑定对应的 Out)
		rules = append(rules, map[string]interface{}{
			"type":        "field",
			"inboundTag":  []string{inTag},
			"outboundTag": outTag,
		})
	}

	xrayConfig := map[string]interface{}{
		"log":       map[string]string{"loglevel": "warning"},
		"inbounds":  inbounds,
		"outbounds": outbounds,
		"routing": map[string]interface{}{
			"domainStrategy": "AsIs",
			"rules":          rules,
		},
	}

	// 4. 写出全局总控配置文件
	configBytes, err := json.MarshalIndent(xrayConfig, "", "  ")
	if err != nil { return fmt.Errorf("JSON 序列化失败: %v", err) }

	os.MkdirAll("xray_bin/configs", os.ModePerm)
	err = os.WriteFile("xray_bin/configs/matrix.json", configBytes, 0644)
	if err != nil { return fmt.Errorf("写出配置文件失败: %v", err) }

	// 5. 暴力斩杀僵尸进程，平滑启动新矩阵
	if xrayProcess != nil && xrayProcess.Process != nil {
		_ = xrayProcess.Process.Kill()
		_ = xrayProcess.Wait() // 🚀 修复核心崩溃：强行等待内核回收资源，绝不遗留僵尸
	}

	xrayProcess = exec.Command("xray_bin/xray", "-c", "xray_bin/configs/matrix.json")
	if err := xrayProcess.Start(); err != nil {
		return fmt.Errorf("启动单进程 Xray 矩阵失败: %v", err)
	}

	return nil
}
