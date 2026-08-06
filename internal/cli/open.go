package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
	"github.com/talea/talea/internal/resume"
	"github.com/talea/talea/internal/search"
)

func newOpenCmd() *cobra.Command {
	var (
		agentFlag  string
		cwdFlag    string
		dryRunFlag bool
	)
	cmd := &cobra.Command{
		Use:   "open <session-id>",
		Short: "恢复会话",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			a, err := app.New(ctx)
			if err != nil {
				return err
			}
			sess, err := findSession(ctx, a, args[0], agentFlag)
			if err != nil {
				return err
			}
			return resumeSession(ctx, a, sess, cwdFlag, dryRunFlag)
		},
	}
	cmd.Flags().StringVar(&agentFlag, "agent", "", "Agent 标识")
	cmd.Flags().StringVar(&cwdFlag, "cwd", "", "目标目录（覆盖原目录）")
	cmd.Flags().BoolVar(&dryRunFlag, "dry-run", false, "仅打印恢复命令")
	return cmd
}

func newLastCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "last",
		Short: "当前目录最近会话",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			a, err := app.New(ctx)
			if err != nil {
				return err
			}
			db, err := index.Open(a.Paths.DBPath)
			if err != nil {
				return err
			}
			defer db.Close()
			if err := db.Migrate(ctx); err != nil {
				return err
			}
			if err := search.Ensure(ctx, db); err != nil {
				return err
			}
			results, err := search.Search(ctx, db, search.Query{Cwd: cwd, Limit: 1})
			if err != nil {
				return err
			}
			if len(results) == 0 {
				fmt.Println("当前目录没有索引的会话")
				return nil
			}
			sess := &results[0].Session
			sess.WorkingDirExists = dirExists(sess.WorkingDirectory)
			sess.Activity = model.ActivityInactive
			fmt.Printf("Agent：%s\n", sess.AgentID)
			fmt.Printf("会话 ID：%s\n", sess.SessionID)
			fmt.Printf("首次提问：%s\n", firstLine(sess.FirstQuestion))
			if sess.StartedAt != nil {
				fmt.Printf("开始：%s\n", sess.StartedAt.Format("2006-01-02 15:04"))
			}
			if sess.WorkingDirectory != "" {
				fmt.Printf("目录：%s\n", sess.WorkingDirectory)
			}
			return nil
		},
	}
	return cmd
}

// mapChoice 将用户输入映射为动作。
func mapChoice(line string) string {
	switch strings.TrimSpace(line) {
	case "1":
		return "mapped"
	case "2":
		return "current"
	case "3":
		return "view"
	case "4":
		return "copy"
	default:
		return "cancel"
	}
}

// resumeSession 执行会话恢复：构造命令、处理目录缺失、执行或打印。
// dryRun 为 true 时仅打印命令不执行；返回错误以退出码区分。
func resumeSession(ctx context.Context, a *app.App, sess *model.Session, cwdFlag string, dryRun bool) error {
	plan, err := resume.Build(*sess, cwdFlag, a.Config.PathMapping)
	if err != nil {
		return err
	}
	if !plan.DirExists {
		newTarget, action, err := handleMissingDir(sess, plan.TargetDir, a.Config.PathMapping)
		if err != nil {
			return err
		}
		switch action {
		case "cancel":
			return exitError{code: ExitNoWorkdir, msg: "已取消恢复"}
		case "view":
			fmt.Printf("会话 ID：%s\n", sess.SessionID)
			fmt.Printf("首次提问：%s\n", firstLine(sess.FirstQuestion))
			return nil
		case "copy":
			ad2, _ := a.Registry.Get(sess.AgentID)
			if resumer, ok := adapters.As[adapters.Resumer](ad2); ok {
				cmd3, err := resumer.BuildResumeCommand(*sess, plan.TargetDir)
				if err == nil {
					fmt.Printf("%s %s\n", cmd3.Program, strings.Join(cmd3.Args, " "))
				}
			}
			return nil
		case "current":
			plan.TargetDir, err = os.Getwd()
			if err != nil {
				return err
			}
		case "mapped":
			if newTarget != "" {
				plan.TargetDir = newTarget
			}
		}
		plan.DirExists = dirExists(plan.TargetDir)
	}

	ad, ok := a.Registry.Get(sess.AgentID)
	if !ok {
		return exitError{code: ExitFormatUnsup, msg: "会话格式不支持"}
	}
	resumer, ok := adapters.As[adapters.Resumer](ad)
	if !ok {
		return exitError{code: ExitCapMissing, msg: "Agent 不支持恢复能力"}
	}
	cmd2, err := resumer.BuildResumeCommand(*sess, plan.TargetDir)
	if err != nil {
		return err
	}
	plan.Command = cmd2

	if dryRun {
		fmt.Printf("Agent：%s\n", displayNameOf(ad))
		fmt.Printf("目录：%s\n", plan.TargetDir)
		fmt.Printf("程序：%s\n", plan.Command.Program)
		fmt.Printf("参数：%s\n", strings.Join(plan.Command.Args, " "))
		return nil
	}

	if _, err := resume.ResolveProgram(plan.Command.Program); err != nil {
		return exitError{code: ExitAgentMissing, msg: err.Error()}
	}
	return resume.Exec(plan)
}

