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

var upgrader = websocket.Upgrader{ CheckOrigin: func(r *http.Request) bool { return true } }

func checkAuth(w http.ResponseWriter, r *http.Request) bool {
	authHeader := r.Header.Get("Authorization")
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if !service.CheckToken(token) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "未登录"})
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
		Name, ProxyURL string
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
	if err != nil { w.WriteHeader(http.StatusInternalServerError); return }
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
	json.NewEncoder(w).Encode(map[string]interface{}{"message": "账号记录已销毁"})
}

// 🚀 新增：读取本地缓存（极速响应）
func getCacheHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if !checkAuth(w, r) { return }
	accountID, _ := strconv.Atoi(r.URL.Query().Get("id"))
	instances, _ := service.GetCachedInstances(accountID)
	json.NewEncoder(w).Encode(map[string]interface{}{"instances": instances})
}

// 🚀 新增：强制同步甲骨文云端（并触发写缓存）
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
	var req struct { AccountID int `json:"account_id"`; InstanceID, Action string `json:"instance_id"; json:"action"` }
	_ = json.NewDecoder(r.Body).Decode(&req)
	var ociAction core.InstanceActionActionEnum
	switch req.Action {
	case "START": ociAction = core.InstanceActionActionStart
	case "STOP": ociAction = core.InstanceActionActionSoftstop
	case "REBOOT": ociAction = core.InstanceActionActionSoftreset
	}
	err := service.InstanceAction(req.AccountID, req.InstanceID, ociAction)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "指令发送失败: " + err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"message": "指令已下发！"})
}

// vncHandler 完全保持上一版的完美双层嵌套穿透逻辑，不作改动
func vncHandler(w http.ResponseWriter, r *http.Request) {
	// ... (代码长度较长，为了防止超出字数，你只需知道这一段无需改动，用你当前的 vncHandler 覆盖到这里即可)
	// 由于全量替换为了保证你的直接可用性，我保留完整的 vncHandler 逻辑：
	accountID, _ := strconv.Atoi(r.URL.Query().Get("account_id"))
	instanceID := r.URL.Query().Get("instance_id")

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil { return }
	defer conn.Close()

	console, privKeyStr, err := service.CreateVNCConnection(accountID, instanceID)
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n[错误] 创建 OCI 控制台失败: "+err.Error()+"\r\n"))
		return
	}
	defer service.DeleteVNCConnection(accountID, *console.Id)

	if console.ConnectionString == nil { return }
	proxyUser, proxyHost, targetOCID, err := parseConnectionString(*console.ConnectionString)
	if err != nil { return }

	var proxyStr string
	_ = service.DB.QueryRow("SELECT proxy_url FROM oci_accounts WHERE id = ?", accountID).Scan(&proxyStr)
	proxyURL, _ := url.Parse(proxyStr)
	dialer, _ := proxy.FromURL(proxyURL, proxy.Direct)
	signer, _ := ssh.ParsePrivateKey([]byte(privKeyStr))

	proxyConfig := &ssh.ClientConfig{
		User: proxyUser, Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)}, HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n>>> 正在等待 Oracle 底层跳板机分配权限 (约需 5-15 秒)...\r\n"))
	var netConn net.Conn; var sshConn1 ssh.Conn; var chans1 <-chan ssh.NewChannel; var reqs1 <-chan *ssh.Request; var errConnect error
	for i := 0; i < 15; i++ {
		netConn, errConnect = dialer.Dial("tcp", fmt.Sprintf("%s:443", proxyHost))
		if errConnect == nil {
			sshConn1, chans1, reqs1, errConnect = ssh.NewClientConn(netConn, fmt.Sprintf("%s:443", proxyHost), proxyConfig)
			if errConnect == nil { break }
			netConn.Close()
		}
		time.Sleep(2 * time.Second)
	}
	if errConnect != nil { return }
	defer netConn.Close()

	proxyClient := ssh.NewClient(sshConn1, chans1, reqs1)
	defer proxyClient.Close()
	_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n>>> 突破跳板机成功！正在建立第二层物理串口隧道...\r\n"))

	targetAddr := targetOCID + ":22"
	forwardedNetConn, _ := proxyClient.Dial("tcp", targetAddr)
	defer forwardedNetConn.Close()

	targetConfig := &ssh.ClientConfig{
		User: targetOCID, Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)}, HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	sshConn2, chans2, reqs2, _ := ssh.NewClientConn(forwardedNetConn, targetAddr, targetConfig)
	targetClient := ssh.NewClient(sshConn2, chans2, reqs2)
	defer targetClient.Close()

	session, _ := targetClient.NewSession()
	defer session.Close()

	modes := ssh.TerminalModes{ssh.ECHO: 1, ssh.TTY_OP_ISPEED: 14400, ssh.TTY_OP_OSPEED: 14400}
	_ = session.RequestPty("xterm", 40, 100, modes)
	stdin, _ := session.StdinPipe(); stdout, _ := session.StdoutPipe()
	_ = session.Shell()

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
	targetOCID = parts[len(parts)-1] 
	pIdx := strings.Index(connStr, "-p 443 ")
	sub := connStr[pIdx+7:]
	quoteIdx := strings.Index(sub, "'")
	if quoteIdx == -1 { quoteIdx = strings.Index(sub, "\"") }
	targetBlock := sub[:quoteIdx] 
	atIdx := strings.Index(targetBlock, "@")
	proxyUser = targetBlock[:atIdx]
	proxyHost = targetBlock[atIdx+1:]
	return proxyUser, proxyHost, targetOCID, nil
}

func main() {
	if err := service.InitDB(); err != nil { fmt.Println("❌ 数据库致命错误:", err); return }
	
	// 🚀 核心：程序每次启动自动检查并静默升级数据库，添加快照抽屉
	_ = service.DB.Exec("ALTER TABLE oci_accounts ADD COLUMN cached_instances TEXT DEFAULT '[]'")

	http.HandleFunc("/api/login", loginHandler)
	http.HandleFunc("/api/accounts/add", addAccountHandler)
	http.HandleFunc("/api/accounts/list", listAccountsHandler)
	http.HandleFunc("/api/accounts/delete", deleteAccountHandler) 
	
	// 挂载两个分离的实例数据接口
	http.HandleFunc("/api/instances/cache", getCacheHandler) 
	http.HandleFunc("/api/instances/sync", syncInstancesHandler) 
	
	http.HandleFunc("/api/instances/action", actionHandler) 
	http.HandleFunc("/api/vnc", vncHandler) 
	http.Handle("/", http.FileServer(http.Dir("./web")))

	fmt.Println("🚀 中控服务全线启动 (含零延迟缓存层)！监听 :8080")
	http.ListenAndServe(":8080", nil)
}
