package tui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/config"
	"github.com/talea/talea/internal/i18n"
	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
	"github.com/talea/talea/internal/search"
	"github.com/talea/talea/internal/syncer"
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
		mkTuiSession("ses_1", "/home/user/nexora"),          // opencode
		mkTuiSession("ses_2", "/home/user/nexora/frontend"), // opencode
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

func TestSessionDescriptionUsesViewportWidth(t *testing.T) {
	longQuestion := strings.Repeat("question-", 20)
	s := mkTuiSession("wide", "/home/user/nexora")
	s.FirstQuestion = longQuestion
	s.LastUserPrompt = strings.Repeat("最近消息-", 20)
	m := newMain(context.Background(), &app.App{Registry: adapters.NewRegistry(), Config: config.Default()}, []*model.Session{s}, nil, nil, "", "")

	nm, _ := m.Update(tea.WindowSizeMsg{Width: 240, Height: 16})
	m = nm.(*mainModel)
	wide := m.list.View()
	if !strings.Contains(wide, longQuestion) {
		t.Fatalf("wide list should keep more than 100 characters of question, got %q", wide)
	}

	nm, _ = m.Update(tea.WindowSizeMsg{Width: 40, Height: 16})
	m = nm.(*mainModel)
	narrow := m.list.View()
	assertLinesFit(t, narrow, 40)
	if strings.Contains(narrow, longQuestion) {
		t.Fatal("narrow list should clip the long question")
	}

	s.FirstQuestion = strings.Repeat("中文问题", 30)
	m.list.SetItems(itemsOf([]*model.Session{s}, nil))
	chinese := m.list.View()
	assertLinesFit(t, chinese, 40)
}

