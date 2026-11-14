package oci

import (
	"errors"
	"testing"
)

// 回归：过去容量预检遇到任何错误都会「降级为直接创建」，
// 导致 OCI 一开始限流(429)系统反而按最短间隔狂发 LaunchInstance。
func TestTransientErrorsNeverDegradeToDirectLaunch(t *testing.T) {
	cases := []struct {
		msg       string
		rate      bool
		transient bool
	}{
		{"TooManyRequests: HTTP 429, rate limit exceeded", true, true},
		{"request throttled by service", true, true},
		{"InternalServerError (HTTP 500)", false, true},
		{"ServiceUnavailable (HTTP 503)", false, true},
		{"context deadline exceeded", false, true},
		{"dial tcp: i/o timeout", false, true},
		{"connection reset by peer", false, true},
		{"Out of host capacity", false, false},
	}
	for _, tc := range cases {
		err := errors.New(tc.msg)
		if got := IsRateLimited(err); got != tc.rate {
			t.Errorf("%q: IsRateLimited = %v, want %v", tc.msg, got, tc.rate)
		}
		if got := IsTransientError(err); got != tc.transient {
			t.Errorf("%q: IsTransientError = %v, want %v", tc.msg, got, tc.transient)
		}
		if tc.transient && IsCapacityReportUnsupported(err) {
			t.Errorf("%q: 暂时性错误被误判为「接口不支持」，会触发降级直创", tc.msg)
		}
	}
}

// 明确的参数错误也不能再被当成「接口不支持」。
func TestCapacityReportUnsupportedMarkersAreNarrow(t *testing.T) {
	unsupported := []string{
		"NotAuthorized: authorization failed",
		"UnknownOperation for CreateComputeCapacityReport",
		"NotFound (HTTP 404)",
	}
	for _, msg := range unsupported {
		if !IsCapacityReportUnsupported(errors.New(msg)) {
			t.Errorf("%q 应判定为接口不支持", msg)
		}
	}
	notUnsupported := []string{
		"InvalidParameter: compartment id is malformed",
		"invalid parameter for shape",
	}
	for _, msg := range notUnsupported {
		if IsCapacityReportUnsupported(errors.New(msg)) {
			t.Errorf("%q 不应判定为接口不支持", msg)
		}
	}
}
