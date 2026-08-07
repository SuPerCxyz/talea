//go:build !windows

package adapters

import "os"

// detectProcessRunningOS 读取 /proc 目录检测进程是否运行（Linux/macOS）。
func detectProcessRunningOS(execName string) bool {
	if execName == "" {
		return false
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if name[0] < '0' || name[0] > '9' {
			continue
		}
		exe, err := os.Readlink("/proc/" + name + "/exe")
		if err != nil {
			continue
		}
		base := exe
		if idx := lastSlash(exe); idx >= 0 {
			base = exe[idx+1:]
		}
		if base == execName {
			return true
		}
	}
	return false
}

func lastSlash(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' {
			return i
		}
	}
	return -1
}