// handleMissingDir 交互处理原工作目录不存在的情况。
// 返回 (映射后的新目录, 选择动作, 错误)。非 TTY 时自动取消。
func handleMissingDir(sess *model.Session, missingDir string, mappings map[string]string) (string, string, error) {
	fmt.Fprintf(os.Stderr, "\n原工作目录不存在：\n\n%s\n\n请选择：\n", missingDir)
	fmt.Fprintln(os.Stderr, "1. 映射到新目录")
	fmt.Fprintln(os.Stderr, "2. 在当前目录恢复")
	fmt.Fprintln(os.Stderr, "3. 仅查看会话")
	fmt.Fprintln(os.Stderr, "4. 复制恢复命令")
	fmt.Fprintln(os.Stderr, "5. 取消")

	if !isTTY(os.Stdin) {
		return "", "cancel", nil
	}
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", "cancel", err
	}
	action := mapChoice(line)
	switch action {
	case "mapped":
		fmt.Fprint(os.Stderr, "输入新目录路径：")
		dirLine, err := reader.ReadString('\n')
		if err != nil {
			return "", "cancel", err
		}
		dir := strings.TrimSpace(dirLine)
		if dir == "" {
			return "", "cancel", nil
		}
		return dir, "mapped", nil
	default:
		return "", action, nil
	}
}

func isTTY(f *os.File) bool {
	if f == nil {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// exitError 携带退出码的错误。
type exitError struct {
	code int
	msg  string
}

func (e exitError) Error() string {
	if e.msg != "" {
		return e.msg
	}
	return fmt.Sprintf("退出码 %d", e.code)
}

// findSession 从索引定位会话（支持前缀匹配）。
func findSession(ctx context.Context, a *app.App, id, agent string) (*model.Session, error) {
	db, err := index.Open(a.Paths.DBPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		return nil, err
	}
	if err := search.Ensure(ctx, db); err != nil {
		return nil, err
	}
	results, err := search.Search(ctx, db, search.Query{Term: id, Agent: agent, Limit: 50})
	if err != nil {
		return nil, err
	}
	for i := range results {
		s := results[i].Session
		if s.SessionID == id {
			s.WorkingDirExists = dirExists(s.WorkingDirectory)
			return &s, nil
		}
	}
	// 前缀匹配
	for i := range results {
		s := results[i].Session
		if strings.HasPrefix(s.SessionID, id) {
			s.WorkingDirExists = dirExists(s.WorkingDirectory)
			return &s, nil
		}
	}
	if len(results) > 0 {
		return nil, exitError{code: ExitNotFound, msg: fmt.Sprintf("未找到会话 %q", id)}
	}
	return nil, exitError{code: ExitNotFound, msg: fmt.Sprintf("未找到会话 %q", id)}
}

func displayNameOf(ad adapters.Adapter) string {
	return ad.Info().DisplayName
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

var _ = errors.Is
