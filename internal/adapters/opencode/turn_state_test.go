package opencode

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/model"
)

const turnSessionTableDDL = `CREATE TABLE session (
	id TEXT, project_id TEXT, workspace_id TEXT, parent_id TEXT, slug TEXT,
	directory TEXT, path TEXT, title TEXT, version TEXT, metadata TEXT,
	cost REAL, tokens_input INTEGER, tokens_output INTEGER,
	tokens_reasoning INTEGER, tokens_cache_read INTEGER, tokens_cache_write INTEGER,
	time_created INTEGER, time_updated INTEGER, time_compacting INTEGER,
	time_archived INTEGER, agent TEXT, model TEXT)`

const turnMessageTableDDL = `CREATE TABLE message (
	id TEXT, session_id TEXT, time_created INTEGER, time_updated INTEGER, data TEXT)`

func createTurnStateDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "opencode.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	execAll(t, db, []string{
		turnSessionTableDDL,
		turnMessageTableDDL,
		sessionV2DDL,
		sessionMessageDDL,
	})
	execAll(t, db, []string{
		`INSERT INTO session_v2 (id, directory, title, time_created, time_updated)
		 VALUES ('ses_idle_ok', '/w', 'ok', 1000, 2000)`,
		`INSERT INTO session_v2 (id, directory, title, time_created, time_updated, time_idle, idle_outcome)
		 VALUES ('ses_idle_col', '/w', 'col', 1000, 2000, 3000, 'failed')`,
		`INSERT INTO session_v2 (id, directory, title, time_created, time_updated, time_idle, idle_outcome)
		 VALUES ('ses_idle_old', '/w', 'old', 1000, 4000, 2500, 'succeeded')`,
		`INSERT INTO session_v2 (id, directory, title, time_created, time_updated)
		 VALUES ('ses_unknown', '/w', 'unknown', 1000, 2000)`,
		// 最新消息是 idle，outcome=succeeded
		`INSERT INTO session_message (id, session_id, type, seq, time_created, time_updated, data)
		 VALUES ('m1', 'ses_idle_ok', 'assistant', 1, 1500, 1500, '{}'),
		        ('m2', 'ses_idle_ok', 'idle', 2, 2000, 2000, '{"outcome":"succeeded"}')`,
		// time_idle 列标为 failed
		`INSERT INTO session_message (id, session_id, type, seq, time_created, time_updated, data)
		 VALUES ('m3', 'ses_idle_col', 'assistant', 1, 2000, 2000, '{}')`,
		// time_idle 早于最后一条消息：仍应视为进行中
		`INSERT INTO session_message (id, session_id, type, seq, time_created, time_updated, data)
		 VALUES ('m4', 'ses_idle_old', 'assistant', 1, 3500, 3500, '{}')`,
		// 传统表会话：最后一条 assistant
		`INSERT INTO session (id, directory, title, time_created, time_updated)
		 VALUES ('ses_legacy', '/w', 'legacy', 1000, 2000)`,
		`INSERT INTO message (id, session_id, time_created, time_updated, data)
		 VALUES ('lm1', 'ses_legacy', 2000, 2000, '{"role":"assistant","time":{"created":2000}}')`,
	})
	return path
}

func parseTurnSession(t *testing.T, path, id string) *model.Session {
	t.Helper()
	s, err := New().ParseMetadata(context.Background(),
		model.AgentInstance{InstanceID: "t", AgentID: model.AgentOpenCode},
		adapters.SessionSource{SessionID: id, Path: path})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestOpenCodeTurnStateIdleCompleted(t *testing.T) {
	s := parseTurnSession(t, createTurnStateDB(t), "ses_idle_ok")
	if s.TurnState != model.TurnStateCompleted || s.TurnStateSource != model.TurnStateSourceNative {
		t.Fatalf("state=%q source=%q", s.TurnState, s.TurnStateSource)
	}
	if s.TurnStateAt == nil || s.TurnStateAt.UnixMilli() != 2000 {
		t.Fatalf("state_at=%v", s.TurnStateAt)
	}
}

func TestOpenCodeTurnStateIdleColumnFailed(t *testing.T) {
	s := parseTurnSession(t, createTurnStateDB(t), "ses_idle_col")
	if s.TurnState != model.TurnStateFailed {
		t.Fatalf("state=%q", s.TurnState)
	}
	if s.TurnStateDetail != "idle outcome: failed" {
		t.Fatalf("detail=%q", s.TurnStateDetail)
	}
}

func TestOpenCodeTurnStateNewerMessageBeatsOldIdle(t *testing.T) {
	s := parseTurnSession(t, createTurnStateDB(t), "ses_idle_old")
	if s.TurnState != model.TurnStateWorking || s.TurnStateSource != model.TurnStateSourceInferred {
		t.Fatalf("state=%q source=%q", s.TurnState, s.TurnStateSource)
	}
}

func TestOpenCodeTurnStateLegacyWorking(t *testing.T) {
	s := parseTurnSession(t, createTurnStateDB(t), "ses_legacy")
	if s.TurnState != model.TurnStateWorking || s.TurnStateSource != model.TurnStateSourceInferred {
		t.Fatalf("state=%q source=%q", s.TurnState, s.TurnStateSource)
	}
	if s.TurnStateDetail != "assistant message" {
		t.Fatalf("detail=%q", s.TurnStateDetail)
	}
}

func TestOpenCodeTurnStateUnknown(t *testing.T) {
	s := parseTurnSession(t, createTurnStateDB(t), "ses_unknown")
	if s.TurnState != model.TurnStateUnknown || s.TurnStateSource != model.TurnStateSourceUnknown {
		t.Fatalf("state=%q source=%q", s.TurnState, s.TurnStateSource)
	}
}

func TestMapIdleOutcome(t *testing.T) {
	cases := map[string]model.TurnState{
		"succeeded":   model.TurnStateCompleted,
		"failed":      model.TurnStateFailed,
		"error":       model.TurnStateFailed,
		"interrupted": model.TurnStateInterrupted,
		"cancelled":   model.TurnStateInterrupted,
		"":            model.TurnStateIdle,
		"unknown":     model.TurnStateIdle,
	}
	for outcome, want := range cases {
		got, _, _, _ := mapIdleOutcome(outcome, 1000)
		if got != want {
			t.Errorf("outcome %q: got %q want %q", outcome, got, want)
		}
	}
}
