package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/config"
	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
)

// mkListSession 构造带子会话标记、可指定结束时间的测试会话。
func mkListSession(id string, sub bool, endedAt time.Time) *model.Session {
	ts := endedAt
	return &model.Session{
		AgentID:          model.AgentOpenCode,
		AgentInstanceID:  "oc-inst",
		SessionID:        id,
		FirstQuestion:    "list test " + id,
		WorkingDirectory: "/tmp",
		StartedAt:        &ts,
		EndedAt:          &ts,
		LastActivityAt:   &ts,
		Activity:         model.ActivityInactive,
		IsSubagent:       sub,
		IndexedAt:        ts,
		UpdatedAt:        ts,
	}
}

// newListLoadDB 创建写入指定会话的临时索引库（List 无关键词不查 FTS，无需 Populate）。
func newListLoadDB(t *testing.T, sessions []*model.Session) *index.DB {
	t.Helper()
	ctx := context.Background()
	db, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	for _, s := range sessions {
		if err := db.UpsertSession(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

// TestLoadTuiSessionsHidesSubagentsByDefault 默认配置（include_subagents=false）
// 下 TUI 列表隐藏子 Agent 会话，与 talea list 结果层过滤行为一致。
func TestLoadTuiSessionsHidesSubagentsByDefault(t *testing.T) {
	if config.Default().General.IncludeSubagents {
		t.Fatal("配置默认值必须为隐藏子会话（include_subagents=false）")
	}
	db := newListLoadDB(t, []*model.Session{
		mkListSession("main-1", false, time.Now()),
		mkListSession("sub-1", true, time.Now()),
	})
	a := &app.App{Registry: adapters.NewRegistry(), Config: config.Default()}

	sessions, _, err := loadTuiSessions(context.Background(), a, db, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].SessionID != "main-1" || sessions[0].IsSubagent {
		t.Fatalf("默认应仅含主会话, got %d 条: %v", len(sessions), sessionIDs(sessions))
	}
}

// TestLoadTuiSessionsIncludesSubagentsWhenConfigured include_subagents=true 时显示全部会话。
func TestLoadTuiSessionsIncludesSubagentsWhenConfigured(t *testing.T) {
	cfg := config.Default()
	cfg.General.IncludeSubagents = true
	db := newListLoadDB(t, []*model.Session{
		mkListSession("main-1", false, time.Now()),
		mkListSession("sub-1", true, time.Now()),
	})
	a := &app.App{Registry: adapters.NewRegistry(), Config: cfg}

	sessions, _, err := loadTuiSessions(context.Background(), a, db, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("include_subagents=true 应含全部会话, got %d 条: %v", len(sessions), sessionIDs(sessions))
	}
	byID := map[string]bool{}
	for _, s := range sessions {
		byID[s.SessionID] = s.IsSubagent
	}
	if _, ok := byID["main-1"]; !ok {
		t.Fatalf("应包含主会话, got %v", sessionIDs(sessions))
	}
	if sub, ok := byID["sub-1"]; !ok || !sub {
		t.Fatalf("include_subagents=true 应保留子会话, got %v", byID)
	}
}

// TestLoadTuiSessionsNoTruncation 移除硬编码 500 上限后，超过 500 条的夹具
// （镜像真实索引 548 条非子会话）全量可见，最早的会话不被静默截断。
func TestLoadTuiSessionsNoTruncation(t *testing.T) {
	const total = 548
	base := time.Now()
	sessions := make([]*model.Session, 0, total)
	for i := 0; i < total; i++ {
		// 结束时间递减：最后一条为最早的会话，位于倒序列表末尾
		sessions = append(sessions, mkListSession(
			fmt.Sprintf("ses-%04d", i), false, base.Add(-time.Duration(i)*time.Minute)))
	}
	db := newListLoadDB(t, sessions)
	a := &app.App{Registry: adapters.NewRegistry(), Config: config.Default()}

	got, _, err := loadTuiSessions(context.Background(), a, db, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != total {
		t.Fatalf("全量加载被截断: got %d 条, want %d 条", len(got), total)
	}
	if got[0].SessionID != "ses-0000" {
		t.Fatalf("列表头部应为最新会话 ses-0000, got %s", got[0].SessionID)
	}
	if oldest := got[len(got)-1]; oldest.SessionID != fmt.Sprintf("ses-%04d", total-1) {
		t.Fatalf("最早的会话应可见, 列表末尾 got %s, want ses-%04d", oldest.SessionID, total-1)
	}
}

// sessionIDs 返回会话 ID 列表（测试失败信息用）。
func sessionIDs(sessions []*model.Session) []string {
	out := make([]string, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, s.SessionID)
	}
	return out
}
