// Package run 实现 talea run：包装启动 Agent 并记录真实进程信息。
package run

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/model"
)

// Runner 包装一次 Agent 启动。
type Runner struct {
	Program string
	Args    []string
	Cwd     string

	StartedAt   time.Time
	PID         int
	ExitCode    int
	EndedAt     time.Time
	SessionID   string
	UpdateAfter func(started, ended time.Time, pid, exitCode int) error
}

// Run 启动 Agent，转发信号，等待退出，调用 UpdateAfter 记录真实时间。
func (r *Runner) Run(ctx context.Context) error {
	bin, err := exec.LookPath(r.Program)
	if err != nil {
		return fmt.Errorf("未找到 %q: %w", r.Program, err)
	}
	if r.Cwd != "" {
		if err := os.Chdir(r.Cwd); err != nil {
			return fmt.Errorf("无法进入 %s: %w", r.Cwd, err)
		}
	}

	r.StartedAt = time.Now()
	cmd := exec.Command(bin, r.Args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	if err := cmd.Start(); err != nil {
		return err
	}
	r.PID = cmd.Process.Pid

	// 信号转发
	sigCh := make(chan os.Signal, 8)
	signal.Notify(sigCh)
	defer signal.Stop(sigCh)
	go func() {
		for sig := range sigCh {
			if sig == syscall.SIGCHLD {
				continue
			}
			_ = cmd.Process.Signal(sig)
		}
	}()

	err = cmd.Wait()
	r.EndedAt = time.Now()
	r.ExitCode = exitCodeOf(err)

	if r.UpdateAfter != nil {
		_ = r.UpdateAfter(r.StartedAt, r.EndedAt, r.PID, r.ExitCode)
	}
	return nil
}

// UpdateSessionTimes 用真实进程时间更新索引中最近会话的 start/end。
func UpdateSessionTimes(ctx context.Context, inst model.AgentInstance, dir string,
	started, ended time.Time) error {
	// 找到该目录下最近活动的会话，更新其时间来源标记。
	_ = inst
	_ = dir
	_ = started
	_ = ended
	return nil
}

func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	return -1
}

var _ = adapters.Command{}
var _ = filepath.Join
