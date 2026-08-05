package claude

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/model"
)

func fixturePath(name string) string {
	return filepath.Join("..", "..", "..", "testdata", "claude", name)
}

func TestParseMetadataSimple(t *testing.T) {
	ctx := context.Background()
	a := New()
	src := adapters.SessionSource{
		SessionID: "11111111-aaaa-4f46-99eb-32346319749e",
		Path:      fixturePath("simple.jsonl"),
	}
	inst := model.AgentInstance{
		InstanceID: "test-instance",
		AgentID:    model.AgentClaudeCode,
	}
	s, err := a.ParseMetadata(ctx, inst, src)
	if err != nil {
		t.Fatal(err)
	}
	if s.FirstQuestion != "请设计一个 VSCode 插件，支持 Git 历史可视化。" {
		t.Fatalf("first question: %q", s.FirstQuestion)
	}
	if s.WorkingDirectory != "/home/alice/code/pentimento" {
		t.Fatalf("cwd: %q", s.WorkingDirectory)
	}
	if s.GitBranch != "feature/audit" {
		t.Fatalf("branch: %q", s.GitBranch)
	}
	if s.StartedAt == nil {
		t.Fatal("startedAt nil")
	}
	if s.StartTimeSource != model.TimeSourceFirstUserMsg {
		t.Fatalf("start source: %q", s.StartTimeSource)
	}
	if s.EndedAt == nil {
		t.Fatal("endedAt nil")
	}
	if s.EndTimeSource != model.TimeSourceLastActivity {
		t.Fatalf("end source: %q", s.EndTimeSource)
	}
	if !s.HasTokenUsage || s.TokenUsage == nil {
		t.Fatal("expected token usage")
	}
	if s.TokenUsage.InputTokens == nil || *s.TokenUsage.InputTokens != 6400 {
		t.Fatalf("input tokens: %v", s.TokenUsage.InputTokens)
	}
	if s.TokenUsage.OutputTokens == nil || *s.TokenUsage.OutputTokens != 2140 {
		t.Fatalf("output tokens: %v", s.TokenUsage.OutputTokens)
	}
	if s.UserMessageCount != 2 {
		t.Fatalf("user msg count: %d", s.UserMessageCount)
	}
}

func TestParseMetadataSystemReminderFiltered(t *testing.T) {
	ctx := context.Background()
	a := New()
	src := adapters.SessionSource{
		SessionID: "22222222-bbbb-4f46-99eb-32346319749e",
		Path:      fixturePath("system-reminder.jsonl"),
	}
	inst := model.AgentInstance{InstanceID: "t", AgentID: model.AgentClaudeCode}
	s, err := a.ParseMetadata(ctx, inst, src)
	if err != nil {
		t.Fatal(err)
	}
	// 系统提醒块被剥离，仅保留真实提问
	if s.FirstQuestion != "第一条真实提问在这里。" {
		t.Fatalf("first question: %q", s.FirstQuestion)
	}
}

func TestParseMetadataMissingFile(t *testing.T) {
	ctx := context.Background()
	a := New()
	src := adapters.SessionSource{SessionID: "x", Path: "/nonexistent/x.jsonl"}
	inst := model.AgentInstance{InstanceID: "t", AgentID: model.AgentClaudeCode}
	if _, err := a.ParseMetadata(ctx, inst, src); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestBuildResumeCommand(t *testing.T) {
	a := New()
	cmd, err := a.BuildResumeCommand(model.Session{SessionID: "abc-123"}, "/work")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Program != "claude" {
		t.Fatalf("program: %q", cmd.Program)
	}
	if len(cmd.Args) != 2 || cmd.Args[0] != "--resume" || cmd.Args[1] != "abc-123" {
		t.Fatalf("args: %v", cmd.Args)
	}
}

func TestInfo(t *testing.T) {
	info := New().Info()
	if info.ID != model.AgentClaudeCode {
		t.Fatalf("id: %q", info.ID)
	}
	if len(info.Capabilities) == 0 {
		t.Fatal("no capabilities")
	}
}
