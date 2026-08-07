//go:build !windows

package resume

import (
	"os"
	"os/signal"
	"syscall"
)

// ignoreSignal 是子进程退出时不需要转发的信号。
// Unix 上 SIGCHLD 是子进程退出通知，转发无意义且会干扰。
var ignoreSignal os.Signal = syscall.SIGCHLD

// notifyResumeSignals 仅注册需要转发的信号，避免 SIGWINCH 等无关信号干扰子进程。
func notifyResumeSignals(ch chan<- os.Signal) {
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
}
