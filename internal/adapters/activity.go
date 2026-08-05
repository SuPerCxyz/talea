package adapters

import (
	"context"
	"os"
	"time"

	"github.com/talea/talea/internal/model"
)

// detectProcessRunning 检查进程表中是否存在指定可执行文件名的进程。
// 读取 /proc 目录，避免依赖 ps 命令。
func detectProcessRunning(execName string) bool {
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

// ProcessActivityDetector 是通用的进程活动检测器。
// 判断依据：
//  1. Agent 可执行文件是否有运行中的进程（active）。
//  2. 源文件在最近 30 秒内更新（possibly_active）。
//  3. 其余 inactive。
type ProcessActivityDetector struct {
	Executable string // 可执行文件名（不含路径）
}

// DetectActivity 检测会话活动状态。
func (d ProcessActivityDetector) DetectActivity(
	ctx context.Context,
	session model.Session,
) (model.ActivityState, error) {
	if detectProcessRunning(d.Executable) {
		return model.ActivityActive, nil
	}
	// 文件近期更新 -> 可能进行中
	if session.SourcePath != "" && session.SourceMtime > 0 {
		fi, err := os.Stat(session.SourcePath)
		if err == nil {
			if time.Since(fi.ModTime()) < 30*time.Second {
				return model.ActivityPossiblyActive, nil
			}
		}
	}
	return model.ActivityInactive, nil
}

var _ ActivityDetector = ProcessActivityDetector{}
