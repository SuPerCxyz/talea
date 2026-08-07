//go:build windows

package adapters

import (
	"os/exec"
	"strings"
)

// detectProcessRunningOS 使用 tasklist 检测进程是否运行（Windows）。
// 退化方案：若 tasklist 不可用则返回 false，回退到 mtime 检测。
func detectProcessRunningOS(execName string) bool {
	if execName == "" {
		return false
	}
	// 去掉可能的 .exe 后缀统一比较
	name := strings.TrimSuffix(strings.ToLower(execName), ".exe")
	out, err := exec.Command("tasklist", "/FO", "CSV", "/NH").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// CSV 格式: "name","pid","session","sessionnum","mem"
		fields := strings.Split(line, ",")
		if len(fields) == 0 {
			continue
		}
		procName := strings.Trim(strings.Trim(fields[0], "\""), " ")
		procName = strings.TrimSuffix(strings.ToLower(procName), ".exe")
		if procName == name {
			return true
		}
	}
	return false
}
