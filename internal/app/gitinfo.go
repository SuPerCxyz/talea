package app

import (
	"os/exec"
	"strings"
)

// fillGitInfoDir 读取工作目录的 Git 信息，失败时返回零值，不影响会话显示。
func fillGitInfoDir(cwd string) (g struct {
	root, branch, remote string
}) {
	root := gitRoot(cwd)
	if root == "" {
		return g
	}
	g.root = root
	g.branch = gitBranch(root)
	g.remote = gitRemote(root)
	return g
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
