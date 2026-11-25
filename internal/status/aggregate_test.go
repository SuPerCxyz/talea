package status

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
)

func aggregateTestDB(t *testing.T) *index.DB {
	t.Helper()
	db, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestBuildMergesStatesAndEvents(t *testing.T) {
	ctx := context.Background()
	db := aggregateTestDB(t)
	now := testTime()
	older := now.Add(-2 * time.Hour)
	newer := now.Add(-10 * time.Minute)
	completedAt := now.Add(-30 * time.Minute)

	for _, s := range []*model.Session{
		{
			AgentID: model.AgentClaudeCode, AgentInstanceID: "i", SessionID: "old",
			WorkingDirectory: "/w/old", ProjectName: "old", LastActivityAt: &older,
			Activity: model.ActivityInactive, IndexedAt: now, UpdatedAt: now,
			TurnState: model.TurnStateWorking, TurnStateAt: &older,
			TurnStateSource: model.TurnStateSourceNative,
		},
		{
			AgentID: model.AgentOpenCode, AgentInstanceID: "i", SessionID: "recent",
			WorkingDirectory: "/w/recent", ProjectName: "recent", LastActivityAt: &newer,
			Activity: model.ActivityInactive, IndexedAt: now, UpdatedAt: now,
			TurnState: model.TurnStateCompleted, TurnStateAt: &completedAt,
			TurnStateSource: model.TurnStateSourceNative,
		},
	} {
		if err := db.UpsertSession(ctx, s); err != nil {
			t.Fatal(err)
		}
	}

	eventPath := filepath.Join(t.TempDir(), EventFileName)
	if err := AppendEvent(eventPath, Event{
		AgentID: string(model.AgentClaudeCode), SessionID: "old",
		State: model.TurnStateWaitingInput, At: now.Add(-time.Minute),
		Detail: "permission_prompt",
	}); err != nil {
		t.Fatal(err)
	}

	snap, err := Build(ctx, db, Options{Now: now, EventsPath: eventPath})
	if err != nil {
		t.Fatal(err)
	}
	if snap.Counts.Total != 2 || snap.Counts.Waiting != 1 || snap.Counts.Completed != 1 {
		t.Fatalf("counts=%+v", snap.Counts)
	}
	if snap.Counts.NeedsAttention != 1 {
		t.Fatalf("attention=%d", snap.Counts.NeedsAttention)
	}
	if snap.Sessions[0].SessionID != "recent" {
		t.Fatalf("排序错误: %+v", snap.Sessions)
	}
	for _, ss := range snap.Sessions {
		if ss.SessionID == "old" {
			if ss.State != StateWaiting || ss.Source != model.TurnStateSourceHook ||
				ss.Confidence != ConfidenceExact {
				t.Fatalf("hook 覆盖失败: %+v", ss)
			}
		}
	}
}

func TestBuildFiltersSessionsOutsideRecentWindow(t *testing.T) {
	ctx := context.Background()
	db := aggregateTestDB(t)
	now := testTime()
	old := now.Add(-25 * time.Hour)
	recent := now.Add(-time.Hour)
	workingAt := now.Add(-time.Minute)

	mk := func(id string, state model.TurnState, at *time.Time, activity model.ActivityState) *model.Session {
		s := &model.Session{
			AgentID: model.AgentClaudeCode, AgentInstanceID: "i", SessionID: id,
			LastActivityAt: at, Activity: activity, IndexedAt: now, UpdatedAt: now,
		}
		if at != nil {
			s.StartedAt = at
			s.EndedAt = at
		}
		if state != "" {
			s.TurnState = state
			s.TurnStateAt = at
			s.TurnStateSource = model.TurnStateSourceNative
		}
		return s
	}
	sessions := []*model.Session{
		mk("old-idle", model.TurnStateIdle, &old, model.ActivityInactive),
		mk("old-unknown", "", &old, model.ActivityInactive),
		mk("old-completed", model.TurnStateCompleted, &old, model.ActivityInactive),
		mk("recent-waiting", model.TurnStateWaitingInput, &recent, model.ActivityInactive),
		mk("recent-working", model.TurnStateWorking, &workingAt, model.ActivityInactive),
		mk("no-time-waiting", model.TurnStateWaitingInput, nil, model.ActivityInactive),
	}
	for _, s := range sessions {
		if err := db.UpsertSession(ctx, s); err != nil {
			t.Fatal(err)
		}
	}

	snap, err := Build(ctx, db, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Sessions) != 3 {
		t.Fatalf("可见会话=%d want=3: %+v", len(snap.Sessions), snap.Sessions)
	}
	seen := map[string]bool{}
	for _, ss := range snap.Sessions {
		seen[ss.SessionID] = true
	}
	for _, id := range []string{"old-idle", "old-unknown", "old-completed"} {
		if seen[id] {
			t.Fatalf("旧会话 %s 不应展示", id)
		}
	}
	for _, id := range []string{"recent-waiting", "recent-working", "no-time-waiting"} {
		if !seen[id] {
			t.Fatalf("窗口内会话 %s 应展示", id)
		}
	}
	if snap.Counts.Total != len(snap.Sessions) {
		t.Fatalf("计数不一致 total=%d sessions=%d", snap.Counts.Total, len(snap.Sessions))
	}
	if snap.Counts.Waiting != 2 || snap.Counts.Working != 1 || snap.Counts.Completed != 0 ||
		snap.Counts.Idle != 0 || snap.Counts.Unknown != 0 {
		t.Fatalf("计数=%+v", snap.Counts)
	}

	// 自定义窗口更短时，旧 waiting 也应被过滤；无时间的 waiting 仍保留。
	custom, err := Build(ctx, db, Options{Now: now, RecentWindow: 30 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if len(custom.Sessions) != 2 {
		t.Fatalf("30 分钟窗口会话数=%d want=2: %+v", len(custom.Sessions), custom.Sessions)
	}
	for _, ss := range custom.Sessions {
		if ss.SessionID == "recent-waiting" {
			t.Fatalf("30 分钟窗口不应包含 1 小时前的 waiting: %+v", custom.Sessions)
		}
	}
}

func TestBuildToleratesEventFileErrors(t *testing.T) {
	ctx := context.Background()
	db := aggregateTestDB(t)
	now := testTime()
	s := &model.Session{
		AgentID: model.AgentClaudeCode, AgentInstanceID: "i", SessionID: "s",
		LastActivityAt: &now, Activity: model.ActivityInactive,
		IndexedAt: now, UpdatedAt: now,
	}
	if err := db.UpsertSession(ctx, s); err != nil {
		t.Fatal(err)
	}
	// 目录不能作为事件文件读取：应产生警告但保留会话状态。
	dir := t.TempDir()
	snap, err := Build(ctx, db, Options{Now: now, EventsPath: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Sessions) != 1 || len(snap.Warnings) == 0 {
		t.Fatalf("sessions=%d warnings=%v", len(snap.Sessions), snap.Warnings)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatal(err)
	}
}
