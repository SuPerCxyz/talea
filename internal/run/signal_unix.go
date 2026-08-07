//go:build !windows

package run

import (
	"os"
	"os/signal"
	"syscall"
)

// isChildExitSignal 判断信号是否为子进程退出通知（Unix: SIGCHLD）。
func isChildExitSignal(sig os.Signal) bool {
	return sig == syscall.SIGCHLD
}

// notifyRunSignals 仅注册需要转发的信号，避免 SIGWINCH 等无关信号干扰子进程。
func notifyRunSignals(ch chan<- os.Signal) {
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
}
