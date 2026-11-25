package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
	"github.com/talea/talea/internal/status"
)

// TestDashboardFromSyntheticIndex 用临时索引库和合成会话跑通状态聚合与看板渲染。
func TestDashboardFromSyntheticIndex(t *testing.T) {
	ctx := context.Background()
	db, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	at := now.Add(-time.Minute)
	for _, s := range []*model.Session{
		{
			AgentID: model.AgentClaudeCode, AgentInstanceID: "i", SessionID: "ses_wait",
			FirstQuestion: "waiting task", ProjectName: "proj", WorkingDirectory: "/w",
			StartedAt: &now, EndedAt: &at, LastActivityAt: &at,
			Activity: model.ActivityInactive, IndexedAt: now, UpdatedAt: now,
			TurnState: model.TurnStateWorking, TurnStateAt: &at,
			TurnStateSource: model.TurnStateSourceNative,
		},
		{
			AgentID: model.AgentOpenCode, AgentInstanceID: "i", SessionID: "ses_done",
			FirstQuestion: "done task", ProjectName: "proj", WorkingDirectory: "/w",
			StartedAt: &now, EndedAt: &now, LastActivityAt: &now,
			Activity: model.ActivityInactive, IndexedAt: now, UpdatedAt: now,
			TurnState: model.TurnStateCompleted, TurnStateAt: &now,
			TurnStateSource: model.TurnStateSourceNative,
		},
	} {
		if err := db.UpsertSession(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	eventsPath := filepath.Join(t.TempDir(), status.EventFileName)
	if err := status.AppendEvent(eventsPath, status.Event{
		AgentID: string(model.AgentClaudeCode), SessionID: "ses_wait",
		State: model.TurnStateWaitingInput, At: at.Add(30 * time.Second),
		Detail: "permission_prompt",
	}); err != nil {
		t.Fatal(err)
	}
	snap, err := status.Build(ctx, db, status.Options{EventsPath: eventsPath})
	if err != nil {
		t.Fatal(err)
	}
	if snap.Counts.Total != 2 || snap.Counts.Waiting != 1 || snap.Counts.Completed != 1 {
		t.Fatalf("counts=%+v", snap.Counts)
	}
	m := newDashboard(ctx, nil, db, eventsPath, snap, nil)
	m.width = 100
	m.height = 30
	out := ansi.Strip(m.View())
	for _, want := range []string{"Needs you 1", "Completed (turn ended) 1", "ses_wait", "ses_done"} {
		if !strings.Contains(out, want) {
			t.Fatalf("看板缺少 %q:\n%s", want, out)
		}
	}
}
