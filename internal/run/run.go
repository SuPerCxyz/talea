// Package run 实现 talea run：包装启动 Agent 并记录真实进程信息。
package run

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"time"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/config"
	"github.com/talea/talea/internal/index"
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
// 返回子进程的退出错误（如有），调用方可通过 r.ExitCode 获取退出码。
func (r *Runner) Run(ctx context.Context) error {
	bin, err := exec.LookPath(r.Program)
	if err != nil {
		return fmt.Errorf("未找到 %q: %w", r.Program, err)
	}

	r.StartedAt = time.Now()
	cmd := exec.Command(bin, r.Args...)
	if r.Cwd != "" {
		cmd.Dir = r.Cwd
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	if err := cmd.Start(); err != nil {
		return err
	}
	r.PID = cmd.Process.Pid

	// 信号转发：仅转发中断/终止信号，避免 SIGWINCH 等无关信号干扰子进程
	sigCh := make(chan os.Signal, 8)
	notifyRunSignals(sigCh)
	defer signal.Stop(sigCh)
	go func() {
		for sig := range sigCh {
			if isChildExitSignal(sig) {
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
	return err
}

// UpdateSessionTimes 用真实进程时间更新索引中该目录下时间窗口内的会话。
// 优先匹配 [started, ended] 窗口内的会话；若该 Agent 会话在启动前已存在于
// 索引中（Agent 启动即创建会话），则更新其 started/ended 时间来源。
func UpdateSessionTimes(ctx context.Context, inst model.AgentInstance, dir string,
	started, ended time.Time) error {
	db, err := index.Open(indexPaths().DBPath)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		return err
	}

	// 1) 优先：查找 last_activity_at 在 [started-5s, ended+5s] 窗口内、
	//    且工作目录匹配的会话（进程期间产生的真实会话）。
	rows, err := db.SQL().QueryContext(ctx, `
		SELECT session_id FROM sessions
		WHERE agent_instance_id = ? AND working_directory = ?
		  AND last_activity_at >= ? AND last_activity_at <= ?
		ORDER BY last_activity_at DESC LIMIT 1`,
		inst.InstanceID, dir, started.Add(-5*time.Second).Unix(), ended.Add(5*time.Second).Unix())
	if err != nil {
		return err
	}
	var sessionID string
	found := rows.Next()
	if found {
		if err := rows.Scan(&sessionID); err != nil {
			rows.Close()
			return err
		}
	}
	rows.Close()

	// 2) 回退：窗口内无匹配，取该目录最近会话。
	if !found {
		rows, err = db.SQL().QueryContext(ctx, `
			SELECT session_id FROM sessions
			WHERE agent_instance_id = ? AND working_directory = ?
			ORDER BY last_activity_at DESC LIMIT 1`, inst.InstanceID, dir)
		if err != nil {
			return err
		}
		found = rows.Next()
		if found {
			if err := rows.Scan(&sessionID); err != nil {
				rows.Close()
				return err
			}
		}
		rows.Close()
	}
	if !found {
		return fmt.Errorf("未找到 %s 目录下的会话，请先执行 talea index", dir)
	}

	_, err = db.SQL().ExecContext(ctx, `
		UPDATE sessions SET
			started_at = ?, start_time_source = 'process_start',
			ended_at = ?, end_time_source = 'process_exit',
			duration_seconds = ?,
			updated_at = ?
		WHERE agent_instance_id = ? AND session_id = ?`,
		started.Unix(), ended.Unix(),
		int64(ended.Sub(started).Seconds()),
		time.Now().Unix(), inst.InstanceID, sessionID)
	return err
}

func indexPaths() config.Paths {
	return config.ResolvePaths()
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
