package claude

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/model"
)

func parseClaudeContent(t *testing.T, content string) *model.Session {
	t.Helper()
	path := filepath.Join(t.TempDir(), "turn.jsonl")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := New().ParseMetadata(context.Background(),
		model.AgentInstance{InstanceID: "t", AgentID: model.AgentClaudeCode},
		adapters.SessionSource{SessionID: "s", Path: path})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestTurnStateCompletedByEndTurn(t *testing.T) {
	s := parseClaudeContent(t, `{"type":"user","timestamp":"2026-07-17T04:57:32Z","message":{"role":"user","content":"hi"}}
{"type":"assistant","timestamp":"2026-07-17T04:58:00Z","message":{"role":"assistant","content":[{"type":"text","text":"done"}],"stop_reason":"end_turn"}}
`)
	if s.TurnState != model.TurnStateCompleted || s.TurnStateSource != model.TurnStateSourceNative {
		t.Fatalf("state=%q source=%q", s.TurnState, s.TurnStateSource)
	}
	if s.TurnStateAt == nil || s.TurnStateAt.Format("15:04") != "04:58" {
		t.Fatalf("state_at=%v", s.TurnStateAt)
	}
}

func TestTurnStateCompletedByTurnDuration(t *testing.T) {
	s := parseClaudeContent(t, `{"type":"user","timestamp":"2026-07-17T04:57:32Z","message":{"role":"user","content":"hi"}}
{"type":"system","subtype":"turn_duration","timestamp":"2026-07-17T04:58:05Z"}
`)
	if s.TurnState != model.TurnStateCompleted {
		t.Fatalf("state=%q", s.TurnState)
	}
	if s.TurnStateDetail != "system.turn_duration" {
		t.Fatalf("detail=%q", s.TurnStateDetail)
	}
}

func TestTurnStateWorkingOnToolUse(t *testing.T) {
	s := parseClaudeContent(t, `{"type":"user","timestamp":"2026-07-17T04:57:32Z","message":{"role":"user","content":"hi"}}
{"type":"assistant","timestamp":"2026-07-17T04:58:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tool-1"}],"stop_reason":"tool_use"}}
`)
	if s.TurnState != model.TurnStateWorking {
		t.Fatalf("state=%q", s.TurnState)
	}
	if s.TurnStateDetail != "assistant tool_use" {
		t.Fatalf("detail=%q", s.TurnStateDetail)
	}
}

func TestTurnStateFailedOnAPIError(t *testing.T) {
	s := parseClaudeContent(t, `{"type":"assistant","timestamp":"2026-07-17T04:58:00Z","isApiErrorMessage":true,"message":{"role":"assistant","content":[{"type":"text","text":"API error"}]}}
`)
	if s.TurnState != model.TurnStateFailed || s.TurnStateSource != model.TurnStateSourceNative {
		t.Fatalf("state=%q source=%q", s.TurnState, s.TurnStateSource)
	}
}

func TestTurnStateUnknownWithoutTurnEvents(t *testing.T) {
	s := parseClaudeContent(t, `{"type":"permission-mode","permissionMode":"plan","timestamp":"2026-07-17T04:58:00Z"}
{"type":"summary","timestamp":"2026-07-17T04:59:00Z","summary":"x"}
`)
	if s.TurnState != model.TurnStateUnknown || s.TurnStateSource != model.TurnStateSourceUnknown {
		t.Fatalf("state=%q source=%q", s.TurnState, s.TurnStateSource)
	}
	if s.TurnStateAt != nil {
		t.Fatalf("unknown 不应有时间: %v", s.TurnStateAt)
	}
}

func TestTurnStateOlderEventDoesNotOverrideNewer(t *testing.T) {
	s := parseClaudeContent(t, `{"type":"system","subtype":"turn_duration","timestamp":"2026-07-17T05:00:00Z"}
{"type":"user","timestamp":"2026-07-17T04:59:00Z","message":{"role":"user","content":"late"}}
`)
	if s.TurnState != model.TurnStateCompleted {
		t.Fatalf("旧事件覆盖了新状态: %q", s.TurnState)
	}
}
