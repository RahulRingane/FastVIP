package server

import (
	"os"
	"runtime"
	"strings"

	"go.uber.org/zap"
)

type kernelParamCheck struct {
	expecteds map[string]struct{}
	name      string
	expected  string
}

var (
	kernelParamChecks = []kernelParamCheck{
		{name: "net.ipv4.ip_forward", expected: "1"},
		{name: "net.ipv4.vs.conntrack", expected: "1"},
		{name: "net.ipv4.conf.all.rp_filter", expecteds: map[string]struct{}{"0": {}, "2": {}}},
		{name: "net.ipv4.conf.default.rp_filter", expecteds: map[string]struct{}{"0": {}, "2": {}}},
	}
	kernelParamCheckEnabled = runtime.GOOS == "linux"
	readKernelParamFile     = os.ReadFile
)

func (s *Server) logKernelParamPreflight() {
	if !kernelParamCheckEnabled || s.logger == nil {
		return
	}

	allMatched := true
	for _, check := range kernelParamChecks {
		raw, err := readKernelParamFile(kernelParamPath(check.name))
		if err != nil {
			allMatched = false
			s.logger.Error("failed to read kernel parameter",
				zap.String("name", check.name),
				zap.Error(err),
			)
			continue
		}

		actual := strings.TrimSpace(string(raw))
		if check.isValid(actual) {
			continue
		}

		allMatched = false
		s.logger.Error("kernel parameter mismatch",
			zap.String("name", check.name),
			zap.String("expected", check.expectedString()),
			zap.String("actual", actual),
		)
	}

	if allMatched {
		s.logger.Info("kernel parameter preflight passed")
	}
}

// isValid 检查实际值是否符合期望值
func (c kernelParamCheck) isValid(actual string) bool {
	if c.expecteds != nil {
		_, ok := c.expecteds[actual]
		return ok
	}
	return actual == c.expected
}

// expectedString 返回期望值的字符串表示，用于日志
func (c kernelParamCheck) expectedString() string {
	if c.expecteds != nil {
		var values []string
		for v := range c.expecteds {
			values = append(values, v)
		}
		return strings.Join(values, " or ")
	}
	return c.expected
}

func kernelParamPath(name string) string {
	return "/proc/sys/" + strings.ReplaceAll(name, ".", "/")
}
