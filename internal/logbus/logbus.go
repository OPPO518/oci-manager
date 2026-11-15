// Package logbus 提供一个进程内的环形日志总线，用于把管理面板的运行事件
// （启动、调度、抢购、审计、错误）汇聚起来，并通过 SSE 实时推送给前端
// 「日志查看」页面。
//
// 设计要点：
//   - 内存环形缓冲，容量固定（默认 1000 条），不落盘、不占数据库；
//   - 订阅者通过 chan 接收增量，断线后自动清理；
//   - Default 是全局实例，供各模块直接 Publish。
package logbus

import (
	"sync"
	"time"
)

// Level 表示日志级别。
type Level string

const (
	LevelInfo    Level = "info"
	LevelWarn    Level = "warn"
	LevelError   Level = "error"
	LevelSuccess Level = "success"
)

// Source 表示日志来源模块。
type Source string

const (
	SourceStartup   Source = "startup"
	SourceScheduler Source = "scheduler"
	SourceGrabber   Source = "grabber"
	SourceAudit     Source = "audit"
	SourceSystem    Source = "system"
)

// Entry 是一条日志事件。
type Entry struct {
	ID        int64  `json:"id"`
	Timestamp string `json:"timestamp"` // RFC3339
	Level     Level  `json:"level"`
	Source    Source `json:"source"`
	Account   string `json:"account"`
	Message   string `json:"message"`
}

// Bus 是环形日志缓冲 + 订阅器。
type Bus struct {
	mu   sync.RWMutex
	seq  int64
	cap  int
	buf  []Entry
	subs map[chan Entry]struct{}
}

// Default 是全局日志总线实例。
var Default *Bus

func init() {
	Default = New(1000)
}

// New 创建一个容量为 cap 的日志总线。
func New(capacity int) *Bus {
	if capacity <= 0 {
		capacity = 500
	}
	return &Bus{
		cap:  capacity,
		buf:  make([]Entry, 0, capacity),
		subs: make(map[chan Entry]struct{}),
	}
}

// Publish 发布一条日志事件。
func (b *Bus) Publish(level Level, source Source, account, message string) {
	b.mu.Lock()
	b.seq++
	e := Entry{
		ID:        b.seq,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     level,
		Source:    source,
		Account:   account,
		Message:   message,
	}
	b.buf = append(b.buf, e)
	if len(b.buf) > b.cap {
		b.buf = b.buf[len(b.buf)-b.cap:]
	}
	// 拷贝订阅者切片，避免在持有锁时向 chan 写入阻塞其他发布者。
	subs := make([]chan Entry, 0, len(b.subs))
	for ch := range b.subs {
		subs = append(subs, ch)
	}
	b.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- e:
		default:
			// 订阅者消费过慢，丢弃增量避免阻塞总线。
		}
	}
}

// Recent 返回最近 n 条日志（按时间升序）。
func (b *Bus) Recent(n int) []Entry {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if n <= 0 || n >= len(b.buf) {
		out := make([]Entry, len(b.buf))
		copy(out, b.buf)
		return out
	}
	out := make([]Entry, n)
	copy(out, b.buf[len(b.buf)-n:])
	return out
}

// All 返回全部缓冲日志（按时间升序）。
func (b *Bus) All() []Entry {
	return b.Recent(0)
}

// Subscribe 注册一个订阅通道；返回的取消函数用于退订。
func (b *Bus) Subscribe(ch chan Entry) func() {
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return func() { b.Unsubscribe(ch) }
}

// Unsubscribe 退订并关闭通道。
func (b *Bus) Unsubscribe(ch chan Entry) {
	b.mu.Lock()
	if _, ok := b.subs[ch]; ok {
		delete(b.subs, ch)
		close(ch)
	}
	b.mu.Unlock()
}
