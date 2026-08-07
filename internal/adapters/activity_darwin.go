//go:build darwin

package adapters

// macOS 无 /proc，进程检测退化为 mtime 检测（detectByMtime）。
// detectProcessRunningOS 始终返回 false，由调用方回退到文件 mtime 判定。
func detectProcessRunningOS(execName string) bool {
	return false
}
