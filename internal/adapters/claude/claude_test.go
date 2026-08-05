package claude

import (
	"context"
	"os"
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
	// input 为累计上下文值：取最后非零值 5200
	if s.TokenUsage.InputTokens == nil || *s.TokenUsage.InputTokens != 5200 {
		t.Fatalf("input tokens: %v", s.TokenUsage.InputTokens)
	}
	// output 为增量：求和 340+1800=2140
	if s.TokenUsage.OutputTokens == nil || *s.TokenUsage.OutputTokens != 2140 {
		t.Fatalf("output tokens: %v", s.TokenUsage.OutputTokens)
	}
	if s.TokenUsage.TotalTokens == nil || *s.TokenUsage.TotalTokens != 7340 {
		t.Fatalf("total tokens: %v", s.TokenUsage.TotalTokens)
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

func TestFirstQuestionCommandOnly(t *testing.T) {
	// 首次提问只有命令，应保留
	dir := t.TempDir()
	path := filepath.Join(dir, "cmd.jsonl")
	content := `{"type":"user","timestamp":"2026-07-17T04:57:32.179Z","cwd":"/x","sessionId":"cmd-s","message":{"role":"user","content":"ls -la /var/log"}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	a := New()
	src := adapters.SessionSource{SessionID: "cmd-s", Path: path}
	s, err := a.ParseMetadata(ctx, model.AgentInstance{InstanceID: "t", AgentID: model.AgentClaudeCode}, src)
	if err != nil {
		t.Fatal(err)
	}
	if s.FirstQuestion != "ls -la /var/log" {
		t.Fatalf("first question: %q", s.FirstQuestion)
	}
}

func TestFirstQuestionANSI(t *testing.T) {
	// 首次提问含 ANSI 控制字符，应清理
	dir := t.TempDir()
	path := filepath.Join(dir, "ansi.jsonl")
	content := `{"type":"user","timestamp":"2026-07-17T04:57:32.179Z","cwd":"/x","sessionId":"ansi-s","message":{"role":"user","content":"\u001b[31m分析\u001b[0m残留原因"}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	a := New()
	src := adapters.SessionSource{SessionID: "ansi-s", Path: path}
	s, err := a.ParseMetadata(ctx, model.AgentInstance{InstanceID: "t", AgentID: model.AgentClaudeCode}, src)
	if err != nil {
		t.Fatal(err)
	}
	if s.FirstQuestion != "分析残留原因" {
		t.Fatalf("first question: %q", s.FirstQuestion)
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
