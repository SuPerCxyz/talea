package codex

import (
	"context"
	"os"
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

func TestTurnContextPoliciesAreRestored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "turn-context.jsonl")
	content := `{"type":"session_meta","payload":{"session_id":"s"}}
{"type":"turn_context","payload":{"approval_policy":"never","sandbox_policy":{"type":"danger-full-access"}}}
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := New().ParseMetadata(context.Background(),
		model.AgentInstance{InstanceID: "t", AgentID: model.AgentCodexCLI},
		adapters.SessionSource{SessionID: "s", Path: path})
	if err != nil {
		t.Fatal(err)
	}
	cmd, err := New().BuildResumeCommand(*s, "/work")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--dangerously-bypass-approvals-and-sandbox", "resume", "s"}
	if !sameArgs(cmd.Args, want) {
		t.Fatalf("args=%v want=%v", cmd.Args, want)
	}
}

func TestLatestTurnContextPolicyWins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "turn-context-history.jsonl")
	content := `{"type":"session_meta","payload":{"session_id":"s"}}
{"type":"turn_context","payload":{"approval_policy":"on-request","sandbox_policy":{"type":"workspace-write"}}}
{"type":"turn_context","payload":{"approval_policy":"never","sandbox_policy":{"type":"danger-full-access"}}}
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := New().ParseMetadata(context.Background(),
		model.AgentInstance{InstanceID: "t", AgentID: model.AgentCodexCLI},
		adapters.SessionSource{SessionID: "s", Path: path})
	if err != nil {
		t.Fatal(err)
	}
	cmd, err := New().BuildResumeCommand(*s, "/work")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--dangerously-bypass-approvals-and-sandbox", "resume", "s"}
	if !sameArgs(cmd.Args, want) {
		t.Fatalf("args=%v want=%v", cmd.Args, want)
	}
}

func TestUnknownTurnContextPolicyIsIgnored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "turn-context.jsonl")
	content := `{"type":"turn_context","payload":{"approval_policy":"always","sandbox_policy":{"type":"unknown"}}}
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := New().ParseMetadata(context.Background(),
		model.AgentInstance{InstanceID: "t", AgentID: model.AgentCodexCLI},
		adapters.SessionSource{SessionID: "s", Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.ResumeLaunchArgs) != 0 {
		t.Fatalf("unknown policy args=%v", s.ResumeLaunchArgs)
	}
}

func TestCodexPartialPolicyArgs(t *testing.T) {
	if got := codexResumeArgs("never", ""); !sameArgs(got, []string{"--ask-for-approval", "never"}) {
		t.Fatalf("approval args=%v", got)
	}
	if got := codexResumeArgs("", "workspace-write"); !sameArgs(got, []string{"--sandbox", "workspace-write"}) {
		t.Fatalf("sandbox args=%v", got)
	}
}

func TestInvalidPersistedPolicyArgsAreIgnored(t *testing.T) {
	cmd, err := New().BuildResumeCommand(model.Session{
		SessionID:        "s",
		ResumeLaunchArgs: []string{"--shell", "rm -rf /"},
	}, "/work")
	if err != nil {
		t.Fatal(err)
	}
	if !sameArgs(cmd.Args, []string{"resume", "s"}) {
		t.Fatalf("args=%v", cmd.Args)
	}
}

func sameArgs(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
