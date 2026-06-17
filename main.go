package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"oci-manager/service"
	"github.com/gorilla/websocket"
	"github.com/oracle/oci-go-sdk/v65/core"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/proxy"
)

// WebSocket 升级器配置
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

// ================= API 接口区域 =================

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

// 🚀 终极硬核：无缝中转 VNC 数据流的 WebSocket 桥接处理器
func vncHandler(w http.ResponseWriter, r *http.Request) {
	accountID, _ := strconv.Atoi(r.URL.Query().Get("account_id"))
	instanceID := r.URL.Query().Get("instance_id")

	// 1. 握手升级为 WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil { return }
	defer conn.Close()

	// 2. 调用 VNC 引擎创建云端连接并拿到内存临时私钥
	console, privKeyStr, err := service.CreateVNCConnection(accountID, instanceID)
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n[错误] 创建 OCI 控制台连接失败: "+err.Error()+"\r\n"))
		return
	}
	// 确保用户断开 Web 界面时，自动销毁云端的独占控制台通道，防不留痕迹的安全隐患
	defer service.DeleteVNCConnection(accountID, *console.Id)

	if console.ConnectionString == nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n[错误] Oracle 未返回有效的连接字符串\r\n"))
		return
	}

	// 3. 深度解析甲骨文复杂的 SSH 代理命令字串
	username, host, err := parseConnectionString(*console.ConnectionString)
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n[错误] 解析连接字符串失败: "+err.Error()+"\r\n"))
		return
	}

	// 4. 从数据库提取当前账号绑定的专属代理，防止 VPS 公网 IP 暴露泄密
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

	// 5. 劫持原生 TCP 拨号，强行注入 SOCKS5 代理网络建立安全连接
	dialer, err := proxy.FromURL(proxyURL, proxy.Direct)
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n[错误] 初始化代理拨号器失败: "+err.Error()+"\r\n"))
		return
	}

	netConn, err := dialer.Dial("tcp", fmt.Sprintf("%s:443", host))
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n[错误] 通过代理物理隔离通道连接终端服务器失败: "+err.Error()+"\r\n"))
		return
	}
	defer netConn.Close()

	// 6. 配置底层客户端 SSH 握手协议
	signer, err := ssh.ParsePrivateKey([]byte(privKeyStr))
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n[错误] 内存解析临时私钥失败: "+err.Error()+"\r\n"))
		return
	}

	sshConfig := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // 串口网关为动态指纹，强制放行
	}

	// 7. 在代理隧道内部打通 SSH 二层会话
	sshConn, chans, reqs, err := ssh.NewClientConn(netConn, fmt.Sprintf("%s:443", host), sshConfig)
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n[错误] 建立底层网络 SSH 握手失败: "+err.Error()+"\r\n"))
		return
	}
	client := ssh.NewClient(sshConn, chans, reqs)
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n[错误] 打开底层串口会话通道失败: "+err.Error()+"\r\n"))
		return
	}
	defer session.Close()

	// 请求分配伪终端伪装成标准的 xterm 模式
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

	// 8. 真正的双向全双工异步搬运工（零占位符，纯字节流转发）
	_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n>>> 安全通道已桥接！正在打通甲骨文物理机房串口控制台... <<<\r\n\r\n"))

	// 协程 A：将 Oracle 底层丢出来的物理主板画面 ➔ 毫秒级灌入用户的 Web 网页
	go func() {
		buf := make([]byte, 2048)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				_ = conn.WriteMessage(websocket.BinaryMessage, buf[:n])
			}
			if err != nil { break }
		}
	}()

	// 协程 B（主线程）：将用户在网页浏览器上敲击键盘的动作 ➔ 毫秒级灌入甲骨文物理串口
	for {
		msgType, msg, err := conn.ReadMessage()
		if err != nil { break }
		if msgType == websocket.TextMessage || msgType == websocket.BinaryMessage {
			_, _ = stdin.Write(msg)
		}
	}
}

// 辅助工具函数：精准解构甲骨文反人类的 ProxyCommand 连接字符串
func parseConnectionString(connStr string) (username, host string, err error) {
	// 典型格式: ssh -o ProxyCommand="ssh -W %h:%p -p 443 ocid1.instanceconsoleconnection...@instance-console.ap-seoul-1.oraclecloud.com" ocid1.instance...
	pIdx := strings.Index(connStr, "-p 443 ")
	if pIdx == -1 {
		return "", "", fmt.Errorf("未匹配到标准端口 443 路由标记")
	}
	sub := connStr[pIdx+7:]
	quoteIdx := strings.Index(sub, "\"")
	if quoteIdx == -1 {
		quoteIdx = strings.Index(sub, "'") // 兼容单引号解析
		if quoteIdx == -1 {
			return "", "", fmt.Errorf("连接串语法边界异常")
		}
	}
	targetBlock := sub[:quoteIdx] // 提取出: ocid1.xxx@instance-console.xxx

	atIdx := strings.Index(targetBlock, "@")
	if atIdx == -1 {
		return "", "", fmt.Errorf("未发现特权用户分界符")
	}
	username = targetBlock[:atIdx]
	host = targetBlock[atIdx+1:]
	return username, host, nil
}

// ================= 主函数启动区域 =================
func main() {
	// 严格检查初始化错误，保障系统不带病上线
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
	http.HandleFunc("/api/vnc", vncHandler) // 挂载终极网络武器接口
	
	http.Handle("/", http.FileServer(http.Dir("./web")))

	fmt.Println("🚀 核心全功能中控服务已成功启动！请访问: http://您的VPS公网IP:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Println("❌ 服务器端口监听遭遇致命碰撞错误:", err)
	}
}
