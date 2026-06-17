package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"oci-manager/service"
)

// ================= API 接口区域 =================

// 1. 登录接口
func loginHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	
	if r.Method != http.MethodPost {
		http.Error(w, `{"error": "只允许 POST 请求"}`, http.StatusMethodNotAllowed)
		return
	}

	// 解析前端传过来的 JSON 账号密码
	var reqBody struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		http.Error(w, `{"error": "数据格式错误"}`, http.StatusBadRequest)
		return
	}

	// 呼叫鉴权服务
	token, err := service.VerifyLogin(reqBody.Username, reqBody.Password)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		return
	}

	// 登录成功，把 Token 发给前端
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "登录成功",
		"token":   token,
	})
}

// 2. 获取机器列表接口 (已加锁)
func getInstancesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	// 🛑 核心保安逻辑：检查前端有没有带合法的 Token 过来
	authHeader := r.Header.Get("Authorization")
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if !service.CheckToken(token) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "未登录或登录已过期，请重新登录！"})
		return
	}

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

// ================= 主函数启动区域 =================
func main() {
	// 初始化数据库
	if err := service.InitDB(); err != nil {
		fmt.Println("❌ 数据库初始化致命错误:", err)
		return
	}
	fmt.Println("✅ 数据库与安全模块挂载成功！")

	// 注册路由
	http.HandleFunc("/api/login", loginHandler)
	http.HandleFunc("/api/instances", getInstancesHandler)
	http.Handle("/", http.FileServer(http.Dir("./web")))

	fmt.Println("🚀 核心服务已成功启动！请访问: http://您的VPS公网IP:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Println("❌ 服务器启动遭遇致命错误:", err)
	}
}
