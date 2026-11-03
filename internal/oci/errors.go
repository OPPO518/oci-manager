package oci

import (
	"errors"
	"fmt"
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
