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

// TestFilterModeIgnoresFunctionKeys 回归：过滤模式下按 o/d/enter 等
// 功能键不得触发恢复/详情，按键应交给 list 作为过滤输入。
func TestFilterModeIgnoresFunctionKeys(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	reg := adapters.NewRegistry()
	a := &app.App{Registry: reg, Config: cfg, Paths: config.Paths{}}
	s := mkTuiSession("ses_1", "/home/user/nexora")
	m := newMain(ctx, a, []*model.Session{s}, nil, "")

	// 进入过滤模式：list 收到 "/" 后状态变为 Filtering
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	mm, ok := nm.(*mainModel)
	if !ok {
		t.Fatal("model type changed")
	}
	if mm.list.FilterState() != list.Filtering {
		t.Fatalf("expected Filtering state, got %v", mm.list.FilterState())
	}
	_ = mm // 初始 Filtering 状态引用保留，后续通过 mm2.. 演化

	// 过滤模式下按 "o"：不得触发恢复（picked 必须保持 nil）
	nm2, _ := mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	mm2, ok := nm2.(*mainModel)
	if !ok {
		t.Fatal("model type changed")
	}
	if mm2.picked != nil {
		t.Fatalf("filter mode: 'o' triggered resume, picked=%v", mm2.picked.SessionID)
	}
	if got := mm2.list.FilterValue(); got != "o" {
		t.Fatalf("filter mode: 'o' not entered filter input, got %q", got)
	}
	if mm2.list.FilterState() == list.Unfiltered {
		t.Fatal("expected non-Unfiltered state after filter input")
	}

	// 过滤模式下按 "d"：不得打开详情
	nm3, _ := mm2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	mm3, ok := nm3.(*mainModel)
	if !ok {
		t.Fatal("model type changed")
	}
	if mm3.detail != nil {
		t.Fatal("filter mode: 'd' opened detail")
	}

	// enter 应用过滤：状态从 Filtering 变为 FilterApplied，不得进入会话
	nmEnter, _ := mm3.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mmEnter, ok := nmEnter.(*mainModel)
	if !ok {
		t.Fatal("model type changed")
	}
	if mmEnter.list.FilterState() != list.FilterApplied {
		t.Fatalf("expected FilterApplied after enter, got %v", mmEnter.list.FilterState())
	}
	if mmEnter.picked != nil {
		t.Fatal("filter enter should not trigger resume")
	}

	// 过滤已应用（FilterApplied）后按 "o"：应进入会话
	nm6, _ := mmEnter.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	mm6, ok := nm6.(*mainModel)
	if !ok {
		t.Fatal("model type changed")
	}
	if mm6.picked == nil || mm6.picked.SessionID != "ses_1" {
		t.Fatal("FilterApplied mode: 'o' should trigger resume")
	}

	// FilterApplied 状态下 esc 退出过滤模式
	nm4, _ := mmEnter.Update(tea.KeyMsg{Type: tea.KeyEsc})
	mm4, ok := nm4.(*mainModel)
	if !ok {
		t.Fatal("model type changed")
	}
	if mm4.list.FilterState() != list.Unfiltered {
		t.Fatalf("expected Unfiltered after esc, got %v", mm4.list.FilterState())
	}

	// 退出过滤后按 "o"：应触发恢复
	nm5, _ := mm4.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	mm5, ok := nm5.(*mainModel)
	if !ok {
		t.Fatal("model type changed")
	}
	if mm5.picked == nil || mm5.picked.SessionID != "ses_1" {
		t.Fatal("unfiltered mode: 'o' should trigger resume")
	}
}

// TestLoadTuiSessionsTimeSort 验证 TUI 会话列表固定按开始时间倒序排列。
func TestLoadTuiSessionsTimeSort(t *testing.T) {
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
	base := time.Date(2026, 8, 5, 16, 0, 0, 0, time.Local)
	newer := mkTuiSession("ses_new", "/home/user/nexora")
	newer.StartedAt = timePtr(base.Add(3 * time.Hour))
	mid := mkTuiSession("ses_mid", "/home/user/nexora")
	mid.StartedAt = timePtr(base.Add(2 * time.Hour))
	older := mkTuiSession("ses_old", "/home/user/nexora")
	older.StartedAt = timePtr(base.Add(time.Hour))
	for _, s := range []*model.Session{mid, older, newer} {
		if err := db.UpsertSession(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	if err := search.Populate(ctx, db); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	// 故意把 default_sort 设为 name，验证 TUI 仍按时间排
	cfg.General.DefaultSort = "name"
	reg := adapters.NewRegistry()
	a := &app.App{Registry: reg, Config: cfg}

	sessions, err := loadTuiSessions(ctx, a, db, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 3 {
		t.Fatalf("got %d sessions, want 3", len(sessions))
	}
	if sessions[0].SessionID != "ses_new" || sessions[1].SessionID != "ses_mid" || sessions[2].SessionID != "ses_old" {
		t.Fatalf("unexpected order: %s, %s, %s", sessions[0].SessionID, sessions[1].SessionID, sessions[2].SessionID)
	}
}

func timePtr(t time.Time) *time.Time { return &t }

// 确保 list 类型已使用（避免导入未用）
var _ list.Item = item{}
var _ tea.Model = (*mainModel)(nil)
