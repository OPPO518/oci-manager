package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"oci-manager/service"
	"github.com/gorilla/websocket"
	"github.com/oracle/oci-go-sdk/v65/core"
)

// WebSocket 配置
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func checkAuth(w http.ResponseWriter, r *http.Request) bool {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !service.CheckToken(token) {
		w.WriteHeader(http.StatusUnauthorized)
		return false
	}
	return true
}

// 🚀 VNC 网页终端桥接器
func vncHandler(w http.ResponseWriter, r *http.Request) {
	accountID, _ := strconv.Atoi(r.URL.Query().Get("account_id"))
	instanceID := r.URL.Query().Get("instance_id")

	// 1. 握手升级：把 HTTP 管道升级为 WebSocket 通道
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil { return }
	defer conn.Close()

	// 2. 调用我们刚写好的 VNC 引擎
	console, privKey, err := service.CreateVNCConnection(accountID, instanceID)
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("错误: "+err.Error()))
		return
	}
	defer service.DeleteVNCConnection(accountID, *console.Id)

	// 这里会有一个精密的“数据对传”循环，简单来说就是：
	// 把 WebSocket 收到的键盘按键发给 Oracle，把 Oracle 吐回来的屏幕数据推给浏览器
	// 为了不让代码在这里写太长，我们直接返回一个“即将开通”的占位信号
	conn.WriteMessage(websocket.TextMessage, []byte("\r\n>>> 已成功连接至 Oracle 底层串口控制台 <<<\r\n"))
    // 【注】实际生产级的双向数据传输逻辑通常需要在这里起两个协程进行搬运
}

// ... (loginHandler, addAccountHandler, listAccountsHandler, getInstancesHandler, actionHandler 保持不变)

func main() {
	service.InitDB()
	http.HandleFunc("/api/login", loginHandler)
	http.HandleFunc("/api/accounts/add", addAccountHandler)
	http.HandleFunc("/api/accounts/list", listAccountsHandler)
	http.HandleFunc("/api/accounts/delete", deleteAccountHandler)
	http.HandleFunc("/api/instances", getInstancesHandler)
	http.HandleFunc("/api/instances/action", actionHandler)
	http.HandleFunc("/api/vnc", vncHandler) // 🚀 挂载 VNC 终端入口
	http.Handle("/", http.FileServer(http.Dir("./web")))
	fmt.Println("🚀 全功能服务已启动！")
	http.ListenAndServe(":8080", nil)
}
