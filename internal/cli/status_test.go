package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/talea/talea/internal/config"
	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
	"github.com/talea/talea/internal/status"
)

func isolateTaleaEnv(t *testing.T) config.Paths {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	t.Setenv("PATH", "")
	paths := config.ResolvePaths()
	if err := os.MkdirAll(paths.DataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	return paths
}

func seedStatusDB(t *testing.T, paths config.Paths) {
	t.Helper()
	ctx := context.Background()
	db, err := index.Open(paths.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	at := now.Add(-time.Minute)
	sessions := []*model.Session{
		{
			AgentID: model.AgentClaudeCode, AgentInstanceID: "inst",
			SessionID: "ses_wait", FirstQuestion: "waiting task",
			WorkingDirectory: "/work/a", ProjectName: "proj-a",
			StartedAt: &now, EndedAt: &at, LastActivityAt: &at,
			Activity: model.ActivityInactive, IndexedAt: now, UpdatedAt: now,
			TurnState: model.TurnStateWaitingInput, TurnStateAt: &at,
			TurnStateSource: model.TurnStateSourceNative, TurnStateDetail: "permission",
		},
		{
			AgentID: model.AgentOpenCode, AgentInstanceID: "inst",
			SessionID: "ses_done", FirstQuestion: "done task",
			WorkingDirectory: "/work/b", ProjectName: "proj-b",
			StartedAt: &now, EndedAt: &now, LastActivityAt: &now,
			Activity: model.ActivityInactive, IndexedAt: now, UpdatedAt: now,
			TurnState: model.TurnStateCompleted, TurnStateAt: &now,
			TurnStateSource: model.TurnStateSourceNative,
		},
	}
	for _, s := range sessions {
		if err := db.UpsertSession(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
}

func runStatus(t *testing.T, args ...string) string {
	t.Helper()
	cmd := newStatusCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("status %v: %v\n%s", args, err, out.String())
	}
	return out.String()
}

func TestStatusJSON(t *testing.T) {
	paths := isolateTaleaEnv(t)
	seedStatusDB(t, paths)
	out := runStatus(t, "--json")

	var snap status.Snapshot
	if err := json.Unmarshal([]byte(out), &snap); err != nil {
		t.Fatalf("JSON 解析失败: %v\n%s", err, out)
	}
	if snap.GeneratedAt.IsZero() || snap.Counts.Total != 2 || len(snap.Sessions) != 2 {
		t.Fatalf("snapshot=%+v", snap)
	}
	if snap.Counts.NeedsAttention != 1 || snap.Counts.Waiting != 1 || snap.Counts.Completed != 1 {
		t.Fatalf("counts=%+v", snap.Counts)
	}
	foundWait, foundDone := false, false
	for _, ss := range snap.Sessions {
		if ss.AgentID == "" || ss.SessionID == "" || ss.WorkingDirectory == "" ||
			ss.LastActivityAt == nil || ss.State == "" || ss.Source == "" || ss.Reason == "" {
			t.Fatalf("会话字段不完整: %+v", ss)
		}
		switch ss.SessionID {
		case "ses_wait":
			foundWait = ss.State == status.StateWaiting && ss.NeedsAttention
		case "ses_done":
			foundDone = ss.State == status.StateCompleted && ss.Label == "Turn ended"
		}
	}
	if !foundWait || !foundDone {
		t.Fatalf("状态不符合预期: %+v", snap.Sessions)
	}
}

func TestStatusShortIsSingleLine(t *testing.T) {
	paths := isolateTaleaEnv(t)
	seedStatusDB(t, paths)
	out := strings.TrimSpace(runStatus(t, "--short"))
	if out == "" || strings.ContainsAny(out, "\r\n") {
		t.Fatalf("short 输出不是单行: %q", out)
	}
	if !strings.Contains(out, "1") {
		t.Fatalf("short 缺少计数: %q", out)
	}
}

func TestStatusTextListsSessions(t *testing.T) {
	paths := isolateTaleaEnv(t)
	seedStatusDB(t, paths)
	out := runStatus(t)
	if !strings.Contains(out, "ses_wait") || !strings.Contains(out, "ses_done") {
		t.Fatalf("text 输出缺少会话: %s", out)
	}
}
