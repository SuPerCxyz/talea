//go:build windows

package resume

import "os"

// Windows 无 SIGCHLD，信号转发中无需跳过任何信号。
// signal.Notify 在 Windows 上仅能收到 os.Interrupt (Ctrl+C)。
var ignoreSignal = os.Signal(nil)
