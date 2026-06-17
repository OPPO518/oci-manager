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

// 🚀 VNC 终极处理引擎 (包含握手重试机制)
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

	username, host, err := parseConnectionString(*console.ConnectionString)
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

	sshConfig := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), 
	}

	_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n>>> 正在等待 Oracle 底层分配通道权限 (约需 5-15 秒，请耐心等待)...\r\n"))

	var netConn net.Conn
	var sshConn ssh.Conn
	var chans <-chan ssh.NewChannel
	var reqs <-chan *ssh.Request
	var errConnect error

	// 🚀 核心修复：加入重试机制，给 Oracle 15*2=30秒 的时间注入公钥
	for i := 0; i < 15; i++ {
		netConn, errConnect = dialer.Dial("tcp", fmt.Sprintf("%s:443", host))
		if errConnect == nil {
			sshConn, chans, reqs, errConnect = ssh.NewClientConn(netConn, fmt.Sprintf("%s:443", host), sshConfig)
			if errConnect == nil {
				break // 握手成功！跳出等待循环
			}
			netConn.Close() // 握手失败，关掉网络通道准备下一次重试
		}
		time.Sleep(2 * time.Second) // 睡 2 秒再敲门
	}

	if errConnect != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n[错误] 底层网络 SSH 握手超时失败: "+errConnect.Error()+"\r\n"))
		return
	}
	defer netConn.Close()

	client := ssh.NewClient(sshConn, chans, reqs)
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n[错误] 打开底层串口会话通道失败: "+err.Error()+"\r\n"))
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

	_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n>>> 安全通道已桥接！正在打通甲骨文物理机房串口控制台... <<<\r\n\r\n"))

	go func() {
		buf := make([]byte, 2048)
		for {
			n, err := stdout.Read(buf)
			if n > 0 { _ = conn.WriteMessage(websocket.BinaryMessage, buf[:n]) }
			if err != nil { break }
		}
	}()

	for {
		msgType, msg, err := conn.ReadMessage()
		if err != nil { break }
		if msgType == websocket.TextMessage || msgType == websocket.BinaryMessage {
			_, _ = stdin.Write(msg)
		}
	}
}

func parseConnectionString(connStr string) (username, host string, err error) {
	pIdx := strings.Index(connStr, "-p 443 ")
	if pIdx == -1 { return "", "", fmt.Errorf("未匹配到标准端口 443 路由标记") }
	sub := connStr[pIdx+7:]
	quoteIdx := strings.Index(sub, "\"")
	if quoteIdx == -1 {
		quoteIdx = strings.Index(sub, "'") 
		if quoteIdx == -1 { return "", "", fmt.Errorf("连接串语法边界异常") }
	}
	targetBlock := sub[:quoteIdx] 

	atIdx := strings.Index(targetBlock, "@")
	if atIdx == -1 { return "", "", fmt.Errorf("未发现特权用户分界符") }
	username = targetBlock[:atIdx]
	host = targetBlock[atIdx+1:]
	return username, host, nil
}

// ================= 主函数启动区域 =================
func main() {
	if err := service.InitDB(); err != nil {
		fmt.Println("❌ 数据库初始化致命错误:", err)
		return
	}
	fmt.Println("✅ 扁平化安全数据库与多账号模块挂载成功！")

	http.HandleFunc("/api/login", loginHandler)
	http.HandleFunc("/api/accounts/add", addAccountHandler)
	http.HandleFunc("/api/accounts/list", listAccountsHandler)
	http.HandleFunc("/api/accounts/delete", deleteAccountHandler) 
	http.HandleFunc("/api/instances", getInstancesHandler)
	http.HandleFunc("/api/instances/action", actionHandler) 
	http.HandleFunc("/api/vnc", vncHandler) 
	
	http.Handle("/", http.FileServer(http.Dir("./web")))

	fmt.Println("🚀 核心全功能中控服务已成功启动！请访问: http://您的VPS公网IP:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Println("❌ 服务器端口监听遭遇致命碰撞错误:", err)
	}
}
