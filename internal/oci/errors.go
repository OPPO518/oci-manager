package oci

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/oracle/oci-go-sdk/v65/common"
)

// IsCapacityError 判断错误是否表示"资源容量不足"。
//
// OCI 在可用域没有可用宿主机时会返回 HTTP 500，消息为 "Out of host capacity"。
// 这是 ARM 抢购场景下唯一需要继续重试的错误，其余错误应立即停止。
func IsCapacityError(err error) bool {
	if err == nil {
		return false
	}
	var svcErr common.ServiceError
	if !errors.As(err, &svcErr) {
		return false
	}
	msg := strings.ToLower(svcErr.GetMessage())
	code := strings.ToLower(svcErr.GetCode())
	return strings.Contains(msg, "out of host capacity") ||
		strings.Contains(msg, "out of capacity") ||
		code == "outofhostcapacity"
}

// IsRateLimited 判断 OCI 是否正在限流（HTTP 429 / TooManyRequests）。
func IsRateLimited(err error) bool {
	if err == nil {
		return false
	}
	var svcErr common.ServiceError
	if errors.As(err, &svcErr) && svcErr.GetHTTPStatusCode() == 429 {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "too many request") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "throttl") ||
		strings.Contains(msg, "429")
}

// IsTransientError 判断是否为暂时性故障（限流、5xx、超时、连接重置）。
// 这类错误必须退避暂停：既不能当成「没货」继续高频重试，也不能触发
// 「关闭预检、每轮直创」的降级，否则 OCI 一开始限流就会把请求量放大。
func IsTransientError(err error) bool {
	if err == nil {
		return false
	}
	if IsRateLimited(err) {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	var svcErr common.ServiceError
	if errors.As(err, &svcErr) {
		code := svcErr.GetHTTPStatusCode()
		if code == 408 || code == 425 || code >= 500 {
			return true
		}
	}
	msg := strings.ToLower(err.Error())
	// 很多包装错误已经丢失了 ServiceError 类型，只能从文本里识别 5xx。
	for _, code := range []string{
		"http 500", "http 502", "http 503", "http 504",
		"status 500", "status 502", "status 503", "status 504",
	} {
		if strings.Contains(msg, code) {
			return true
		}
	}
	return strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "timed out") ||
		strings.Contains(msg, "deadline exceeded") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "temporary failure") ||
		strings.Contains(msg, "service unavailable")
}

// APIError 包装 OCI SDK 返回的错误。
// Error() 给出面向用户的可读文本，Unwrap() 保留原始错误，
// 使得 IsCapacityError 这类判断能够穿透包装层。
type APIError struct {
	Op      string
	Message string
	Err     error
}

func (e *APIError) Error() string { return e.Op + ": " + e.Message }

// Unwrap 暴露原始错误，供 errors.As/errors.Is 使用。
func (e *APIError) Unwrap() error { return e.Err }

// wrap 把 SDK 错误包装为 APIError。
func wrap(op string, err error) error {
	return &APIError{Op: op, Message: DescribeError(err), Err: err}
}

// DescribeError 把 SDK 错误转换成可直接展示给用户的文本。
func DescribeError(err error) string {
	if err == nil {
		return ""
	}
	var svcErr common.ServiceError
	if errors.As(err, &svcErr) {
		msg := svcErr.GetMessage()
		if code := svcErr.GetCode(); code != "" {
			return fmt.Sprintf("%s（%s，HTTP %d）", msg, code, svcErr.GetHTTPStatusCode())
		}
		return fmt.Sprintf("%s（HTTP %d）", msg, svcErr.GetHTTPStatusCode())
	}
	return err.Error()
}
