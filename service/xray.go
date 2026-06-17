package service

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
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

// 自动寻找系统上的空闲端口（从 10800 开始试探）
func getFreePort() (int, error) {
	for port := 10800; port <= 10900; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			ln.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("没有找到空闲的本地端口")
}

// StartXrayProcess 核心：拼装 JSON 并静默启动 Xray
func StartXrayProcess(config ProxyConfig) (string, error) {
	localPort, err := getFreePort()
	if err != nil { return "", err }

	// 1. 构建 Xray 的骨架结构
	xrayConfig := map[string]interface{}{
		"log": map[string]string{"loglevel": "warning"},
		"inbounds": []map[string]interface{}{
			{
				"listen":   "127.0.0.1",
				"port":     localPort,
				"protocol": "socks",
				"settings": map[string]interface{}{"auth": "noauth", "udp": true},
			},
		},
	}

	// 2. 根据协议智能拼装复杂的 Outbound (出站规则)
	outbound := map[string]interface{}{
		"protocol": config.Protocol,
		"tag":      "proxy",
	}

	streamSettings := map[string]interface{}{
		"network":  config.Network,
		"security": config.Security,
	}

	// Reality 与 TLS 特殊处理
	if config.Security == "reality" {
		streamSettings["realitySettings"] = map[string]interface{}{
			"serverName":  config.Raw["sni"],
			"publicKey":   config.Raw["pbk"],
			"fingerprint": config.Raw["fp"],
			"shortId":     config.Raw["sid"],
			"spiderX":     config.Raw["spx"],
		}
	} else if config.Security == "tls" {
		streamSettings["tlsSettings"] = map[string]interface{}{
			"serverName": config.Raw["sni"],
		}
	}

	// VLESS 节点装配
	if config.Protocol == "vless" {
		outbound["settings"] = map[string]interface{}{
			"vnext": []map[string]interface{}{
				{
					"address": config.Host,
					"port":    json.Number(config.Port), // 确保端口是数字
					"users": []map[string]interface{}{
						{
							"id":         config.ID,
							"encryption": "none",
							"flow":       config.Raw["flow"],
						},
					},
				},
			},
		}
	} else if config.Protocol == "vmess" {
		// VMess 节点装配
		outbound["settings"] = map[string]interface{}{
			"vnext": []map[string]interface{}{
				{
					"address": config.Host,
					"port":    json.Number(config.Port),
					"users": []map[string]interface{}{ {"id": config.ID, "alterId": 0} },
				},
			},
		}
	} else {
		return "", fmt.Errorf("当前后端引擎仅完美适配 VLESS/VMess，其他协议正在开发中")
	}

	outbound["streamSettings"] = streamSettings
	xrayConfig["outbounds"] = []map[string]interface{}{outbound}

	// 3. 将装配好的 JSON 写出到本地磁盘
	configBytes, _ := json.MarshalIndent(xrayConfig, "", "  ")
	os.MkdirAll("xray_bin/configs", os.ModePerm)
	configPath := fmt.Sprintf("xray_bin/configs/proxy_%d.json", localPort)
	os.WriteFile(configPath, configBytes, 0644)

	// 4. 召唤幽灵进程：在系统底层跑起 xray
	execPath, _ := filepath.Abs("xray_bin/xray")
	cmd := exec.Command(execPath, "-c", configPath)
	
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("唤醒 Xray 进程失败，请检查 xray_bin/xray 是否具有执行权限: %v", err)
	}

	// 成功！返回供 OCI 引擎使用的内部 SOCKS5 代理地址
	return fmt.Sprintf("socks5://127.0.0.1:%d", localPort), nil
}
