package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"oci-manager/service"
)

func getInstancesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	instances, err := service.GetInstances("socks5://127.0.0.1:10801")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		return
	}
	
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":   "请求成功",
		"instances": instances,
	})
}

func main() {
	// 🚀 新增：程序启动第一件事，挂载数据库引擎！
	err := service.InitDB()
	if err != nil {
		fmt.Println("❌ 数据库初始化致命错误:", err)
		return
	}
	fmt.Println("✅ SQLite 数据库与安全模块已挂载！")

	// 注册接口与网页
	http.HandleFunc("/api/instances", getInstancesHandler)
	http.Handle("/", http.FileServer(http.Dir("./web")))

	fmt.Println("🚀 核心 Web 服务已成功启动！")
	fmt.Println("👉 请打开浏览器访问: http://您的VPS公网IP:8080")
	fmt.Println("==================================================")
	
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Println("❌ 服务器启动遭遇致命错误:", err)
	}
}
