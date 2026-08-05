// Package version 保存版本信息。
package version

import (
	"fmt"
	"runtime"
)

// 通过 ldflags 注入。
var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

// String 返回完整版本字符串。
func String() string {
	return fmt.Sprintf("talea %s (%s) %s/%s commit %s built %s",
		Version, runtime.Version(), runtime.GOOS, runtime.GOARCH, Commit, BuildDate)
}

// Short 返回简短版本号。
func Short() string { return Version }
