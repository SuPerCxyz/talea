package resume

import (
	"testing"

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
