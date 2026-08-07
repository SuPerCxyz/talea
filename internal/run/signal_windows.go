//go:build windows

package run

import "os"

// Windows 无子进程退出信号，始终返回 false。
func isChildExitSignal(sig os.Signal) bool {
	return false
}
