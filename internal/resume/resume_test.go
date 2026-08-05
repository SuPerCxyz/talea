package resume

import (
	"testing"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/model"
)

func TestApplyPathMapping(t *testing.T) {
	m := map[string]string{
		"/home/user/old-projects": "/home/user/projects",
	}
	cases := []struct {
		dir    string
		want   string
		mapped bool
	}{
		{"/home/user/old-projects", "/home/user/projects", true},
		{"/home/user/old-projects/cinder", "/home/user/projects/cinder", true},
		{"/home/user/old-projectsx", "/home/user/old-projectsx", false},
		{"/home/user/other", "/home/user/other", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, mapped := ApplyPathMapping(c.dir, m)
		if got != c.want || mapped != c.mapped {
			t.Fatalf("dir=%q got=(%q,%v) want=(%q,%v)", c.dir, got, mapped, c.want, c.mapped)
		}
	}
}

func TestApplyPathMappingLongestPrefix(t *testing.T) {
	m := map[string]string{
		"/data":     "/mnt/data",
		"/data/sub": "/mnt/sub",
	}
	// 最长前缀匹配：/data/sub 优先
	got, _ := ApplyPathMapping("/data/sub/deep", m)
	if got != "/mnt/sub/deep" {
		t.Fatalf("got %q", got)
	}
}

func TestBuildEmptyDir(t *testing.T) {
	_, err := Build(model.Session{}, "", nil)
	if err == nil {
		t.Fatal("expected error for empty working dir")
	}
}

func TestBuildWithOverride(t *testing.T) {
	s := model.Session{WorkingDirectory: "/old/path"}
	plan, err := Build(s, "/new/path", nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.TargetDir != "/new/path" {
		t.Fatalf("target: %q", plan.TargetDir)
	}
}

func TestBuildWithMapping(t *testing.T) {
	s := model.Session{WorkingDirectory: "/home/u/old/proj"}
	plan, err := Build(s, "", map[string]string{
		"/home/u/old": "/home/u/new",
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.TargetDir != "/home/u/new/proj" {
		t.Fatalf("target: %q", plan.TargetDir)
	}
	if !plan.DirMapped {
		t.Fatal("expected mapped=true")
	}
}

// TestResumeCommandArgsNotShellInterpreted 验证恶意 Session ID / cwd 作为参数数组
// 传递时不会被 shell 解释（无 sh -c 拼接）。
func TestResumeCommandArgsNotShellInterpreted(t *testing.T) {
	// 恶意 session id 包含 shell 元字符
	malicious := []string{
		"8f463a2e; rm -rf /",
		"$(id)",
		"`whoami`",
		"abc && shutdown",
		"x|ls",
		"a' OR '1'='1",
	}
	for _, sid := range malicious {
		s := model.Session{SessionID: sid, WorkingDirectory: "/safe/dir"}
		plan, err := Build(s, "", nil)
		if err != nil {
			t.Fatalf("session %q: %v", sid, err)
		}
		// 构造命令：参数应原样保留，不做 shell 展开
		cmd := adapters.Command{Program: "claude", Args: []string{"--resume", sid}}
		_ = plan
		if len(cmd.Args) != 2 || cmd.Args[1] != sid {
			t.Fatalf("session %q: args altered: %v", sid, cmd.Args)
		}
	}
}

// TestWorkingDirPassedAsArg 验证包含引号/空格/中文的目录原样传入。
func TestWorkingDirPassedAsArg(t *testing.T) {
	dirs := []string{
		"/home/user/code/cinder",
		"/tmp/dir with space",
		"/tmp/中文目录/项目",
		"/tmp/it's here",
		`/tmp/quote"dir`,
	}
	for _, dir := range dirs {
		s := model.Session{SessionID: "s1", WorkingDirectory: dir}
		plan, err := Build(s, "", nil)
		if err != nil {
			t.Fatalf("dir %q: %v", dir, err)
		}
		if plan.TargetDir != dir {
			t.Fatalf("dir %q altered to %q", dir, plan.TargetDir)
		}
	}
}
