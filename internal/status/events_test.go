package status

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/talea/talea/internal/model"
)

func TestAppendEventCreatesPrivateDirAndFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	path := filepath.Join(dir, EventFileName)
	at := testTime()
	if err := AppendEvent(path, Event{
		AgentID:   string(model.AgentClaudeCode),
		SessionID: "s1",
		State:     model.TurnStateWaitingInput,
		At:        at,
		Detail:    "permission_prompt",
	}); err != nil {
		t.Fatal(err)
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("目录权限=%s", dirInfo.Mode().Perm())
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("文件权限=%s", fileInfo.Mode().Perm())
	}
}

func TestLoadLatestEventsSkipsBadLinesAndKeepsNewest(t *testing.T) {
	path := filepath.Join(t.TempDir(), EventFileName)
	content := strings.Join([]string{
		`{"agent_id":"claude-code","session_id":"s1","state":"working","at":"2026-09-01T11:00:00Z"}`,
		`not-json`,
		`{"agent_id":"claude-code","session_id":"s1","state":"completed","at":"2026-09-01T11:05:00Z"}`,
		`{"agent_id":"claude-code","session_id":"s1","state":"working","at":"2026-09-01T11:01:00Z"}`,
		`{"agent_id":"codex-cli","session_id":"s2","state":"failed","at":"2026-09-01T11:02:00Z"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	idx, err := LoadLatestEvents(path)
	if err != nil {
		t.Fatal(err)
	}
	e1, ok := idx.Latest(model.AgentClaudeCode, "i", "s1")
	if !ok || e1.State != model.TurnStateCompleted {
		t.Fatalf("最新事件=%+v ok=%v", e1, ok)
	}
	e2, ok := idx.Latest(model.AgentCodexCLI, "i", "s2")
	if !ok || e2.State != model.TurnStateFailed {
		t.Fatalf("s2=%+v ok=%v", e2, ok)
	}
}

func TestLoadLatestEventsInstanceIsolation(t *testing.T) {
	path := filepath.Join(t.TempDir(), EventFileName)
	content := strings.Join([]string{
		`{"agent_id":"opencode","agent_instance_id":"inst-a","session_id":"s","state":"waiting_input","at":"2026-09-01T11:00:00Z"}`,
		`{"agent_id":"opencode","session_id":"s","state":"completed","at":"2026-09-01T11:05:00Z"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	idx, err := LoadLatestEvents(path)
	if err != nil {
		t.Fatal(err)
	}
	if e, ok := idx.Latest(model.AgentOpenCode, "inst-a", "s"); !ok || e.State != model.TurnStateWaitingInput {
		t.Fatalf("实例事件=%+v ok=%v", e, ok)
	}
	if e, ok := idx.Latest(model.AgentOpenCode, "inst-b", "s"); !ok || e.State != model.TurnStateCompleted {
		t.Fatalf("Agent 回退事件=%+v ok=%v", e, ok)
	}
}

func TestLoadLatestEventsMissingEmptyAndLargeFile(t *testing.T) {
	idx, err := LoadLatestEvents(filepath.Join(t.TempDir(), "missing.jsonl"))
	if err != nil || len(idx.byAgent) != 0 || len(idx.byInstance) != 0 {
		t.Fatalf("缺失文件: %+v err=%v", idx, err)
	}

	empty := filepath.Join(t.TempDir(), EventFileName)
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	idx, err = LoadLatestEvents(empty)
	if err != nil || len(idx.byAgent) != 0 {
		t.Fatalf("空文件: %+v err=%v", idx, err)
	}

	large := filepath.Join(t.TempDir(), EventFileName)
	filler := strings.Repeat("x", MaxEventReadBytes+100)
	content := filler + "\n" +
		`{"agent_id":"claude-code","session_id":"tail","state":"completed","at":"2026-09-01T11:05:00Z"}` + "\n"
	if err := os.WriteFile(large, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	idx, err = LoadLatestEvents(large)
	if err != nil {
		t.Fatal(err)
	}
	if e, ok := idx.Latest(model.AgentClaudeCode, "i", "tail"); !ok || e.State != model.TurnStateCompleted {
		t.Fatalf("大文件尾部事件=%+v ok=%v", e, ok)
	}
}

func TestLoadLatestEventsInvalidStatesSkipped(t *testing.T) {
	path := filepath.Join(t.TempDir(), EventFileName)
	content := strings.Join([]string{
		`{"agent_id":"claude-code","session_id":"s","state":"bogus","at":"2026-09-01T11:00:00Z"}`,
		`{"agent_id":"claude-code","session_id":"s","state":"unknown","at":"2026-09-01T11:01:00Z"}`,
		`{"agent_id":"","session_id":"s","state":"completed","at":"2026-09-01T11:02:00Z"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	idx, err := LoadLatestEvents(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := idx.Latest(model.AgentClaudeCode, "i", "s"); ok {
		t.Fatal("无效事件不应生效")
	}
}

func TestAppendEventRepairsMissingTrailingNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), EventFileName)
	first := `{"agent_id":"claude-code","session_id":"s1","state":"working","at":"2026-09-01T11:00:00Z"}`
	if err := os.WriteFile(path, []byte(first), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := AppendEvent(path, Event{
		AgentID: string(model.AgentClaudeCode), SessionID: "s2",
		State: model.TurnStateCompleted, At: testTime(),
	}); err != nil {
		t.Fatal(err)
	}
	idx, err := LoadLatestEvents(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := idx.Latest(model.AgentClaudeCode, "i", "s1"); !ok {
		t.Fatal("原有事件应保持可读")
	}
	if e, ok := idx.Latest(model.AgentClaudeCode, "i", "s2"); !ok || e.State != model.TurnStateCompleted {
		t.Fatalf("新事件=%+v ok=%v", e, ok)
	}
}

func TestAppendEventAccumulatesJSONL(t *testing.T) {
	path := filepath.Join(t.TempDir(), EventFileName)
	for _, state := range []model.TurnState{model.TurnStateWorking, model.TurnStateCompleted} {
		if err := AppendEvent(path, Event{
			AgentID: string(model.AgentClaudeCode), SessionID: "s",
			State: state, At: time.Now(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Count(string(data), "\n"); lines != 2 {
		t.Fatalf("JSONL 行数=%d content=%q", lines, data)
	}
}