func TestSessionListResizePreservesSelection(t *testing.T) {
	first := mkTuiSession("first", "/home/user/one")
	second := mkTuiSession("second", "/home/user/two")
	m := newMain(context.Background(), &app.App{Registry: adapters.NewRegistry(), Config: config.Default()}, []*model.Session{first, second}, nil, nil, "", "")
	m.list.SetSize(240, 12)
	m.list.Select(1)

	nm, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 16})
	mm := nm.(*mainModel)
	selected, ok := mm.list.SelectedItem().(item)
	if !ok || selected.sess.SessionID != "second" {
		t.Fatalf("resize changed selection: got %#v", mm.list.SelectedItem())
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

func TestDetailTablesUseViewportWidth(t *testing.T) {
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
	sess := mkTuiSession("session", "/home/user/nexora")
	sess.AgentInstanceID = "instance"
	if err := db.UpsertSession(ctx, sess); err != nil {
		t.Fatal(err)
	}
	longModel := strings.Repeat("model-", 8)
	longPrompt := strings.Repeat("prompt-", 9)
	total := int64(1000)
	events := []*model.UsageTimelineEvent{
		{AgentInstanceID: "instance", SessionID: "session", EventType: model.UsageEventUserMessage,
			Timestamp: &now, Sequence: 1, UserPromptPreview: longPrompt, SourceIdentity: "user-1"},
		{AgentInstanceID: "instance", SessionID: "session", EventType: model.UsageEventRequest,
			Timestamp: &now, Sequence: 2, Model: longModel, TotalTokens: &total, SourceIdentity: "request-1"},
	}
	if _, err := db.UpsertTimelineEvents(ctx, events); err != nil {
		t.Fatal(err)
	}
	child := mkTuiSession("child", "/home/user/nexora")
	child.AgentInstanceID = "instance"
	child.ParentSessionID = sess.SessionID
	child.FirstQuestion = longPrompt
	if err := db.UpsertSession(ctx, child); err != nil {
		t.Fatal(err)
	}

	d := &detailModel{ctx: ctx, db: db, sess: sess, width: 100}
	modelWide := d.renderModel()
	assertLinesFit(t, modelWide, 100)
	if !strings.Contains(modelWide, longModel) {
		t.Fatalf("wide model table should show full model name, got %q", modelWide)
	}
	turnsWide := d.renderTurnsTable()
	assertLinesFit(t, turnsWide, 100)
	if !strings.Contains(turnsWide, longPrompt) {
		t.Fatalf("wide turns table should show full prompt, got %q", turnsWide)
	}
	subagentsWide := d.renderSubagents()
	assertLinesFit(t, subagentsWide, 100)
	if !strings.Contains(subagentsWide, longPrompt) {
		t.Fatalf("wide sub-agent table should show full prompt, got %q", subagentsWide)
	}

	d.width = 40
	assertLinesFit(t, d.renderModel(), 40)
	assertLinesFit(t, d.renderTurnsTable(), 40)
	assertLinesFit(t, d.renderSubagents(), 40)

	m := newMain(ctx, &app.App{Registry: adapters.NewRegistry(), Config: config.Default()}, nil, nil, db, "", "")
	m.detail = d
	d.contentValid = true
	_, _ = m.Update(tea.WindowSizeMsg{Width: 32, Height: 16})
	if d.width != 32 || d.height != 14 || d.contentValid {
		t.Fatalf("detail resize state not updated: width=%d height=%d valid=%v", d.width, d.height, d.contentValid)
	}
	assertLinesFit(t, d.renderModel(), 32)
}

func assertLinesFit(t *testing.T, output string, width int) {
	t.Helper()
	for i, line := range strings.Split(output, "\n") {
		if got := lipgloss.Width(line); got > width {
			t.Errorf("line %d exceeds width %d: got %d, line=%q", i+1, width, got, line)
		}
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

// TestPaginationDotsApplied 回归：分页字形样式必须真正应用到 Paginator
// （否则 bubbles 默认深灰圆点在深色终端几乎不可见），且选中/未选中
// 用字形（★/☆）+ 粗细 + 颜色三重区分，字形后跟 2 空格间隔。
func TestPaginationDotsApplied(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	reg := adapters.NewRegistry()
	a := &app.App{Registry: reg, Config: cfg, Paths: config.Paths{}}
	m := newMain(ctx, a, []*model.Session{mkTuiSession("ses_1", "/home/user/nexora")}, nil, nil, "", "")

	active := m.list.Paginator.ActiveDot
	inactive := m.list.Paginator.InactiveDot
	if !strings.Contains(active, "★") || strings.Contains(active, "☆") {
		t.Errorf("选中分页位应为实心 ★, got %q", active)
	}
	if !strings.Contains(inactive, "☆") || strings.Contains(inactive, "★") {
		t.Errorf("未选中分页位应为空心 ☆, got %q", inactive)
	}
	if active == inactive {
		t.Error("选中与未选中必须使用不同字形")
	}
	// 分页字形后跟 2 空格间隔（pageDotGapCols）
	if !strings.Contains(active, "★  ") {
		t.Errorf("★ 后应有 2 空格间隔, got %q", active)
	}
	if !strings.Contains(inactive, "☆  ") {
		t.Errorf("☆ 后应有 2 空格间隔, got %q", inactive)
	}
	// 无 TTY 测试环境下 lipgloss 不输出 ANSI 序列，这里断言样式配置本身：
	// 当前页须加粗突出，且与非当前页颜色不同。
	if !m.list.Styles.ActivePaginationDot.GetBold() {
		t.Error("active dot should be bold to stand out")
	}
	if m.list.Styles.ActivePaginationDot.GetForeground() == m.list.Styles.InactivePaginationDot.GetForeground() {
		t.Error("active and inactive dots must use different colors")
	}
}

// TestPaginationDotsRender 验证：设置尺寸后，真实 View 渲染输出的分页点阵
// （Dots 分支，少量条目保证点阵放得下）同时含选中 ★ 与未选中 ☆、
// 字形后 2 空格间隔，且不含旧菱形 ◆ 或 emoji 🌟。
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
	if !strings.Contains(out, "★") || !strings.Contains(out, "☆") {
		t.Errorf("分页点阵应同时含选中 ★ 与未选中 ☆, got: %q", out)
	}
	if !strings.Contains(out, "★  ") {
		t.Errorf("点阵中 ★ 后应有 2 空格间隔, got: %q", out)
	}
	if strings.Contains(out, "◆") || strings.Contains(out, "🌟") {
		t.Errorf("分页不得含 ◆/🌟, got: %q", out)
	}
}

// TestStarSeparatorsInPaginationAndFooter 分页与页脚的星星字形（真实渲染输出）：
// 分页用 ★（选中）/☆（未选中）双字形区分；页脚恒为实心 ★（无选中概念）；
// 两处均不含旧菱形 ◆ 与 emoji 🌟。
// 本断言推翻了旧版“分页渲染不得含 ☆”——最终方案规定未选中位改用空心 ☆。
func TestStarSeparatorsInPaginationAndFooter(t *testing.T) {
	ctx := context.Background()
	a := &app.App{Registry: adapters.NewRegistry(), Config: config.Default(), Paths: config.Paths{}}
	var sessions []*model.Session
	for i := 0; i < 30; i++ {
		sessions = append(sessions, mkTuiSession(fmt.Sprintf("star_%02d", i), "/home/user/nexora"))
	}
	m := newMain(ctx, a, sessions, nil, nil, "", "")
	m.list.SetSize(80, 30)
	page := m.list.View()
	if !strings.Contains(page, "★") {
		t.Errorf("分页选中位应含实心 ★, got: %q", page)
	}
	if !strings.Contains(page, "☆") {
		t.Errorf("分页未选中位应含空心 ☆（推翻旧断言：☆ 现在必须出现）, got: %q", page)
	}
	if strings.Contains(page, "◆") || strings.Contains(page, "🌟") {
		t.Errorf("分页不得含 ◆/🌟, got: %q", page)
	}

	m.width = 140
	footer := m.keyHelpView()
	if !strings.Contains(footer, "★") {
		t.Errorf("页脚应含实心 ★, got: %q", footer)
	}
	if strings.Contains(footer, "☆") {
		t.Errorf("页脚无选中概念，不得含空心 ☆, got: %q", footer)
	}
	if strings.Contains(footer, "◆") || strings.Contains(footer, "🌟") {
		t.Errorf("页脚不得含 ◆/🌟, got: %q", footer)
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
	if !strings.Contains(out, "正在检查本地 Agent") && !strings.Contains(out, "Checking local agents") {
		t.Errorf("loading view should show sync message, got: %q", out)
	}
	if strings.Contains(out, "Agent Sessions") {
		t.Errorf("loading view should not use the list title, got: %q", out)
	}
}

func TestCachedListShowsBackgroundSyncStatus(t *testing.T) {
	m := newMain(context.Background(), &app.App{Registry: adapters.NewRegistry(), Config: config.Default()},
		[]*model.Session{mkTuiSession("cached", "/tmp")}, nil, nil, "", "")
	m.width, m.height = 100, 20
	m.list.SetSize(100, m.listHeight(m.height, m.width))
	m.syncing = true
	out := m.View()
	if !strings.Contains(out, "后台更新") && !strings.Contains(out, "Updating session index") {
		t.Fatalf("cached view should show syncing status: %q", out)
	}
	if !strings.Contains(out, "cached") {
		t.Fatalf("cached view should keep session list: %q", out)
	}
	m.syncing = false
	m.syncErr = errors.New("boom")
	out = m.View()
	if !strings.Contains(out, "后台同步失败") && !strings.Contains(out, "sync failed") {
		t.Fatalf("cached view should show sync failure: %q", out)
	}
}

func TestBackgroundSyncFailureKeepsCachedSessions(t *testing.T) {
	session := mkTuiSession("cached", "/tmp")
	m := newMain(context.Background(), &app.App{Registry: adapters.NewRegistry(), Config: config.Default()},
		[]*model.Session{session}, nil, nil, "", "")
	m.syncing = true
	m.indexErr = errors.New("boom")
	nm, _ := m.Update(indexedMsg{})
	got := nm.(*mainModel)
	if got.syncing || got.syncErr == nil || len(got.sessions) != 1 {
		t.Fatalf("background failure state: syncing=%v err=%v sessions=%d", got.syncing, got.syncErr, len(got.sessions))
	}
}

func TestIndexedMsgPreservesCachedSelection(t *testing.T) {
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
	first := mkTuiSession("first", "/tmp")
	second := mkTuiSession("second", "/tmp")
	for _, session := range []*model.Session{first, second} {
		if err := db.UpsertSession(ctx, session); err != nil {
			t.Fatal(err)
		}
	}
	if err := search.Populate(ctx, db); err != nil {
		t.Fatal(err)
	}
	m := newMain(ctx, &app.App{Registry: adapters.NewRegistry(), Config: config.Default()},
		[]*model.Session{first}, nil, db, "", "")
	m.syncing = true
	m.list.Select(0)
	if _, _ = m.Update(indexedMsg{}); m.list.SelectedItem().(item).sess.SessionID != "first" {
		t.Fatal("selected cached session should survive refresh")
	}
}

func TestLoadingViewShowsCurrentStage(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	reg := adapters.NewRegistry()
	a := &app.App{Registry: reg, Config: cfg, Paths: config.Paths{}}
	m := newMain(ctx, a, nil, nil, nil, "", "")
	m.loading = true
	m.loadingStage = syncer.StageSyncing
	out := m.View()
	for _, want := range []string{"✓ Check local agents", "● Sync session history", "○ Prepare session list", "Syncing session history…"} {
		if !strings.Contains(out, want) {
			t.Errorf("loading view missing %q, got: %q", want, out)
		}
	}
}

func TestLoadingViewChineseCopy(t *testing.T) {
	i18n.Set(i18n.LangZh)
	t.Cleanup(func() { i18n.Set(i18n.LangEn) })
	m := newMain(context.Background(), &app.App{Registry: adapters.NewRegistry(), Config: config.Default()}, nil, nil, nil, "", "")
	m.loading = true
	out := m.View()
	for _, want := range []string{"检查本地 Agent", "同步会话记录", "准备会话列表", "正在检查本地 Agent…", "数据量较大时可能需要一点时间", "按 q 退出"} {
		if !strings.Contains(out, want) {
			t.Errorf("Chinese loading view missing %q, got: %q", want, out)
		}
	}
	if strings.Contains(out, "首次") {
		t.Errorf("loading view should not imply first launch, got: %q", out)
	}
}

func TestRunIndexSendsLoadingStages(t *testing.T) {
	ctx := context.Background()
	db, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	a := &app.App{Registry: adapters.NewRegistry(), Config: config.Default()}
	m := newMain(ctx, a, nil, nil, db, "", "")
	var msgs []tea.Msg
	m.send = func(msg tea.Msg) { msgs = append(msgs, msg) }
	if _, ok := m.runIndex().(indexedMsg); !ok {
		t.Fatal("runIndex should return indexedMsg")
	}
	var got []syncer.Stage
	for _, msg := range msgs {
		if progress, ok := msg.(loadingStageMsg); ok {
			got = append(got, progress.stage)
		}
	}
	want := []syncer.Stage{syncer.StageDetecting, syncer.StageSyncing, syncer.StagePreparing}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("loading stage messages = %v, want %v", got, want)
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

func TestLoadingCardResponsiveWidth(t *testing.T) {
	wide := loadingCard(120, "Talea\nSync session history")
	if !strings.Contains(wide, "╭") || !strings.Contains(wide, "╰") {
		t.Fatalf("loading card should render rounded borders, got %q", wide)
	}
	assertLinesFit(t, wide, loadingCardMaxWidth)
	if got := lipgloss.Width(strings.Split(wide, "\n")[0]); got != loadingCardMaxWidth {
		t.Fatalf("wide card width = %d, want %d", got, loadingCardMaxWidth)
	}

	narrow := loadingCard(40, "Talea\nSync session history")
	assertLinesFit(t, narrow, 40)
	if got := lipgloss.Width(strings.Split(narrow, "\n")[0]); got >= loadingCardMaxWidth {
		t.Fatalf("narrow card should shrink, got width %d", got)
	}
}

func TestLoadingViewUsesResponsiveCard(t *testing.T) {
	m := newMain(context.Background(), &app.App{Registry: adapters.NewRegistry(), Config: config.Default()}, nil, nil, nil, "", "")
	m.loading = true
	m.width = 40
	m.height = 20
	out := m.loadingView()
	assertLinesFit(t, out, 40)
	if !strings.Contains(out, "╭") {
		t.Fatalf("loading view should include the card border, got %q", out)
	}
	if !loadingStyle.GetBold() || loadingStyle.GetBackground() == nil {
		t.Fatal("active loading style should use bold text and a background")
	}
}

func TestLoadingTitleCenteredAndProminent(t *testing.T) {
	for _, tc := range []struct {
		name  string
		width int
		err   bool
	}{
		{name: "normal", width: 32},
		{name: "narrow", width: 32},
		{name: "failure", width: 32, err: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newMain(context.Background(), &app.App{Registry: adapters.NewRegistry(), Config: config.Default()}, nil, nil, nil, "", "")
			m.loading = !tc.err
			m.loadingErr = nil
			if tc.err {
				m.loadingErr = errors.New("boom")
			}
			m.width = tc.width
			m.height = 20
			assertLoadingTitleCentered(t, m.loadingView(), tc.width)
		})
	}
	if !loadingTitleStyle.GetBold() || loadingTitleStyle.GetUnderline() {
		t.Fatal("loading title should be bold without an underline")
	}
}

func assertLoadingTitleCentered(t *testing.T, output string, width int) {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		idx := strings.Index(line, "Talea")
		if idx < 0 {
			continue
		}
		center := lipgloss.Width(line[:idx]) + lipgloss.Width("Talea")/2
		delta := center - width/2
		if delta < -1 || delta > 1 {
			t.Fatalf("Talea center=%d, want near %d, line=%q", center, width/2, line)
		}
		return
	}
	t.Fatalf("loading title not found in %q", output)
}

func TestLoadingTitleIsCleanAndResponsive(t *testing.T) {
	for _, width := range []int{32, 100} {
		title := loadingTitle(width)
		if !strings.Contains(title, "Talea") {
			t.Fatalf("title at width %d should contain Talea, got %q", width, title)
		}
		if strings.Contains(title, "█") || strings.Contains(title, "\n") {
			t.Fatalf("title at width %d should not use block bars or multiple lines, got %q", width, title)
		}
		assertLinesFit(t, title, loadingCardTextWidth(width))
	}
}

func TestKeyboardHelpIsCompleteAndResponsive(t *testing.T) {
	m := newMain(context.Background(), &app.App{Registry: adapters.NewRegistry(), Config: config.Default()}, nil, nil, nil, "", "")
	want := []string{
		"[enter]", "open session",
		"[d]", "details",
		"[o]", "resume session",
		"[esc]", "back",
		"[t]", "user turns",
		"[q]", "quit",
	}

	// 页脚总宽 ≈ 93（按键项）+ 5×9（4空格+★+4空格）= 138 列：
	// 分隔符 2→4 空格后 120 列已不足，140 列可容纳单行。
	wide := renderKeyHelp(m.keys.ShortHelp(), 140)
	if strings.Contains(wide, "\n") {
		t.Fatalf("wide keyboard help should fit on one line, got %q", wide)
	}
	for _, text := range want {
		if !strings.Contains(wide, text) {
			t.Errorf("wide keyboard help missing %q, got %q", text, wide)
		}
	}

	narrow := renderKeyHelp(m.keys.ShortHelp(), 44)
	if !strings.Contains(narrow, "\n") {
		t.Fatalf("narrow keyboard help should wrap, got %q", narrow)
	}
	for _, text := range want {
		if !strings.Contains(narrow, text) {
			t.Errorf("narrow keyboard help missing %q, got %q", text, narrow)
		}
	}
	assertLinesFit(t, narrow, 44)

	veryNarrow := renderKeyHelp(m.keys.ShortHelp(), 20)
	for _, text := range want {
		if !strings.Contains(veryNarrow, text) {
			t.Errorf("very narrow keyboard help missing %q, got %q", text, veryNarrow)
		}
	}
	assertLinesFit(t, veryNarrow, 20)
}

func TestKeyboardHelpUsesVisibleStarSeparators(t *testing.T) {
	m := newMain(context.Background(), &app.App{Registry: adapters.NewRegistry(), Config: config.Default()}, nil, nil, nil, "", "")
	out := renderKeyHelp(m.keys.ShortHelp(), 120)
	if !strings.Contains(out, footerSeparator) {
		t.Fatalf("keyboard help should contain %q, got %q", footerSeparator, out)
	}
	// 字面断言：不依赖常量本身，锁死 ★（非 ◆/☆/🌟）
	if !strings.Contains(out, "★") {
		t.Fatalf("keyboard help should contain star ★, got %q", out)
	}
	// 新间距：分隔符两侧各 4 空格（字面锁死 footerSepPadCols 生效）
	if !strings.Contains(out, "    ★    ") {
		t.Fatalf("keyboard help separator should have 4 spaces each side, got %q", out)
	}
	if strings.Contains(out, "◆") || strings.Contains(out, "☆") || strings.Contains(out, "🌟") {
		t.Fatalf("keyboard help should not contain ◆/☆/🌟, got %q", out)
	}
	if strings.Contains(out, "•") {
		t.Fatalf("keyboard help should not use the old bullet separator, got %q", out)
	}
	if !footerSeparatorStyle.GetBold() {
		t.Fatal("footer separator should be bold")
	}
}

// TestFooterSeparatorPaddingAndNarrowFit 页脚分隔符 4 空格间距常量生效；
// 40 列窄终端页脚不横向溢出、keyHelpLineCount 与实际渲染行数一致，
// listHeight 按页脚实际行数扣减（行数不硬编码）。
func TestFooterSeparatorPaddingAndNarrowFit(t *testing.T) {
	m := newMain(context.Background(), &app.App{Registry: adapters.NewRegistry(), Config: config.Default()}, nil, nil, nil, "", "")

	// 间距常量生效（字面锁死两侧各 4 空格）
	if wide := renderKeyHelp(m.keys.ShortHelp(), 140); !strings.Contains(wide, "    ★    ") {
		t.Fatalf("页脚分隔符两侧应各 4 空格, got %q", wide)
	}

	const width, height = 40, 24
	narrow := renderKeyHelp(m.keys.ShortHelp(), width)
	assertLinesFit(t, narrow, width)
	rows := strings.Count(narrow, "\n") + 1
	if want := keyHelpLineCount(m.keys.ShortHelp(), width); rows != want {
		t.Fatalf("keyHelpLineCount = %d, 实际渲染 %d 行", want, rows)
	}

	// listHeight 按页脚实际行数扣减（height-3-页脚行数，无状态条时）
	m.width, m.height = width, height
	if got, want := m.listHeight(height, width), height-3-rows; got != want {
		t.Fatalf("listHeight(40) = %d, want height-3-页脚行数 = %d", got, want)
	}
	narrowList := m.listHeight(height, width)

	m.width = 140
	wideRows := keyHelpLineCount(m.keys.ShortHelp(), 140)
	if wideRows >= rows {
		t.Fatalf("40 列页脚应比 140 列占更多行: 140列=%d 行, 40列=%d 行", wideRows, rows)
	}
	wideList := m.listHeight(height, 140)
	if got, want := wideList, height-3-wideRows; got != want {
		t.Fatalf("listHeight(140) = %d, want %d", got, want)
	}
	if narrowList >= wideList {
		t.Fatalf("页脚行数增加时列表高度应相应扣减: 40列列表=%d, 140列列表=%d", narrowList, wideList)
	}
}

func TestListHeightAccountsForKeyboardHelp(t *testing.T) {
	m := newMain(context.Background(), &app.App{Registry: adapters.NewRegistry(), Config: config.Default()}, nil, nil, nil, "", "")
	width, height := 44, 20
	rows := keyHelpLineCount(m.keys.ShortHelp(), width)
	if rows < 2 {
		t.Fatalf("narrow keyboard help should use multiple rows, got %d", rows)
	}
	if got, want := m.listHeight(height, width), height-3-rows; got != want {
		t.Fatalf("list height = %d, want %d", got, want)
	}
}

func TestMainViewUsesResponsiveKeyboardHelp(t *testing.T) {
	const width, height = 44, 20
	m := newMain(context.Background(), &app.App{Registry: adapters.NewRegistry(), Config: config.Default()}, nil, nil, nil, "", "")
	m.width = width
	m.height = height
	m.list.SetSize(width, m.listHeight(height, width))
	out := m.View()
	assertLinesFit(t, out, width)
	for _, text := range []string{"[enter]", "open session", "[q]", "quit"} {
		if !strings.Contains(out, text) {
			t.Errorf("main view missing keyboard hint %q, got %q", text, out)
		}
	}
}
