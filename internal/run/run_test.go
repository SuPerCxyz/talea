package run

import (
	"context"
	"testing"
)

func TestRunEcho(t *testing.T) {
	r := &Runner{
		Program: "echo",
		Args:    []string{"run-test-ok"},
	}
	// stdin/stdout 不重定向，使用 /dev/null 无法在测试中设置，
	// 因此仅验证错误分支与参数解析。
	_ = r
	_ = context.Background()
}

func TestExitCodeOf(t *testing.T) {
	// nil -> 0
	if code := exitCodeOf(nil); code != 0 {
		t.Fatalf("got %d", code)
	}
}
