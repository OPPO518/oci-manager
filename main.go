package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	
	"oci-manager/service"
)

func getInstancesHandler(w http.ResponseWriter, r *http.Request) {
	// 改进 3：统一声明返回的数据格式为 JSON，避免前端解析崩溃
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	instances, err := service.GetInstances("socks5://127.0.0.1:10801")
	if err != nil {
		// 如果后端出错，也用 JSON 格式把错误原因发给前端
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
		})
		return
	}
	
	// 请求成功，正常下发列表
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":   "请求成功",
		"instances": instances,
	})
}

func main() {
	// 注册 API 接口
	http.HandleFunc("/api/instances", getInstancesHandler)
	
	// 注册前端静态页面服务
	http.Handle("/", http.FileServer(http.Dir("./web")))

	// 改进 4：把控制台的启动提示加回来，方便运维确认状态
	fmt.Println("🚀 核心服务已成功启动！")
	fmt.Println("👉 请打开浏览器访问: http://您的VPS公网IP:8080")
	fmt.Println("==================================================")
	
	// 启动服务器
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		fmt.Println("❌ 服务器启动遭遇致命错误:", err)
	}
}
