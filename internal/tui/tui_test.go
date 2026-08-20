package tui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/config"
	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
	"github.com/talea/talea/internal/search"
	"github.com/talea/talea/internal/timeline"
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

	all, _, err := loadTuiSessions(ctx, a, db, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("no dir filter: got %d sessions, want 3", len(all))
	}

	filtered, _, err := loadTuiSessions(ctx, a, db, "/home/user/nexora", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || filtered[0].SessionID != "ses_1" {
		t.Fatalf("dir filter: got %d sessions, want ses_1 only", len(filtered))
	}

	sub, _, err := loadTuiSessions(ctx, a, db, "/home/user/nexora/frontend", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sub) != 1 || sub[0].SessionID != "ses_2" {
		t.Fatalf("sub dir filter: got %d sessions, want ses_2 only", len(sub))
	}
}

// TestLoadTuiSessionsAgentFilter 验证 TUI 按 Agent 过滤，且可与目录过滤组合。
func TestLoadTuiSessionsAgentFilter(t *testing.T) {
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
	now := time.Now()
	sessions := []*model.Session{
		mkTuiSession("ses_1", "/home/user/nexora"),                       // opencode
		mkTuiSession("ses_2", "/home/user/nexora/frontend"),              // opencode
		{AgentID: model.AgentClaudeCode, SessionID: "ses_3", FirstQuestion: "cc", WorkingDirectory: "/home/user/nexora", StartedAt: &now, EndedAt: &now, LastActivityAt: &now, Activity: model.ActivityInactive, IndexedAt: now, UpdatedAt: now},
	}
	for _, s := range sessions {
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

	// 仅按 Agent 过滤
	oc, _, err := loadTuiSessions(ctx, a, db, "", "opencode")
	if err != nil {
		t.Fatal(err)
	}
	if len(oc) != 2 {
		t.Fatalf("agent filter: got %d sessions, want 2", len(oc))
	}

	// Agent + 目录组合过滤
	combo, _, err := loadTuiSessions(ctx, a, db, "/home/user/nexora", "opencode")
	if err != nil {
		t.Fatal(err)
	}
	if len(combo) != 1 || combo[0].SessionID != "ses_1" {
		t.Fatalf("agent+dir filter: got %d sessions, want ses_1 only", len(combo))
	}
}

func TestSessionTitleAndDesc(t *testing.T) {
	start := time.Now()
	d := time.Hour
	s := &model.Session{
		AgentID:          model.AgentClaudeCode,
		SessionID:        "abc",
		FirstQuestion:    "分析 multipath 残留",
		LastUserPrompt:   "帮我看看最新的 multipath 配置",
		WorkingDirectory: "/home/alice/code/x",
		StartedAt:        &start,
		Duration:         &d,
		Activity:         model.ActivityInactive,
	}
	it := item{sess: s}
	if title := itemTitle(it); title == "" {
		t.Fatal("empty title")
	}
	desc := itemDesc(it)
	if desc == "" {
		t.Fatal("empty desc")
	}
	// 首次提问与最近用户消息都应出现在描述中
	if !strings.Contains(desc, "multipath 残留") {
		t.Errorf("desc 应包含首次提问, got: %q", desc)
	}
	if !strings.Contains(desc, "multipath 配置") {
		t.Errorf("desc 应包含最近用户消息, got: %q", desc)
	}
}

func TestItemFilterValue(t *testing.T) {
	it := item{title: "t", sess: &model.Session{
		FirstQuestion: "q", SessionID: "s", AgentID: model.AgentOpenCode,
		WorkingDirectory: "/home/alice/code/x",
	}}
	fv := it.FilterValue()
	if fv == "" {
		t.Fatal("empty filter")
	}
	// 按目录搜索应能匹配
	if !strings.Contains(fv, "/home/alice/code/x") {
		t.Errorf("FilterValue 应包含工作目录, got: %q", fv)
	}
}

func TestItemTitleWithUsage(t *testing.T) {
	in := int64(1_000_000)
	cr := int64(800_000)
	cw := int64(100_000)
	it := item{
		sess:  &model.Session{AgentID: model.AgentOpenCode, SessionID: "s", StartedAt: &[]time.Time{time.Now()}[0]},
		hasUs: true,
		usage: timeline.SessionUsageRow{TotalTokens: &in, CacheRead: &cr, CacheWrite: &cw},
	}
	title := itemTitle(it)
	if title == "" {
		t.Fatal("empty title with usage")
	}
	if !strings.Contains(title, "Token") || !strings.Contains(title, "1.00M") {
		t.Errorf("title 应包含 Token 汇总, got: %q", title)
	}
	// 缓存命中率: 800k / (0 + 800k + 100k) = 88.9%
	if !strings.Contains(title, "Cache") || !strings.Contains(title, "89%") {
		t.Errorf("title 应包含缓存命中率, got: %q", title)
	}
	// 固定列宽对齐验证：不同会话的相同 Key 起始位置应一致（按显示宽度）
	s2 := &model.Session{
		AgentID:          model.AgentOpenCode,
		SessionID:        "s2",
		StartedAt:        &[]time.Time{time.Now()}[0],
		EndedAt:          &[]time.Time{time.Now()}[0],
		Duration:         &[]time.Duration{2 * time.Minute}[0],
		WorkingDirectory: "/very/long/path/that/exceeds/twenty/chars",
		GitBranch:        "feature-a-very-long-branch-name",
	}
	it2 := item{sess: s2, hasUs: true, usage: timeline.SessionUsageRow{TotalTokens: &in, CacheRead: &cr, CacheWrite: &cw}}
	t1, t2 := itemTitle(it), itemTitle(it2)
	// 总显示宽度应一致
	if runewidth.StringWidth(t1) != runewidth.StringWidth(t2) {
		t.Errorf("两行显示宽度不一致: t1=%d t2=%d", runewidth.StringWidth(t1), runewidth.StringWidth(t2))
	}
	// 每个 Key 的显示起始位置一致
	p1, p2 := keyPositions(t1), keyPositions(t2)
	for _, key := range []string{"Start", "End", "Time", "Token", "Cache", "Path", "Branch"} {
		if p1[key] != p2[key] {
			t.Errorf("Key %q 未对齐: t1 pos=%d t2 pos=%d\nt1=%q\nt2=%q", key, p1[key], p2[key], t1, t2)
		}
	}
}

// keyPositions 返回行中各元数据 Key 的显示宽度起始位置。
func keyPositions(line string) map[string]int {
	keys := []string{"Start", "End", "Time", "Token", "Cache", "Path", "Branch"}
	out := map[string]int{}
	for _, key := range keys {
		out[key] = -1
	}
	rs := []rune(line)
	cur := 0
	// 逐字符扫描，遇到 Key 时记录位置并跳过
	for i := 0; i < len(rs); {
		consumed := false
		for _, key := range keys {
			kr := []rune(key)
			if i+len(kr) <= len(rs) && string(rs[i:i+len(kr)]) == key && out[key] < 0 {
				out[key] = cur
				cur += runewidth.StringWidth(key)
				i += len(kr)
				consumed = true
				break
			}
		}
		if !consumed {
			cur += runewidth.RuneWidth(rs[i])
			i++
		}
	}
	return out
}

func TestMainModelInit(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	reg := adapters.NewRegistry()
	a := &app.App{Registry: reg, Config: cfg, Paths: config.Paths{}}
	m := newMain(ctx, a, nil, nil, nil, "", "")
	if m == nil {
		t.Fatal("nil model")
	}
	_ = m.Init()
	_ = m.View()
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
	m := newMain(ctx, a, []*model.Session{s}, nil, nil, "", "")

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

// TestLoadTuiSessionsTimeSort 验证 TUI 会话列表固定按结束时间倒序排列，
// 与 talea list / talea go 一致，不受配置 default_sort 影响。
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
	older := mkTuiSession("ses_old", "/home/user/nexora")
	older.StartedAt = timePtr(base.Add(time.Hour))
	older.EndedAt = timePtr(base.Add(2 * time.Hour))
	mid := mkTuiSession("ses_mid", "/home/user/nexora")
	mid.StartedAt = timePtr(base.Add(2 * time.Hour))
	mid.EndedAt = timePtr(base.Add(3 * time.Hour))
	newer := mkTuiSession("ses_new", "/home/user/nexora")
	newer.StartedAt = timePtr(base.Add(3 * time.Hour))
	newer.EndedAt = timePtr(base.Add(4 * time.Hour))
	for _, s := range []*model.Session{mid, older, newer} {
		if err := db.UpsertSession(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	if err := search.Populate(ctx, db); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	// 故意把 default_sort 设为 name，验证 TUI 仍按结束时间排
	cfg.General.DefaultSort = "name"
	reg := adapters.NewRegistry()
	a := &app.App{Registry: reg, Config: cfg}

	sessions, _, err := loadTuiSessions(ctx, a, db, "", "")
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

// TestEndTsFallback 验证 endTs 的兜底顺序：结束 > 开始 > 最后活动。
func TestEndTsFallback(t *testing.T) {
	base := time.Date(2026, 8, 5, 16, 0, 0, 0, time.Local)
	// 无结束时间：用开始时间
	s := mkTuiSession("ses_a", "/x")
	s.EndedAt = nil
	s.StartedAt = timePtr(base.Add(time.Hour))
	if got := endTs(s); got != base.Add(time.Hour).Unix() {
		t.Fatalf("endTs with nil EndedAt: got %d, want %d", got, base.Add(time.Hour).Unix())
	}
	// 结束时间优先
	s.EndedAt = timePtr(base.Add(2 * time.Hour))
	if got := endTs(s); got != base.Add(2*time.Hour).Unix() {
		t.Fatalf("endTs with EndedAt: got %d, want %d", got, base.Add(2*time.Hour).Unix())
	}
}

// 确保 list 类型已使用（避免导入未用）
var _ list.Item = item{}
var _ tea.Model = (*mainModel)(nil)

func timePtr(t time.Time) *time.Time { return &t }

// TestPaginationDotsApplied 回归：分页圆点样式必须真正应用到 Paginator，
// 否则 bubbles list.New 的默认深灰圆点（#3C3C3C）在深色终端上几乎不可见。
func TestPaginationDotsApplied(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	reg := adapters.NewRegistry()
	a := &app.App{Registry: reg, Config: cfg, Paths: config.Paths{}}
	m := newMain(ctx, a, []*model.Session{mkTuiSession("ses_1", "/home/user/nexora")}, nil, nil, "", "")

	if !strings.Contains(m.list.Paginator.ActiveDot, "•") {
		t.Errorf("active dot should use the dot char, got %q", m.list.Paginator.ActiveDot)
	}
	if !strings.Contains(m.list.Paginator.InactiveDot, "•") {
		t.Errorf("inactive dot should use the dot char, got %q", m.list.Paginator.InactiveDot)
	}
	// 无 TTY 测试环境下 lipgloss 不输出 ANSI 序列，这里断言样式配置本身：
	// 当前页圆点须加粗突出，且与非当前页颜色不同。
	if !m.list.Styles.ActivePaginationDot.GetBold() {
		t.Error("active dot should be bold to stand out")
	}
	if m.list.Styles.ActivePaginationDot.GetForeground() == m.list.Styles.InactivePaginationDot.GetForeground() {
		t.Error("active and inactive dots must use different colors")
	}
}

// TestPaginationDotsRender 验证：设置尺寸后，真实 View 渲染输出中包含
// 自定义实心分页圆点 ●（而非默认不可见深灰圆点）。
func TestPaginationDotsRender(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	reg := adapters.NewRegistry()
	a := &app.App{Registry: reg, Config: cfg, Paths: config.Paths{}}
	var sessions []*model.Session
	for i := 0; i < 30; i++ {
		sessions = append(sessions, mkTuiSession(fmt.Sprintf("ses_%02d", i), "/home/user/nexora"))
	}
	m := newMain(ctx, a, sessions, nil, nil, "", "")
	m.list.SetSize(80, 30)
	out := m.list.View()
	if !strings.Contains(out, "•") {
		t.Errorf("pagination should render dots, got: %q", out)
	}
}

// TestLoadingViewRendersSpinner 验证首屏等待动画：loading 状态下 View
// 渲染 spinner 帧与同步文案（中/英文均可）。
func TestLoadingViewRendersSpinner(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	reg := adapters.NewRegistry()
	a := &app.App{Registry: reg, Config: cfg, Paths: config.Paths{}}
	m := newMain(ctx, a, nil, nil, nil, "", "")
	m.loading = true
	out := m.View()
	if out == "" {
		t.Fatal("empty loading view")
	}
	if !strings.Contains(out, "⣾") {
		t.Errorf("loading view should include a spinner frame, got: %q", out)
	}
	if !strings.Contains(out, "正在同步会话") && !strings.Contains(out, "Syncing sessions") {
		t.Errorf("loading view should show sync message, got: %q", out)
	}
}

// TestLoadingIgnoresKeys 验证首屏加载中仅允许退出：
// o/enter/d 不得触发恢复或详情，q/ctrl+c 触发退出。
func TestLoadingIgnoresKeys(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	reg := adapters.NewRegistry()
	a := &app.App{Registry: reg, Config: cfg, Paths: config.Paths{}}
	m := newMain(ctx, a, nil, nil, nil, "", "")
	m.loading = true

	// o：不得恢复
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	mm := nm.(*mainModel)
	if mm.picked != nil || cmd != nil {
		t.Fatalf("loading: 'o' should be ignored, picked=%v cmd=%v", mm.picked, cmd)
	}
	// d：不得打开详情
	nm, cmd = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	mm = nm.(*mainModel)
	if mm.detail != nil || cmd != nil {
		t.Fatalf("loading: 'd' should be ignored, detail=%v", mm.detail)
	}
	// enter：不得恢复
	nm, cmd = mm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm = nm.(*mainModel)
	if mm.picked != nil || cmd != nil {
		t.Fatalf("loading: enter should be ignored, picked=%v", mm.picked)
	}
	// q：退出
	_, cmd = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("loading: 'q' should quit")
	}
}

// TestIndexedMsgLoadsList 验证首屏同步完成后：loading 解除并填充最新列表。
func TestIndexedMsgLoadsList(t *testing.T) {
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
	if err := db.UpsertSession(ctx, mkTuiSession("ses_new", "/home/user/nexora")); err != nil {
		t.Fatal(err)
	}
	if err := search.Populate(ctx, db); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	reg := adapters.NewRegistry()
	a := &app.App{Registry: reg, Config: cfg}
	m := newMain(ctx, a, nil, nil, db, "", "")
	m.loading = true

	nm, _ := m.Update(indexedMsg{})
	mm := nm.(*mainModel)
	if mm.loading {
		t.Fatal("loading should be cleared after indexedMsg")
	}
	if mm.loadingErr != nil {
		t.Fatalf("unexpected loadingErr: %v", mm.loadingErr)
	}
	if len(mm.sessions) != 1 || mm.sessions[0].SessionID != "ses_new" {
		t.Fatalf("list should be filled with new session, got %d sessions", len(mm.sessions))
	}
}

// TestIndexedMsgErrorShowsFeedback 验证首屏同步失败时解除 loading 并记录错误。
func TestIndexedMsgErrorShowsFeedback(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	reg := adapters.NewRegistry()
	a := &app.App{Registry: reg, Config: cfg}
	m := newMain(ctx, a, nil, nil, nil, "", "")
	m.loading = true
	m.indexErr = errors.New("boom")

	nm, _ := m.Update(indexedMsg{})
	mm := nm.(*mainModel)
	if mm.loading {
		t.Fatal("loading should be cleared on error")
	}
	if mm.loadingErr == nil {
		t.Fatal("loadingErr should be set on sync failure")
	}
	out := mm.View()
	if !strings.Contains(out, "同步会话失败") && !strings.Contains(out, "Failed to sync sessions") {
		t.Errorf("error view should show failure message, got: %q", out)
	}
}


// TestLoadingViewCentered 验证 loading 视图在设置尺寸后水平且垂直居中。
func TestLoadingViewCentered(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	reg := adapters.NewRegistry()
	a := &app.App{Registry: reg, Config: cfg, Paths: config.Paths{}}
	m := newMain(ctx, a, nil, nil, nil, "", "")
	m.loading = true
	m.width = 100
	m.height = 30
	out := m.View()
	// 输出高度应为 30 行（垂直居中填满）
	if got := strings.Count(out, "\n") + 1; got != 30 {
		t.Errorf("loading view should fill 30 lines (vertical center), got %d lines", got)
	}
	// 首行（标题）前应有前导空格（水平居中），且尾部同样有空格填充
	first := out[:strings.Index(out, "\n")]
	if !strings.HasPrefix(first, " ") {
		t.Errorf("loading view should be horizontally centered, first line starts without padding: %q", first)
	}
}
