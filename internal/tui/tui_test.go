package tui

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/config"
	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
	"github.com/talea/talea/internal/search"
)

// mkTuiSession 构造测试会话。
func mkTuiSession(id, cwd string) *model.Session {
	now := time.Now()
	return &model.Session{
		AgentID:          model.AgentOpenCode,
		SessionID:        id,
		FirstQuestion:    "tui test " + id,
		WorkingDirectory: cwd,
		StartedAt:        &now,
		EndedAt:          &now,
		LastActivityAt:   &now,
		Activity:         model.ActivityInactive,
		IndexedAt:        now,
		UpdatedAt:        now,
	}
}

func TestLoadTuiSessionsDirFilter(t *testing.T) {
	ctx := context.Background()
	db, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := search.Ensure(ctx, db); err != nil {
		t.Fatal(err)
	}
	for _, s := range []*model.Session{
		mkTuiSession("ses_1", "/home/user/nexora"),
		mkTuiSession("ses_2", "/home/user/nexora/frontend"),
		mkTuiSession("ses_3", "/home/user/other"),
	} {
		if err := db.UpsertSession(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	if err := search.Populate(ctx, db); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	reg := adapters.NewRegistry()
	a := &app.App{Registry: reg, Config: cfg}

	all, err := loadTuiSessions(ctx, a, db, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("no dir filter: got %d sessions, want 3", len(all))
	}

	filtered, err := loadTuiSessions(ctx, a, db, "/home/user/nexora")
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 2 {
		t.Fatalf("dir filter: got %d sessions, want 2", len(filtered))
	}

	sub, err := loadTuiSessions(ctx, a, db, "/home/user/nexora/frontend")
	if err != nil {
		t.Fatal(err)
	}
	if len(sub) != 1 || sub[0].SessionID != "ses_2" {
		t.Fatalf("sub dir filter: got %d sessions, want ses_2 only", len(sub))
	}
}

func TestSessionTitleAndDesc(t *testing.T) {
	start := time.Now()
	d := time.Hour
	s := &model.Session{
		AgentID:          model.AgentClaudeCode,
		SessionID:        "abc",
		FirstQuestion:    "分析 multipath 残留",
		WorkingDirectory: "/home/alice/code/x",
		StartedAt:        &start,
		Duration:         &d,
		Activity:         model.ActivityInactive,
	}
	if title := sessionTitle(s); title == "" {
		t.Fatal("empty title")
	}
	if desc := sessionDesc(s); desc == "" {
		t.Fatal("empty desc")
	}
}

func TestMainModelInit(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	reg := adapters.NewRegistry()
	a := &app.App{Registry: reg, Config: cfg, Paths: config.Paths{}}
	m := newMain(ctx, a, nil, nil, "")
	if m == nil {
		t.Fatal("nil model")
	}
	_ = m.Init()
	_ = m.View()
}

func TestItemFilterValue(t *testing.T) {
	it := item{title: "t", sess: &model.Session{
		FirstQuestion: "q", SessionID: "s", AgentID: model.AgentOpenCode,
	}}
	if it.FilterValue() == "" {
		t.Fatal("empty filter")
	}
}

func TestDetailRenderEmptyDB(t *testing.T) {
	d := &detailModel{
		sess: &model.Session{SessionID: "s", AgentID: model.AgentClaudeCode, FirstQuestion: "q"},
	}
	// 无 db/app 时聚合 render 不崩溃
	if d.render() == "" {
		t.Fatal("empty render")
	}
}

// 确保 list 类型已使用（避免导入未用）
var _ list.Item = item{}
var _ tea.Model = (*mainModel)(nil)
