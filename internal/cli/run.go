package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/i18n"
	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
	"github.com/talea/talea/internal/run"
)

func newRunCmd() *cobra.Command {
	var cwdFlag string
	cmd := &cobra.Command{
		Use:   "run <agent>",
		Short: i18n.Tr("wrap-launch an agent (record real process times)", "包装启动 Agent（记录真实进程时间）"),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			agentID := model.AgentID(args[0])
			a, err := app.New(ctx)
			if err != nil {
				return err
			}
			ad, ok := a.Registry.Get(agentID)
			if !ok {
				return exitError{code: ExitNotFound, msg: i18n.Trf("unknown agent: %s", "未知 Agent：%s", agentID)}
			}
			insts, err := ad.Detect(ctx)
			if err != nil || len(insts) == 0 {
				return exitError{code: ExitAgentMissing, msg: i18n.Trf("agent %s not installed", "Agent %s 未安装", agentID)}
			}
			inst := insts[0]

			cwd := cwdFlag
			if cwd == "" {
				cwd, err = os.Getwd()
				if err != nil {
					return err
				}
			}
			// 用临时会话调用恢复命令构造器生成启动参数
			cmd2, err := buildStartCommand(ad, inst, cwd)
			if err != nil {
				return err
			}

			r := &run.Runner{
				Program: cmd2.Program,
				Args:    cmd2.Args,
				Cwd:     cwd,
				UpdateAfter: func(started, ended time.Time, pid, exitCode int) error {
					// 先重新索引，确保进程期间新建的会话被 Discover
					db, err := index.Open(a.Paths.DBPath)
					if err == nil {
						if err := db.Migrate(ctx); err == nil {
							ix := &index.Indexer{App: a, DB: db}
							if _, err := ix.Run(ctx); err != nil {
								fmt.Fprintf(os.Stderr, "talea run: %v\n", i18n.Trf("index update failed: %v", "索引更新失败: %v", err))
							}
						}
						db.Close()
					}
					// 更新索引中该目录时间窗口内会话的进程时间
					err = run.UpdateSessionTimes(ctx, inst, cwd, started, ended)
					if err != nil {
						return fmt.Errorf(i18n.Tr("update session times failed: %w", "更新会话时间失败: %w"), err)
					}
					fmt.Fprintf(os.Stderr, "talea run: %s\n", i18n.Trf("session updated (PID %d, exit code %d)", "会话已更新（PID %d，退出码 %d）", pid, exitCode))
					return nil
				},
			}
			fmt.Fprintf(os.Stderr, "talea run: %s\n", i18n.Trf("starting %s", "启动 %s", agentID))
			return r.Run(ctx)
		},
	}
	cmd.Flags().StringVar(&cwdFlag, "cwd", "", i18n.Tr("working directory (default current)", "工作目录（默认当前目录）"))
	return cmd
}

// buildStartCommand 构造 Agent 的启动命令。
// 复用 Resumer 能力：无会话时使用 Agent 默认启动参数。
func buildStartCommand(ad adapters.Adapter, inst model.AgentInstance, cwd string) (adapters.Command, error) {
	// 尝试从可执行文件构造基础启动
	exe := inst.ExecutablePath
	if exe == "" {
		exe = string(inst.AgentID)
	}
	return adapters.Command{Program: exe}, nil
}
