package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/yourname/oci-manager/internal/logbus"
)

// logStream 通过 SSE 把实时日志推送到前端「日志查看」页面。
//
// 连接建立时先回放最近 200 条历史，之后持续推送增量。支持 ?source= 与
// ?level= 过滤（逗号分隔），减少无关流量。
func (s *Server) logStream(c *gin.Context) {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		respondError(c, jsonErr("当前环境不支持流式响应"))
		return
	}

	sourceFilter := splitFilter(c.Query("source"))
	levelFilter := splitFilter(c.Query("level"))

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	ch := make(chan logbus.Entry, 128)
	unsub := logbus.Default.Subscribe(ch)
	defer unsub()

	// 先回放历史
	for _, e := range logbus.Default.Recent(200) {
		if !matchLogFilter(e, sourceFilter, levelFilter) {
			continue
		}
		writeSSE(c, e)
	}
	flusher.Flush()

	ctx := c.Request.Context()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case e := <-ch:
			if !matchLogFilter(e, sourceFilter, levelFilter) {
				continue
			}
			writeSSE(c, e)
			flusher.Flush()
		case <-heartbeat.C:
			c.Writer.WriteString(": keep-alive\n\n")
			flusher.Flush()
		}
	}
}

func writeSSE(c *gin.Context, e logbus.Entry) {
	data, err := json.Marshal(e)
	if err != nil {
		return
	}
	c.Writer.WriteString("data: " + string(data) + "\n\n")
}

func splitFilter(v string) map[string]struct{} {
	if v == "" {
		return nil
	}
	out := make(map[string]struct{})
	for _, p := range splitComma(v) {
		out[p] = struct{}{}
	}
	return out
}

func splitComma(v string) []string {
	var out []string
	curr := ""
	for _, r := range v {
		switch r {
		case ',', ' ', ';':
			if curr != "" {
				out = append(out, curr)
				curr = ""
			}
		default:
			curr += string(r)
		}
	}
	if curr != "" {
		out = append(out, curr)
	}
	return out
}

func matchLogFilter(e logbus.Entry, sources, levels map[string]struct{}) bool {
	if sources != nil {
		if _, ok := sources[string(e.Source)]; !ok {
			return false
		}
	}
	if levels != nil {
		if _, ok := levels[string(e.Level)]; !ok {
			return false
		}
	}
	return true
}

func jsonErr(msg string) error {
	return &sseError{msg}
}

type sseError struct{ msg string }

func (e *sseError) Error() string { return e.msg }
