package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/cli/output"
	"github.com/talea/talea/internal/i18n"
	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
	"github.com/talea/talea/internal/resume"
	"github.com/talea/talea/internal/search"
	"github.com/talea/talea/internal/syncer"
)

// resumeSession 执行会话恢复：构造命令、处理目录缺失、执行或打印。
// dryRun 为 true 时仅打印命令不执行；返回错误以退出码区分。
func resumeSession(ctx context.Context, a *app.App, sess *model.Session, targetDir string, dryRun bool) error {
	plan, err := resume.Build(*sess, targetDir, a.Config.PathMapping)
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
			return exitError{code: ExitNoWorkdir, msg: i18n.Tr("resume cancelled", "已取消恢复")}
		case "mapped":
			if newTarget != "" {
				plan.TargetDir = newTarget
			}
		}
		plan.DirExists = dirExists(plan.TargetDir)
	}

	ad, ok := a.Registry.Get(sess.AgentID)
	if !ok {
		return exitError{code: ExitFormatUnsup, msg: i18n.Tr("session format not supported", "会话格式不支持")}
	}
	resumer, ok := adapters.As[adapters.Resumer](ad)
	if !ok {
		return exitError{code: ExitCapMissing, msg: i18n.Tr("agent does not support resume", "Agent 不支持恢复能力")}
	}
	cmd2, err := resumer.BuildResumeCommand(*sess, plan.TargetDir)
	if err != nil {
		return err
	}
	plan.Command = cmd2

	if dryRun {
		fmt.Printf("%s %s\n", i18n.Tr("Agent:", "Agent："), displayNameOf(ad))
		fmt.Printf("%s %s\n", i18n.Tr("Directory:", "目录："), plan.TargetDir)
		fmt.Printf("%s %s\n", i18n.Tr("Program:", "程序："), plan.Command.Program)
		fmt.Printf("%s %s\n", i18n.Tr("Arguments:", "参数："), strings.Join(plan.Command.Args, " "))
		return nil
	}

	if _, err := resume.ResolveProgram(plan.Command.Program); err != nil {
		return exitError{code: ExitAgentMissing, msg: err.Error()}
	}
	return resume.Exec(plan)
}

// handleMissingDir 原目录不存在时提示指定新目录，回车默认 /tmp。
// 非 TTY 时直接使用默认 /tmp。
func handleMissingDir(sess *model.Session, missingDir string, mappings map[string]string) (string, string, error) {
	fmt.Fprintf(os.Stderr, "\n%s\n\n%s\n\n", i18n.Tr("Original working directory missing:", "原工作目录不存在："), missingDir)
	if !isTTY(os.Stdin) {
		fmt.Fprintln(os.Stderr, i18n.Tr("will resume with default directory /tmp.", "将使用默认目录 /tmp 恢复。"))
		return "/tmp", "mapped", nil
	}
	reader := bufio.NewReader(os.Stdin)
	fmt.Fprint(os.Stderr, i18n.Tr("Enter a new directory (Enter defaults to /tmp): ", "请输入新目录（回车默认 /tmp）："))
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", "cancel", err
	}
	dir := strings.TrimSpace(line)
	if dir == "" {
		return "/tmp", "mapped", nil
	}
	if dirExists(dir) {
		return dir, "mapped", nil
	}
	fmt.Fprintf(os.Stderr, i18n.Tr("directory %q does not exist, using default /tmp.\n", "目录 %q 不存在，将使用默认 /tmp 恢复。\n"), dir)
	return "/tmp", "mapped", nil
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
	return fmt.Sprintf(i18n.Tr("exit code %d", "退出码 %d"), e.code)
}

// findSession 从索引定位会话。id 可为完整或前缀 session id；
// agent 非空时作为过滤条件。匹配策略：精确 → 唯一前缀；多前缀候选报歧义。
func findSession(ctx context.Context, a *app.App, id, agent string) (*model.Session, error) {
	db, err := index.Open(a.Paths.DBPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		return nil, err
	}
	if err := syncer.Sync(ctx, a, db); err != nil {
		return nil, err
	}
	results, err := search.ByIDPrefix(ctx, db, id, agent, 100)
	if err != nil {
		return nil, err
	}
	// 精确匹配
	for i := range results {
		s := results[i].Session
		if s.SessionID == id {
			s.WorkingDirExists = dirExists(s.WorkingDirectory)
			return &s, nil
		}
	}
	// 前缀匹配：收集全部候选
	var matches []*model.Session
	for i := range results {
		s := results[i].Session
		if strings.HasPrefix(s.SessionID, id) {
			ss := s
			ss.WorkingDirExists = dirExists(s.WorkingDirectory)
			matches = append(matches, &ss)
		}
	}
	switch len(matches) {
	case 0:
		return nil, exitError{code: ExitNotFound, msg: i18n.Trf("session %q not found", "未找到会话 %q", id)}
	case 1:
		return matches[0], nil
	default:
		var b strings.Builder
		b.WriteString(i18n.Trf("session id %q is ambiguous, matching:", "会话 ID %q 有多个匹配：", id))
		for _, m := range matches {
			fmt.Fprintf(&b, "\n  %s  %s", output.DisplayAgent(m.AgentID), m.SessionID)
		}
		b.WriteString(i18n.Tr("\ntry a longer prefix", "\n请使用更完整的前缀"))
		return nil, exitError{code: ExitNotFound, msg: b.String()}
	}
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
