package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"oci-manager/service"
)

// ================= 安全拦截器 =================
// 所有的敏感接口在执行前，都会先过这一关，检查 Token
func checkAuth(w http.ResponseWriter, r *http.Request) bool {
	authHeader := r.Header.Get("Authorization")
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if !service.CheckToken(token) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "未登录或登录已过期，请重新登录！"})
		return false
	}
	return true
}

// ================= API 接口区域 =================

// 1. 登录接口 (保持不变)
func loginHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if r.Method != http.MethodPost {
		http.Error(w, `{"error": "只允许 POST 请求"}`, http.StatusMethodNotAllowed)
		return
	}

	var reqBody struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		http.Error(w, `{"error": "数据格式错误"}`, http.StatusBadRequest)
		return
	}

	token, err := service.VerifyLogin(reqBody.Username, reqBody.Password)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"message": "登录成功", "token": token})
}

// 2. 录入新账号接口
func addAccountHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if !checkAuth(w, r) { return }
	
	var req struct {
		Name        string `json:"name"`
		ProxyURL    string `json:"proxy_url"`
		Tenancy     string `json:"tenancy"`
		User        string `json:"user"`
		Region      string `json:"region"`
		Fingerprint string `json:"fingerprint"`
		PrivateKey  string `json:"private_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "数据格式错误"})
		return
	}

	creds := service.OCICredentials{
		Tenancy:     req.Tenancy,
		User:        req.User,
		Region:      req.Region,
		Fingerprint: req.Fingerprint,
		PrivateKey:  req.PrivateKey,
	}

	if err := service.AddAccount(req.Name, req.ProxyURL, creds); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "保存账号失败: " + err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"message": "账号添加成功"})
}

// 3. 获取已有账号列表接口
func listAccountsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if !checkAuth(w, r) { return }

	accounts, err := service.ListAccounts()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "获取列表失败: " + err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"accounts": accounts})
}

// 4. 按账号 ID 获取服务器列表 (动态连机核心)
func getInstancesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if !checkAuth(w, r) { return }

	// 从 URL 参数里提取想查的账号 ID (比如 /api/instances?id=1)
	accountIDStr := r.URL.Query().Get("id")
	accountID, err := strconv.Atoi(accountIDStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "无效的账号 ID"})
		return
	}

	instances, err := service.GetInstances(accountID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"message": "请求成功", "instances": instances})
}

// ================= 主函数启动区域 =================
func main() {
	if err := service.InitDB(); err != nil {
		fmt.Println("❌ 数据库初始化致命错误:", err)
		return
	}
	fmt.Println("✅ 数据库与多账号安全模块挂载成功！")

	http.HandleFunc("/api/login", loginHandler)
	http.HandleFunc("/api/accounts/add", addAccountHandler)
	http.HandleFunc("/api/accounts/list", listAccountsHandler)
	http.HandleFunc("/api/instances", getInstancesHandler)
	
	http.Handle("/", http.FileServer(http.Dir("./web")))

	fmt.Println("🚀 核心服务已成功启动！请访问: http://您的VPS公网IP:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Println("❌ 服务器启动遭遇致命错误:", err)
	}
}
