package codex

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/model"
)

func fixturePath(name string) string {
	return filepath.Join("..", "..", "..", "testdata", "codex", name)
}

func TestParseMetadataSimple(t *testing.T) {
	ctx := context.Background()
	a := New()
	src := adapters.SessionSource{
		Path: fixturePath("simple.jsonl"),
	}
	inst := model.AgentInstance{InstanceID: "t", AgentID: model.AgentCodexCLI}
	s, err := a.ParseMetadata(ctx, inst, src)
	if err != nil {
		t.Fatal(err)
	}
	// 首次提问必须过滤 AGENTS.md 注入块与环境上下文
	if s.FirstQuestion != "请修改 env-debugging skill 的输出要求，禁止输出未验证内容。" {
		t.Fatalf("first question: %q", s.FirstQuestion)
	}
	if s.SessionID != "33333333-cccc-73b3-859f-b835fe86b564" {
		t.Fatalf("session id: %q", s.SessionID)
	}
	if s.WorkingDirectory != "/home/alice/code/my-skills" {
		t.Fatalf("cwd: %q", s.WorkingDirectory)
	}
	if s.GitBranch != "master" {
		t.Fatalf("branch: %q", s.GitBranch)
	}
	if s.GitRemote != "ssh://git@example.com/superc/my-skills.git" {
		t.Fatalf("remote: %q", s.GitRemote)
	}
	if s.StartTimeSource != model.TimeSourceSessionMeta {
		t.Fatalf("start source: %q", s.StartTimeSource)
	}
	if !s.HasTokenUsage || s.TokenUsage == nil {
		t.Fatal("expected token usage")
	}
	if s.TokenUsage.InputTokens == nil || *s.TokenUsage.InputTokens != 21154 {
		t.Fatalf("input: %v", s.TokenUsage.InputTokens)
	}
	if s.TokenUsage.CacheReadTokens == nil || *s.TokenUsage.CacheReadTokens != 8000 {
		t.Fatalf("cache read: %v", s.TokenUsage.CacheReadTokens)
	}
	if s.TokenUsage.ReasoningTokens == nil || *s.TokenUsage.ReasoningTokens != 107 {
		t.Fatalf("reasoning: %v", s.TokenUsage.ReasoningTokens)
	}
}

func TestSessionIDFromFilename(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"/sessions/2026/08/03/rollout-2026-08-03T14-01-19-019fc636-5bf9-73b3-859f-b835fe86b564.jsonl",
			"019fc636-5bf9-73b3-859f-b835fe86b564"},
	}
	for _, c := range cases {
		if got := sessionIDFromFilename(c.path); got != c.want {
			t.Fatalf("got %q want %q", got, c.want)
		}
	}
}

func TestBuildResumeCommand(t *testing.T) {
	a := New()
	cmd, err := a.BuildResumeCommand(model.Session{SessionID: "019f-xyz"}, "/work")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Program != "codex" {
		t.Fatalf("program: %q", cmd.Program)
	}
	if len(cmd.Args) != 2 || cmd.Args[0] != "resume" {
		t.Fatalf("args: %v", cmd.Args)
	}
}
