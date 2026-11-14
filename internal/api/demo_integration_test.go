package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/yourname/oci-manager/internal/auth"
	"github.com/yourname/oci-manager/internal/config"
	"github.com/yourname/oci-manager/internal/notify"
	"github.com/yourname/oci-manager/internal/oci"
	"github.com/yourname/oci-manager/internal/secret"
	"github.com/yourname/oci-manager/internal/store"
)

// demoTestServer 构造一个完整但隔离的测试服务：临时 SQLite、真实鉴权链路、
// 未配置 Telegram（因此演示模式是唯一可完成写操作的路径）。
func demoTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	key := []byte("0123456789abcdef0123456789abcdef") // 32 bytes
	sealer, err := secret.NewSealer(key)
	if err != nil {
		t.Fatalf("创建加密器失败: %v", err)
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"), sealer)
	if err != nil {
		t.Fatalf("打开测试数据库失败: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	pool := oci.NewPool(st)
	notifySvc := notify.New(st)
	authSvc := auth.New(st, notifySvc)
	if err := authSvc.Setup("admin", "password123"); err != nil {
		t.Fatalf("初始化管理员失败: %v", err)
	}
	token, err := authSvc.CreateSession("admin")
	if err != nil {
		t.Fatalf("创建会话失败: %v", err)
	}
	srv, err := New(&config.Config{}, st, pool, nil, authSvc, notifySvc, nil)
	if err != nil {
		t.Fatalf("构造服务失败: %v", err)
	}
	return srv, token
}

func demoRequest(t *testing.T, srv *Server, token, method, path string, body any) (int, []byte) {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("序列化请求体失败: %v", err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	return w.Code, w.Body.Bytes()
}

func decodeJSON(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("响应不是 JSON 对象: %v / %s", err, string(raw))
	}
	return out
}

func decodeList(t *testing.T, raw []byte) []map[string]any {
	t.Helper()
	var out []map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("响应不是 JSON 数组: %v / %s", err, string(raw))
	}
	return out
}

// demoOTP 申请一次演示验证码，返回可直接放进业务请求的 otp 字段。
func demoOTP(t *testing.T, srv *Server, token, operation string) map[string]string {
	t.Helper()
	code, raw := demoRequest(t, srv, token, http.MethodPost, "/api/otp/request",
		map[string]any{"operation": operation, "summary": "集成测试：" + operation, "demo": true})
	if code != http.StatusOK {
		t.Fatalf("申请验证码 %s 返回 %d: %s", operation, code, string(raw))
	}
	res := decodeJSON(t, raw)
	if res["demo_code"] != "123456" {
		t.Fatalf("演示验证码缺失: %s", string(raw))
	}
	return map[string]string{"challenge": res["challenge"].(string), "code": "123456"}
}

const demoInstanceID = "ocid1.instance.oc1.ap-tokyo-1.demo001"

// TestDemoAccountNeverCallsRealOCI 覆盖演示账号的全部主要接口。
// 演示 ClientSet 的所有 SDK 客户端都是 nil，任何漏短路都会 panic 并返回 500，
// 因此「全部返回预期状态码」本身就是零真实 OCI 请求的强证明。
func TestDemoAccountNeverCallsRealOCI(t *testing.T) {
	srv, token := demoTestServer(t)

	// 1. 账号列表：演示账号排在首位并带 is_demo 标记
	code, raw := demoRequest(t, srv, token, http.MethodGet, "/api/accounts", nil)
	if code != http.StatusOK {
		t.Fatalf("账号列表返回 %d: %s", code, string(raw))
	}
	accounts := decodeList(t, raw)
	if len(accounts) == 0 || accounts[0]["id"] != store.DemoAccountID || accounts[0]["is_demo"] != true {
		t.Fatalf("演示账号未出现在列表首位: %s", string(raw))
	}

	// 2. 只读接口全部应可用且不能出现 500（500 意味着误触真实 SDK 客户端）
	readPaths := []string{
		"/api/accounts/demo-account/regions",
		"/api/accounts/demo-account/compartments",
		"/api/accounts/demo-account/availability-domains",
		"/api/accounts/demo-account/subnets",
		"/api/accounts/demo-account/images",
		"/api/accounts/demo-account/instances",
		"/api/accounts/demo-account/cost",
		"/api/accounts/demo-account/overview",
		"/api/accounts/demo-account/boot-volumes",
		"/api/accounts/demo-account/vnics",
		"/api/accounts/demo-account/security-lists",
		"/api/accounts/demo-account/public-ips",
		"/api/accounts/demo-account/instances/" + demoInstanceID + "/detail",
		"/api/settings/demo",
		"/api/settings/ip-rotation",
		"/api/settings/iam",
		"/api/accounts/demo-account/iam/overview",
		"/api/audit-logs",
	}
	for _, path := range readPaths {
		code, raw := demoRequest(t, srv, token, http.MethodGet, path, nil)
		if code != http.StatusOK {
			t.Errorf("GET %s 返回 %d: %s", path, code, string(raw))
		}
	}

	// 3. 容量探测（只读）
	code, raw = demoRequest(t, srv, token, http.MethodPost, "/api/accounts/demo-account/capacity",
		map[string]any{
			"region": "ap-tokyo-1", "shape": "VM.Standard.A1.Flex",
			"ocpus": 4, "memory_gbs": 24,
			"availability_domains": []string{"NISh:AP-TOKYO-1-AD-1"},
		})
	if code != http.StatusOK {
		t.Fatalf("容量探测返回 %d: %s", code, string(raw))
	}

	// 4. 演示账号凭证测试也应走模拟数据（不发真实请求）
	code, raw = demoRequest(t, srv, token, http.MethodPost, "/api/accounts/demo-account/test", nil)
	if code != http.StatusOK {
		t.Errorf("演示账号连通性测试返回 %d: %s", code, string(raw))
	}
}

// TestDemoWriteOperations 验证演示模式下的写操作、OTP 一次性消费与服务端禁止清单。
func TestDemoWriteOperations(t *testing.T) {
	srv, token := demoTestServer(t)

	// 实例改名并回读确认状态确实可变
	code, raw := demoRequest(t, srv, token, http.MethodPut,
		"/api/accounts/demo-account/instances/"+demoInstanceID+"/display-name",
		map[string]any{"display_name": "renamed-by-test"})
	if code != http.StatusOK {
		t.Fatalf("实例改名返回 %d: %s", code, string(raw))
	}
	code, raw = demoRequest(t, srv, token, http.MethodGet, "/api/accounts/demo-account/instances", nil)
	if code != http.StatusOK {
		t.Fatalf("回读实例失败: %d", code)
	}
	found := false
	for _, inst := range decodeList(t, raw) {
		if inst["display_name"] == "renamed-by-test" {
			found = true
		}
	}
	if !found {
		t.Fatal("实例改名没有反映在后续读取上")
	}

	// 引导卷 / VNIC / 安全列表更新
	_, volumesRaw := demoRequest(t, srv, token, http.MethodGet, "/api/accounts/demo-account/boot-volumes", nil)
	volumes := decodeList(t, volumesRaw)
	if len(volumes) == 0 {
		t.Fatal("演示数据缺少引导卷")
	}
	code, raw = demoRequest(t, srv, token, http.MethodPut,
		"/api/accounts/demo-account/boot-volumes/"+volumes[0]["id"].(string),
		map[string]any{"display_name": "renamed-volume"})
	if code != http.StatusOK {
		t.Errorf("引导卷更新返回 %d: %s", code, string(raw))
	}

	_, vnicsRaw := demoRequest(t, srv, token, http.MethodGet, "/api/accounts/demo-account/vnics", nil)
	vnics := decodeList(t, vnicsRaw)
	if len(vnics) == 0 {
		t.Fatal("演示数据缺少 VNIC")
	}
	code, raw = demoRequest(t, srv, token, http.MethodPut,
		"/api/accounts/demo-account/vnics/"+vnics[0]["id"].(string),
		map[string]any{"display_name": "renamed-vnic"})
	if code != http.StatusOK {
		t.Errorf("VNIC 更新返回 %d: %s", code, string(raw))
	}

	// 换 IP：OTP 二次确认 + 冷却/次数护栏
	_, ipsRaw := demoRequest(t, srv, token, http.MethodGet, "/api/accounts/demo-account/public-ips", nil)
	ips := decodeList(t, ipsRaw)
	if len(ips) == 0 {
		t.Fatal("演示数据缺少公网 IP")
	}
	privateIPID := ips[0]["private_ip_id"].(string)
	rotateBody := map[string]any{
		"private_ip_id": privateIPID,
		"otp":           demoOTP(t, srv, token, "rotate_public_ip"),
	}
	code, raw = demoRequest(t, srv, token, http.MethodPost,
		"/api/accounts/demo-account/public-ips/rotate", rotateBody)
	if code != http.StatusOK {
		t.Fatalf("换 IP 返回 %d: %s", code, string(raw))
	}
	if decodeJSON(t, raw)["new_ip"] == nil {
		t.Fatal("换 IP 未返回新地址")
	}
	// 同一个 challenge 重放必须被拒绝（一次性消费）
	code, _ = demoRequest(t, srv, token, http.MethodPost,
		"/api/accounts/demo-account/public-ips/rotate", rotateBody)
	if code == http.StatusOK {
		t.Fatal("操作验证码被重放成功，存在越权风险")
	}

	// IAM 模块开启
	code, raw = demoRequest(t, srv, token, http.MethodPut, "/api/settings/iam",
		map[string]any{"enabled": true, "mode": "classic", "home_region": "ap-tokyo-1"})
	if code != http.StatusOK {
		t.Fatalf("开启 IAM 模块返回 %d: %s", code, string(raw))
	}

	// 创建用户
	code, raw = demoRequest(t, srv, token, http.MethodPost, "/api/accounts/demo-account/iam/users",
		map[string]any{
			"name": "test-user", "description": "集成测试", "email": "test@example.com",
			"otp": demoOTP(t, srv, token, "iam_create_user"),
		})
	if code != http.StatusCreated {
		t.Fatalf("创建 IAM 用户返回 %d: %s", code, string(raw))
	}
	newUser := decodeJSON(t, raw)["user"].(map[string]any)["id"].(string)

	// 重置一次性密码：响应含密码，审计日志里不能出现密码
	code, raw = demoRequest(t, srv, token, http.MethodPost,
		"/api/accounts/demo-account/iam/users/"+newUser+"/password",
		map[string]any{"otp": demoOTP(t, srv, token, "iam_reset_password")})
	if code != http.StatusOK {
		t.Fatalf("重置密码返回 %d: %s", code, string(raw))
	}
	oneTimePassword := decodeJSON(t, raw)["password"].(string)
	if oneTimePassword == "" {
		t.Fatal("未返回一次性密码")
	}

	// 服务端硬禁止：Administrators 组成员变更
	code, raw = demoRequest(t, srv, token, http.MethodPost,
		"/api/accounts/demo-account/iam/groups/ocid1.group.oc1..demo-admins/members",
		map[string]any{
			"user_id": newUser,
			"otp":     demoOTP(t, srv, token, "iam_add_member"),
		})
	if code != http.StatusForbidden {
		t.Errorf("Administrators 组未被拦截，返回 %d: %s", code, string(raw))
	}

	// 服务端硬禁止：操作面板自用 API 用户
	code, raw = demoRequest(t, srv, token, http.MethodDelete,
		"/api/accounts/demo-account/iam/users/ocid1.user.oc1..demo-api-user",
		map[string]any{"otp": demoOTP(t, srv, token, "iam_delete_user")})
	if code != http.StatusForbidden {
		t.Errorf("面板自用用户未被拦截，返回 %d: %s", code, string(raw))
	}

	// 审计日志有记录，且不含一次性密码
	code, raw = demoRequest(t, srv, token, http.MethodGet, "/api/audit-logs?limit=200", nil)
	if code != http.StatusOK {
		t.Fatalf("审计日志返回 %d", code)
	}
	if bytes.Contains(raw, []byte(oneTimePassword)) {
		t.Fatal("一次性密码被写进了审计日志")
	}
	if len(decodeList(t, raw)) == 0 {
		t.Fatal("审计日志为空")
	}
}

// TestOperationOTPOneTimeOnly 锁住操作验证码的一次性消费与操作绑定。
func TestOperationOTPOneTimeOnly(t *testing.T) {
	srv, token := demoTestServer(t)

	otp := demoOTP(t, srv, token, "iam_create_user")
	if err := srv.auth.VerifyOperationOTP(otp["challenge"], otp["code"], "iam_create_user", true); err != nil {
		t.Fatalf("首次校验失败: %v", err)
	}
	if err := srv.auth.VerifyOperationOTP(otp["challenge"], otp["code"], "iam_create_user", true); err == nil {
		t.Fatal("同一个验证码被重复消费成功")
	}

	cross := demoOTP(t, srv, token, "iam_delete_user")
	if err := srv.auth.VerifyOperationOTP(cross["challenge"], cross["code"], "iam_create_user", true); err == nil {
		t.Fatal("删除用户的验证码竟然能用于创建用户")
	}

	// 操作挑战不能拿去换登录会话
	loginChallenge := demoOTP(t, srv, token, "rotate_public_ip")
	code, raw := demoRequest(t, srv, token, http.MethodPost, "/api/auth/otp",
		map[string]any{"challenge": loginChallenge["challenge"], "code": loginChallenge["code"]})
	if code == http.StatusOK {
		t.Fatalf("操作挑战被用来换登录会话: %s", string(raw))
	}
}

// TestDemoChallengeCannotAuthorizeRealAccount 锁住「演示固定码不能用于真实账号写操作」。
func TestDemoChallengeCannotAuthorizeRealAccount(t *testing.T) {
	srv, token := demoTestServer(t)
	// 演示挑战的类型是 "opd:"；真实账号校验期望 "op:"，因此同一 challenge
	// 在 account.IsDemo == false 时必须被拒绝。
	challenge := demoOTP(t, srv, token, "iam_create_user")
	if err := srv.auth.VerifyOperationOTP(challenge["challenge"], challenge["code"], "iam_create_user", false); err == nil {
		t.Fatal("演示验证码竟然通过了真实账号的校验")
	}
}
