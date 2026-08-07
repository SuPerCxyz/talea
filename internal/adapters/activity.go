package adapters

import (
	"context"
	"os"
	"time"

	"github.com/talea/talea/internal/model"
)

// detectProcessRunning 检查进程表中是否存在指定可执行文件名的进程。
// 由平台相关文件实现：activity_unix.go (读取 /proc)、activity_windows.go (tasklist)。
func detectProcessRunning(execName string) bool {
	return detectProcessRunningOS(execName)
}

// ProcessActivityDetector 是通用的进程活动检测器。
// 判断依据：
//  1. Agent 可执行文件是否有运行中的进程（active）。
//  2. 源文件在最近 30 秒内更新（possibly_active）。
//  3. 其余 inactive。
type ProcessActivityDetector struct {
	Executable string // 可执行文件名（不含路径）
}

// AgentProcessRunning 判断 Agent 是否有任何进程在运行（全会话共享一次）。
func AgentProcessRunning(execName string) bool {
	return detectProcessRunning(execName)
}

// DetectActivity 检测会话活动状态。
func (d ProcessActivityDetector) DetectActivity(
	ctx context.Context,
	session model.Session,
) (model.ActivityState, error) {
	if detectProcessRunning(d.Executable) {
		return model.ActivityActive, nil
	}
	return detectByMtime(session), nil
}

// DetectByMtime 仅按文件更新时间检测（进程已确认无运行）。
func DetectByMtime(session model.Session) model.ActivityState {
	return detectByMtime(session)
}

func detectByMtime(session model.Session) model.ActivityState {
	if session.SourcePath != "" && session.SourceMtime > 0 {
		fi, err := os.Stat(session.SourcePath)
		if err == nil {
			if time.Since(fi.ModTime()) < 30*time.Second {
				return model.ActivityPossiblyActive
			}
		}
	}
	return model.ActivityInactive
}

var _ ActivityDetector = ProcessActivityDetector{}
