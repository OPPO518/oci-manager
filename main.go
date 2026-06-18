package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"oci-manager/service"
	"github.com/gorilla/websocket"
	"github.com/oracle/oci-go-sdk/v65/core"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/proxy"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

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

// ================= 基础 API 接口区域 =================

func loginHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if r.Method != http.MethodPost { return }

	var reqBody struct { Username, Password string }
	_ = json.NewDecoder(r.Body).Decode(&reqBody)

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
	
	var body struct {
		Name     string `json:"name"`
		ProxyURL string `json:"proxy_url"`
		service.OCICredentials
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

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

func deleteAccountHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if !checkAuth(w, r) { return }

	var req struct { ID int `json:"id"` }
	_ = json.NewDecoder(r.Body).Decode(&req)

	if err := service.DeleteAccount(req.ID); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "删除失败: " + err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"message": "账号记录已安全销毁"})
}

// ================= OCI 实例与缓存 API =================

func getCacheHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if !checkAuth(w, r) { return }

	accountID, _ := strconv.Atoi(r.URL.Query().Get("id"))
	instances, err := service.GetCachedInstances(accountID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"instances": instances})
}

func syncInstancesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if !checkAuth(w, r) { return }

	accountID, _ := strconv.Atoi(r.URL.Query().Get("id"))
	instances, err := service.SyncInstances(accountID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"instances": instances})
}

func actionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if !checkAuth(w, r) { return }

	var req struct {
		AccountID  int    `json:"account_id"`
		InstanceID string `json:"instance_id"`
		Action     string `json:"action"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	var ociAction core.InstanceActionActionEnum
	switch req.Action {
	case "START": ociAction = core.InstanceActionActionStart
	case "STOP": ociAction = core.InstanceActionActionSoftstop
	case "REBOOT": ociAction = core.InstanceActionActionSoftreset
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

// 🚀 新增：Xray 代理工厂核心唤醒 API
func startProxyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if !checkAuth(w, r) { return }

	var req service.ProxyConfig
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "解析节点档案参数失败"})
		return
	}

	proxyURL, err := service.StartXrayProcess(req)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"proxy_url": proxyURL, 
		"message": "底层 Xray 通道已打通！",
	})
}

// ================= VNC 双重隧道引擎 (保持不变) =================
func vncHandler(w http.ResponseWriter, r *http.Request) {
	accountID, _ := strconv.Atoi(r.URL.Query().Get("account_id"))
	instanceID := r.URL.Query().Get("instance_id")

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil { return }
	defer conn.Close()

	console, privKeyStr, err := service.CreateVNCConnection(accountID, instanceID)
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n[错误] 创建 OCI 控制台连接失败: "+err.Error()+"\r\n"))
		return
	}
	defer service.DeleteVNCConnection(accountID, *console.Id)

	if console.ConnectionString == nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n[错误] Oracle 未返回有效的连接字符串\r\n"))
		return
	}

	proxyUser, proxyHost, targetOCID, err := parseConnectionString(*console.ConnectionString)
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n[错误] 解析连接字符串失败: "+err.Error()+"\r\n"))
		return
	}

	var proxyStr string
	err = service.DB.QueryRow("SELECT proxy_url FROM oci_accounts WHERE id = ?", accountID).Scan(&proxyStr)
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n[错误] 读取账号隔离代理配置失败\r\n"))
		return
	}

	proxyURL, err := url.Parse(proxyStr)
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n[错误] 代理 URL 解析失败\r\n"))
		return
	}

	dialer, err := proxy.FromURL(proxyURL, proxy.Direct)
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n[错误] 初始化代理拨号器失败: "+err.Error()+"\r\n"))
		return
	}

	signer, err := ssh.ParsePrivateKey([]byte(privKeyStr))
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n[错误] 内存解析临时私钥失败: "+err.Error()+"\r\n"))
		return
	}

	proxyConfig := &ssh.ClientConfig{
		User:            proxyUser,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n>>> 正在等待 Oracle 底层跳板机分配权限 (约需 5-15 秒)...\r\n"))

	var netConn net.Conn
	var sshConn1 ssh.Conn
	var chans1 <-chan ssh.NewChannel
	var reqs1 <-chan *ssh.Request
	var errConnect error

	for i := 0; i < 15; i++ {
		netConn, errConnect = dialer.Dial("tcp", fmt.Sprintf("%s:443", proxyHost))
		if errConnect == nil {
			sshConn1, chans1, reqs1, errConnect = ssh.NewClientConn(netConn, fmt.Sprintf("%s:443", proxyHost), proxyConfig)
			if errConnect == nil { break }
			netConn.Close()
		}
		time.Sleep(2 * time.Second)
	}

	if errConnect != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n[错误] 第一层跳板机网络 SSH 握手超时失败: "+errConnect.Error()+"\r\n"))
		return
	}
	defer netConn.Close()

	proxyClient := ssh.NewClient(sshConn1, chans1, reqs1)
	defer proxyClient.Close()
	
	_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n>>> 突破跳板机成功！正在建立第二层物理串口隧道...\r\n"))

	targetAddr := targetOCID + ":22"
	forwardedNetConn, err := proxyClient.Dial("tcp", targetAddr)
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n[错误] 申请第二层串口隧道失败: "+err.Error()+"\r\n"))
		return
	}
	defer forwardedNetConn.Close()

	targetConfig := &ssh.ClientConfig{
		User:            targetOCID,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	sshConn2, chans2, reqs2, err := ssh.NewClientConn(forwardedNetConn, targetAddr, targetConfig)
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n[错误] 第二层物理串口握手失败: "+err.Error()+"\r\n"))
		return
	}
	targetClient := ssh.NewClient(sshConn2, chans2, reqs2)
	defer targetClient.Close()

	session, err := targetClient.NewSession()
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n[错误] 打开底层串口控制台失败: "+err.Error()+"\r\n"))
		return
	}
	defer session.Close()

	modes := ssh.TerminalModes{ssh.ECHO: 1, ssh.TTY_OP_ISPEED: 14400, ssh.TTY_OP_OSPEED: 14400}
	if err := session.RequestPty("xterm", 40, 100, modes); err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n[错误] 请求分配虚拟终端失败: "+err.Error()+"\r\n"))
		return
	}

	stdin, _ := session.StdinPipe()
	stdout, _ := session.StdoutPipe()

	if err := session.Shell(); err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n[错误] 唤醒主板串口控制台失败: "+err.Error()+"\r\n"))
		return
	}

	_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n>>> 🚀 物理机房串口通道全线贯通！请敲击 Enter 唤醒终端 <<<\r\n\r\n"))

	go func() {
		buf := make([]byte, 2048)
		for { n, err := stdout.Read(buf); if n > 0 { _ = conn.WriteMessage(websocket.BinaryMessage, buf[:n]) }; if err != nil { break } }
	}()

	for {
		msgType, msg, err := conn.ReadMessage()
		if err != nil { break }
		if msgType == websocket.TextMessage || msgType == websocket.BinaryMessage { _, _ = stdin.Write(msg) }
	}
}

func parseConnectionString(connStr string) (proxyUser, proxyHost, targetOCID string, err error) {
	connStr = strings.TrimSpace(connStr)
	parts := strings.Split(connStr, " ")
	if len(parts) == 0 { return "", "", "", fmt.Errorf("连接串为空") }
	targetOCID = parts[len(parts)-1]

	pIdx := strings.Index(connStr, "-p 443 ")
	if pIdx == -1 { return "", "", "", fmt.Errorf("未匹配到标准端口 443 路由标记") }
	sub := connStr[pIdx+7:]
	quoteIdx := strings.Index(sub, "'")
	if quoteIdx == -1 {
		quoteIdx = strings.Index(sub, "\"")
		if quoteIdx == -1 { return "", "", "", fmt.Errorf("连接串语法边界异常") }
	}
	targetBlock := sub[:quoteIdx]

	atIdx := strings.Index(targetBlock, "@")
	if atIdx == -1 { return "", "", "", fmt.Errorf("未发现特权用户分界符") }
	proxyUser = targetBlock[:atIdx]
	proxyHost = targetBlock[atIdx+1:]
	return proxyUser, proxyHost, targetOCID, nil
}

// ================= 主函数启动区域 =================
func main() {
	if err := service.InitDB(); err != nil {
		fmt.Println("❌ 数据库初始化致命错误:", err)
		return
	}

	_, _ = service.DB.Exec("ALTER TABLE oci_accounts ADD COLUMN cached_instances TEXT DEFAULT '[]'")
	fmt.Println("✅ 扁平化安全数据库与多账号模块挂载成功！")

	http.HandleFunc("/api/login", loginHandler)
	http.HandleFunc("/api/accounts/add", addAccountHandler)
	http.HandleFunc("/api/accounts/list", listAccountsHandler)
	http.HandleFunc("/api/accounts/delete", deleteAccountHandler)
	
	http.HandleFunc("/api/instances/cache", getCacheHandler)
	http.HandleFunc("/api/instances/sync", syncInstancesHandler)
	http.HandleFunc("/api/instances/action", actionHandler)
	
	// 🚀 挂载最新的代理唤醒路由
	http.HandleFunc("/api/proxy/start", startProxyHandler)
	
	http.HandleFunc("/api/vnc", vncHandler)
	http.Handle("/", http.FileServer(http.Dir("./web")))

	fmt.Println("🚀 核心全功能中控服务 (含自动化 Xray 工厂) 已成功启动！请访问: http://您的VPS公网IP:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Println("❌ 服务器端口监听遭遇致命碰撞错误:", err)
	}
}
