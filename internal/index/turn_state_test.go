package index

import (
	"context"
	"testing"
	"time"

	"github.com/talea/talea/internal/model"
)

func TestMigrateCreatesTurnStateTable(t *testing.T) {
	db := newTestDB(t)
	var name string
	err := db.sql.QueryRow(`SELECT name FROM sqlite_master
		WHERE type='table' AND name='session_turn_state'`).Scan(&name)
	if err != nil {
		t.Fatalf("session_turn_state 表不存在: %v", err)
	}
	for _, idx := range []string{
		"idx_session_turn_state_state",
		"idx_session_turn_state_state_at",
	} {
		var got string
		if err := db.sql.QueryRow(`SELECT name FROM sqlite_master
			WHERE type='index' AND name=?`, idx).Scan(&got); err != nil {
			t.Fatalf("索引 %s 不存在: %v", idx, err)
		}
	}
}

func TestMigrateTurnStateIsIdempotent(t *testing.T) {
	db := newTestDB(t)
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("重复迁移失败: %v", err)
	}
	var n int
	if err := db.sql.QueryRow(`SELECT COUNT(*) FROM sqlite_master
		WHERE type='table' AND name='session_turn_state'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("重复迁移后表数量=%d err=%v", n, err)
	}
}

func TestUpsertSessionPersistsAndOverwritesTurnState(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	at := time.Unix(1786000000, 0)

	s := mkSession("turn-1")
	s.TurnState = model.TurnStateWorking
	s.TurnStateAt = &at
	s.TurnStateSource = model.TurnStateSourceNative
	s.TurnStateDetail = "task_started"
	if err := db.UpsertSession(ctx, s); err != nil {
		t.Fatal(err)
	}

	states, err := db.LoadTurnStates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rec, ok := states[model.TurnStateKey(s.AgentInstanceID, s.SessionID)]
	if !ok {
		t.Fatal("未加载到回合状态")
	}
	if rec.State != model.TurnStateWorking || rec.Source != model.TurnStateSourceNative {
		t.Fatalf("state=%q source=%q", rec.State, rec.Source)
	}
	if rec.StateAt == nil || rec.StateAt.Unix() != at.Unix() {
		t.Fatalf("state_at=%v", rec.StateAt)
	}
	if rec.Detail != "task_started" {
		t.Fatalf("detail=%q", rec.Detail)
	}

	// 覆盖为终态
	at2 := at.Add(time.Minute)
	s.TurnState = model.TurnStateCompleted
	s.TurnStateAt = &at2
	s.TurnStateSource = model.TurnStateSourceInferred
	s.TurnStateDetail = "turn ended"
	if err := db.UpsertSession(ctx, s); err != nil {
		t.Fatal(err)
	}
	states, err = db.LoadTurnStates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rec = states[model.TurnStateKey(s.AgentInstanceID, s.SessionID)]
	if rec.State != model.TurnStateCompleted || rec.StateAt == nil || rec.StateAt.Unix() != at2.Unix() {
		t.Fatalf("覆盖后 state=%q at=%v", rec.State, rec.StateAt)
	}
}

func TestTurnStateDefaultsToUnknown(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	s := mkSession("turn-default")
	if err := db.UpsertSession(ctx, s); err != nil {
		t.Fatal(err)
	}
	states, err := db.LoadTurnStates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rec, ok := states[model.TurnStateKey(s.AgentInstanceID, s.SessionID)]
	if !ok {
		t.Fatal("无状态会话也应有 unknown 行")
	}
	if rec.State != model.TurnStateUnknown || rec.Source != model.TurnStateSourceUnknown {
		t.Fatalf("默认状态=%q/%q", rec.State, rec.Source)
	}
}

func TestLoadTurnStatesMergesByCompositeKey(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	at := time.Unix(1786000000, 0)
	a := mkSession("same-id")
	a.AgentInstanceID = "inst-a"
	a.TurnState = model.TurnStateCompleted
	a.TurnStateAt = &at
	a.TurnStateSource = model.TurnStateSourceNative
	b := mkSession("same-id")
	b.AgentInstanceID = "inst-b"
	b.TurnState = model.TurnStateFailed
	b.TurnStateAt = &at
	b.TurnStateSource = model.TurnStateSourceNative

	if _, err := db.UpsertMany(ctx, []*model.Session{a, b}); err != nil {
		t.Fatal(err)
	}
	states, err := db.LoadTurnStates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	recA := states[model.TurnStateKey("inst-a", "same-id")]
	recB := states[model.TurnStateKey("inst-b", "same-id")]
	if recA.State != model.TurnStateCompleted || recB.State != model.TurnStateFailed {
		t.Fatalf("合并错误: a=%q b=%q", recA.State, recB.State)
	}
	if _, ok := states[model.TurnStateKey("missing", "same-id")]; ok {
		t.Fatal("缺失项不应出现在结果中")
	}
}

func TestInsertIfNewWritesTurnState(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	s := mkSession("insert-state")
	s.TurnState = model.TurnStateWaitingInput
	s.TurnStateSource = model.TurnStateSourceHook
	inserted, err := db.InsertIfNew(ctx, s)
	if err != nil || !inserted {
		t.Fatalf("inserted=%v err=%v", inserted, err)
	}
	states, err := db.LoadTurnStates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rec := states[model.TurnStateKey(s.AgentInstanceID, s.SessionID)]
	if rec.State != model.TurnStateWaitingInput || rec.Source != model.TurnStateSourceHook {
		t.Fatalf("insert 状态=%q/%q", rec.State, rec.Source)
	}

	// 已存在时跳过，不得覆盖新状态
	s.TurnState = model.TurnStateCompleted
	inserted, err = db.InsertIfNew(ctx, s)
	if err != nil || inserted {
		t.Fatalf("重复插入 inserted=%v err=%v", inserted, err)
	}
	states, _ = db.LoadTurnStates(ctx)
	if got := states[model.TurnStateKey(s.AgentInstanceID, s.SessionID)].State; got != model.TurnStateWaitingInput {
		t.Fatalf("重复插入覆盖了状态: %q", got)
	}
}
