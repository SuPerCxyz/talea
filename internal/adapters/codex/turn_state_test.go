package codex

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/model"
)

func parseCodexContent(t *testing.T, content string) *model.Session {
	t.Helper()
	path := filepath.Join(t.TempDir(), "turn.jsonl")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := New().ParseMetadata(context.Background(),
		model.AgentInstance{InstanceID: "t", AgentID: model.AgentCodexCLI},
		adapters.SessionSource{SessionID: "s", Path: path})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestTurnStateCompleted(t *testing.T) {
	s := parseCodexContent(t, `{"type":"session_meta","timestamp":"2026-07-28T06:02:57Z","payload":{"session_id":"s"}}
{"type":"event_msg","timestamp":"2026-07-28T06:03:00Z","payload":{"type":"task_started"}}
{"type":"event_msg","timestamp":"2026-07-28T06:04:00Z","payload":{"type":"task_complete"}}
`)
	if s.TurnState != model.TurnStateCompleted || s.TurnStateSource != model.TurnStateSourceNative {
		t.Fatalf("state=%q source=%q", s.TurnState, s.TurnStateSource)
	}
	if s.TurnStateAt == nil || s.TurnStateAt.Format("15:04") != "06:04" {
		t.Fatalf("state_at=%v", s.TurnStateAt)
	}
}

func TestTurnStateWorking(t *testing.T) {
	s := parseCodexContent(t, `{"type":"session_meta","timestamp":"2026-07-28T06:02:57Z","payload":{"session_id":"s"}}
{"type":"event_msg","timestamp":"2026-07-28T06:03:00Z","payload":{"type":"task_started"}}
`)
	if s.TurnState != model.TurnStateWorking || s.TurnStateSource != model.TurnStateSourceNative {
		t.Fatalf("state=%q source=%q", s.TurnState, s.TurnStateSource)
	}
}

func TestTurnStateInterrupted(t *testing.T) {
	s := parseCodexContent(t, `{"type":"session_meta","timestamp":"2026-07-28T06:02:57Z","payload":{"session_id":"s"}}
{"type":"event_msg","timestamp":"2026-07-28T06:03:00Z","payload":{"type":"task_started"}}
{"type":"event_msg","timestamp":"2026-07-28T06:04:00Z","payload":{"type":"turn_aborted","reason":"user interrupt"}}
`)
	if s.TurnState != model.TurnStateInterrupted {
		t.Fatalf("state=%q", s.TurnState)
	}
}

func TestTurnStateFailedAbort(t *testing.T) {
	s := parseCodexContent(t, `{"type":"session_meta","timestamp":"2026-07-28T06:02:57Z","payload":{"session_id":"s"}}
{"type":"event_msg","timestamp":"2026-07-28T06:04:00Z","payload":{"type":"turn_aborted","reason":"stream error"}}
`)
	if s.TurnState != model.TurnStateFailed {
		t.Fatalf("state=%q", s.TurnState)
	}
}

func TestTurnStateWaitingApproval(t *testing.T) {
	s := parseCodexContent(t, `{"type":"session_meta","timestamp":"2026-07-28T06:02:57Z","payload":{"session_id":"s"}}
{"type":"event_msg","timestamp":"2026-07-28T06:03:00Z","payload":{"type":"exec_approval_request"}}
`)
	if s.TurnState != model.TurnStateWaitingInput || s.TurnStateSource != model.TurnStateSourceNative {
		t.Fatalf("state=%q source=%q", s.TurnState, s.TurnStateSource)
	}
}

func TestTurnStateUnknown(t *testing.T) {
	s := parseCodexContent(t, `{"type":"session_meta","timestamp":"2026-07-28T06:02:57Z","payload":{"session_id":"s"}}
`)
	if s.TurnState != model.TurnStateUnknown || s.TurnStateSource != model.TurnStateSourceUnknown {
		t.Fatalf("state=%q source=%q", s.TurnState, s.TurnStateSource)
	}
}
