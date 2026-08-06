// Package resume 负责恢复命令构造、路径映射与进程替换。
package resume

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/i18n"
	"github.com/talea/talea/internal/model"
)

// Plan 描述一次恢复的准备结果。
type Plan struct {
	Session   model.Session
	TargetDir string
	DirExists bool
	DirMapped bool
	Command   adapters.Command
	Binary    string
	Args      []string
}

// Resolver 将路径映射应用到目标目录（最长前缀匹配）。
func ApplyPathMapping(dir string, mappings map[string]string) (string, bool) {
	if dir == "" {
		return dir, false
	}
	// 按前缀长度降序，保证最长前缀优先
	keys := make([]string, 0, len(mappings))
	for k := range mappings {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return len(keys[i]) > len(keys[j])
	})
	for _, prefix := range keys {
		if dir == prefix {
			return mappings[prefix], true
		}
		if strings.HasPrefix(dir, prefix+string(filepath.Separator)) {
			rel := strings.TrimPrefix(dir, prefix)
			return filepath.Join(mappings[prefix], rel), true
		}
	}
	return dir, false
}

// Build 构造恢复计划。
func Build(s model.Session, overrideDir string, mappings map[string]string) (Plan, error) {
	target := s.WorkingDirectory
	mapped := false
	if overrideDir != "" {
		target = overrideDir
	} else if target != "" {
		var m string
		m, mapped = ApplyPathMapping(target, mappings)
		target = m
	}

	if target == "" {
		return Plan{}, errors.New(i18n.Tr("session has no resumable working directory", "会话没有可恢复的工作目录"))
	}

	return Plan{
		Session:   s,
		TargetDir: target,
		DirExists: dirExists(target),
		DirMapped: mapped,
	}, nil
}

// Exec 在目标目录用参数数组执行恢复命令并替换进程。
// binary 由调用方预先解析（LookPath），argv[0] 为二进制路径。
func Exec(plan Plan) error {
	if plan.Command.Program == "" {
		return errors.New(i18n.Tr("missing resume program", "缺少恢复程序"))
	}
	binary, err := exec.LookPath(plan.Command.Program)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.Trf("%q not found", "未找到 %q", plan.Command.Program), err)
	}
	if err := os.Chdir(plan.TargetDir); err != nil {
		return fmt.Errorf("%s: %w", i18n.Trf("cannot enter directory %s", "无法进入目录 %s", plan.TargetDir), err)
	}
	// 恢复终端：退出备用屏幕、显示光标、清屏。TUI 调用时 Bubble Tea
	// 的 alt screen 清理因进程替换不会执行，需手动恢复，避免退出会话后
	// 终端错位/光标消失/历史命令失效。
	resetTerminal()
	argv := append([]string{binary}, plan.Command.Args...)
	return syscall.Exec(binary, argv, os.Environ())
}

// resetTerminal 向终端发送恢复序列（退出备用屏幕、恢复光标、清屏）。
func resetTerminal() {
	// rmcup: 退出备用屏幕回到主屏幕；cnorm: 显示光标；clear: 清屏
	fmt.Fprint(os.Stdout, "\x1b[?1049l\x1b[?25h\x1b[2J\x1b[H")
}

// ResolveProgram 解析恢复程序路径。
func ResolveProgram(program string) (string, error) {
	b, err := exec.LookPath(program)
	if err != nil {
		return "", errors.New(i18n.Trf("executable %q not found; please install the corresponding agent", "未找到可执行文件 %q，请确认已安装对应 Agent", program))
	}
	return b, nil
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
