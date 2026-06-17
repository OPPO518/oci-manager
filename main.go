package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"oci-manager/service"
	"github.com/oracle/oci-go-sdk/v65/core"
)

// ================= 安全拦截器 =================
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

func loginHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if r.Method != http.MethodPost { return }

	var reqBody struct { Username, Password string }
	json.NewDecoder(r.Body).Decode(&reqBody)

	token, err := service.VerifyLogin(reqBody.Username, reqBody.Password)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"message": "登录成功", "token": token})
}

func addAccountHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if !checkAuth(w, r) { return }
	
	var req service.OCICredentials
	var body struct {
		Name, ProxyURL string
		service.OCICredentials
	}
	json.NewDecoder(r.Body).Decode(&body)

	if err := service.AddAccount(body.Name, body.ProxyURL, body.OCICredentials); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"message": "账号添加成功"})
}

func listAccountsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if !checkAuth(w, r) { return }

	accounts, err := service.ListAccounts()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"accounts": accounts})
}

func getInstancesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if !checkAuth(w, r) { return }

	accountID, _ := strconv.Atoi(r.URL.Query().Get("id"))
	instances, err := service.GetInstances(accountID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"instances": instances})
}

// 🚀 新增：电源控制拦截接口
func actionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if !checkAuth(w, r) { return }

	var req struct {
		AccountID  int    `json:"account_id"`
		InstanceID string `json:"instance_id"`
		Action     string `json:"action"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	var ociAction core.InstanceActionActionEnum
	switch req.Action {
	case "START":
		ociAction = core.InstanceActionActionStart
	case "STOP":
		ociAction = core.InstanceActionActionSoftstop // 优雅关机
	case "REBOOT":
		ociAction = core.InstanceActionActionSoftreset // 优雅重启
	default:
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "未知的电源指令"})
		return
	}

	err := service.InstanceAction(req.AccountID, req.InstanceID, ociAction)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "指令发送失败: " + err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"message": "指令已下发！状态即将更新。"})
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
	http.HandleFunc("/api/instances/action", actionHandler) // 挂载电源路由
	
	http.Handle("/", http.FileServer(http.Dir("./web")))

	fmt.Println("🚀 核心服务已成功启动！请访问: http://您的VPS公网IP:8080")
	http.ListenAndServe(":8080", nil)
}
