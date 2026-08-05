package app

import (
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/talea/talea/internal/model"
)

// fillGitInfo 读取工作目录的 Git 信息，失败不影响会话显示。
func fillGitInfo(s *model.Session) {
	root := gitRoot(s.WorkingDirectory)
	if root == "" {
		return
	}
	s.GitRoot = root
	if name := filepath.Base(root); name != "" {
		s.ProjectName = name
	}
	if s.GitBranch == "" {
		s.GitBranch = gitBranch(root)
	}
	if s.GitRemote == "" {
		s.GitRemote = gitRemote(root)
	}
}

func gitRoot(cwd string) string {
	out, err := exec.Command("git", "-C", cwd, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func gitBranch(root string) string {
	out, err := exec.Command("git", "-C", root, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func gitRemote(root string) string {
	out, err := exec.Command("git", "-C", root, "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
