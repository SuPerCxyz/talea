//go:build windows

package resume

import (
	"os"
	"os/signal"
)

// Windows 无 SIGCHLD，信号转发中无需跳过任何信号。
// signal.Notify 在 Windows 上仅能收到 os.Interrupt (Ctrl+C)。
var ignoreSignal os.Signal = nil

// notifyResumeSignals 在 Windows 上仅注册 os.Interrupt。
func notifyResumeSignals(ch chan<- os.Signal) {
	signal.Notify(ch, os.Interrupt)
}
