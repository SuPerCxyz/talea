//go:build !windows

package run

import (
	"os"
	"syscall"
)

// isChildExitSignal 判断信号是否为子进程退出通知（Unix: SIGCHLD）。
func isChildExitSignal(sig os.Signal) bool {
	return sig == syscall.SIGCHLD
}
