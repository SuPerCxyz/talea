//go:build linux

package adapters

import (
	"os"
	"strings"
)

// detectProcessRunningOS 读取 /proc 目录检测进程是否运行（Linux/macOS）。
// 进程名同时匹配 name 与 name.exe：npm 安装的 CLI（如 opencode.exe、claude.exe）
// 二进制常带 .exe 后缀，仅匹配 basename 会漏检导致活动状态误判为闲置。
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
		pid := e.Name()
		if pid[0] < '0' || pid[0] > '9' {
			continue
		}
		exe, err := os.Readlink("/proc/" + pid + "/exe")
		if err != nil {
			continue
		}
		base := exe
		if idx := lastSlash(exe); idx >= 0 {
			base = exe[idx+1:]
		}
		if processNameMatches(base, execName) {
			return true
		}
	}
	return false
}

// processNameMatches 比较进程可执行文件 basename 与期望名，忽略大小写与 .exe 后缀。
func processNameMatches(basename, execName string) bool {
	return strings.EqualFold(strings.TrimSuffix(basename, ".exe"), strings.TrimSuffix(execName, ".exe"))
}

func lastSlash(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' {
			return i
		}
	}
	return -1
}
