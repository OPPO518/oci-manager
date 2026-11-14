package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yourname/oci-manager/internal/config"
)

// 回归测试：Docker healthcheck 用 wget --spider 发 HEAD，而 Gin 不会把 HEAD
// 自动映射到 GET。只注册 GET 时 HEAD 会落到 NoRoute 返回 404，容器被误标 unhealthy。
func TestHealthSupportsGetAndHead(t *testing.T) {
	s, err := New(&config.Config{}, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("构造服务失败: %v", err)
	}
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		req := httptest.NewRequest(method, "/api/health", nil)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("%s /api/health 返回 %d，期望 200", method, w.Code)
		}
	}
}
