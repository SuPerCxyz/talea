//go:build windows

package run

import (
	"os"
	"os/signal"
)

// Windows 无子进程退出信号，始终返回 false。
func isChildExitSignal(sig os.Signal) bool {
	return false
}

// notifyRunSignals 在 Windows 上仅注册 os.Interrupt。
func notifyRunSignals(ch chan<- os.Signal) {
	signal.Notify(ch, os.Interrupt)
}
