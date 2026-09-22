package tui

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/config"
	"github.com/talea/talea/internal/i18n"
	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
	"github.com/talea/talea/internal/search"
	"github.com/talea/talea/internal/syncer"
)

// spinnerFrames 是 spinner.Dot 的全部字形，用于断言 spinner 是否渲染/停止。
const spinnerFrames = "⣾⣽⣻⢿⡿⣟⣯⣷"

// hasSpinnerFrame 判断渲染结果中是否含 spinner 字形。
func hasSpinnerFrame(s string) bool { return strings.ContainsAny(s, spinnerFrames) }

// countViewLines 返回渲染结果的行数。
func countViewLines(s string) int { return strings.Count(s, "\n") + 1 }

// newSyncTestApp 构造空注册表测试 App：syncer 不会发现真实 Agent 数据，计数可预期。
func newSyncTestApp() *app.App {
	return &app.App{Registry: adapters.NewRegistry(), Config: config.Default()}
}

// setupSyncDB 创建临时索引库并写入指定会话（调用方负责 Migrate 后的数据可用性）。
func setupSyncDB(t *testing.T, sessions ...*model.Session) *index.DB {
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
	if err := search.Ensure(ctx, db); err != nil {
		t.Fatal(err)
	}
	for _, s := range sessions {
		if err := db.UpsertSession(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	if len(sessions) > 0 {
		if err := search.Populate(ctx, db); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

// newCachedSyncModel 构造带缓存会话、已设置窗口尺寸的主模型。
func newCachedSyncModel(t *testing.T, db *index.DB, width, height int) *mainModel {
	t.Helper()
	m := newMain(context.Background(), newSyncTestApp(),
		[]*model.Session{mkTuiSession("cached", "/tmp")}, nil, db, "", "")
	m.width, m.height = width, height
	m.list.SetSize(width, m.listHeight(height, width))
	return m
}

// TestSyncInProgressRendersBorderedStageBlock 4.1：进行中渲染带边框状态块，
// 含 spinner、三态阶段标记与当前阶段说明，列表与快捷键提示仍在视口内。
func TestSyncInProgressRendersBorderedStageBlock(t *testing.T) {
	m := newCachedSyncModel(t, nil, 100, 24)
	m.syncing = true
	m.loadingStage = syncer.StageSyncing
	m.refreshListSize() // 真实流程由 Update 转换路径重设列表尺寸
	out := m.View()
	for _, want := range []string{
		"╭", "╰", // 圆角边框
		"⣾",                        // spinner
		"Updating session index",   // 状态块主文案
		"✓ Check local agents",     // 已完成阶段
		"● Sync session history",   // 当前阶段
		"○ Prepare session list",   // 待办阶段
		"Syncing session history…", // 当前阶段说明
	} {
		if !strings.Contains(out, want) {
			t.Errorf("进行中状态块缺少 %q, got: %q", want, out)
		}
	}
	if !strings.Contains(out, "cached") {
		t.Errorf("状态块不得遮挡会话列表, got: %q", out)
	}
	if !strings.Contains(out, "[q]") {
		t.Errorf("状态块不得把快捷键提示推出视口, got: %q", out)
	}
	if got := countViewLines(out); got > m.height {
		t.Errorf("视图 %d 行超过终端高度 %d", got, m.height)
	}
}

// TestSyncStatusBlockLineAccounting 4.2：listHeight 按状态块真实渲染行数扣减
// （进行中与失败多行状态块均覆盖），列表与快捷键提示不被推出视口。
func TestSyncStatusBlockLineAccounting(t *testing.T) {
	m := newCachedSyncModel(t, nil, 100, 24)
	base := m.listHeight(m.height, m.width)

	m.syncing = true
	m.loadingStage = syncer.StageSyncing
	doingLines := lipgloss.Height(m.syncStatusView()) + 1 // 状态块 + 分隔空行
	if doingLines <= 1 {
		t.Fatalf("进行中状态块应为多行, got %d 行: %q", doingLines, m.syncStatusView())
	}
	if got, want := m.listHeight(m.height, m.width), base-doingLines; got != want {
		t.Fatalf("进行中 listHeight = %d, want %d（按实际行数 %d 扣减）", got, want, doingLines)
	}

	m.syncing = false
	m.syncErr = errors.New(strings.Repeat("boom-", 30))
	failLines := lipgloss.Height(m.syncStatusView()) + 1
	if failLines <= 1 {
		t.Fatalf("失败状态块应为多行, got %d 行: %q", failLines, m.syncStatusView())
	}
	if got, want := m.listHeight(m.height, m.width), base-failLines; got != want {
		t.Fatalf("失败 listHeight = %d, want %d（按实际行数 %d 扣减）", got, want, failLines)
	}

	m.refreshListSize()
	out := m.View()
	if got := countViewLines(out); got > m.height {
		t.Errorf("失败态视图 %d 行超过高度 %d", got, m.height)
	}
	if !strings.Contains(out, "[q]") || !strings.Contains(out, "cached") {
		t.Errorf("列表或快捷键提示被推出视口: %q", out)
	}
}

// TestSyncDoneNoticeShowsRealDelta 4.3：存在变化时展示真实数量的完成提示。
func TestSyncDoneNoticeShowsRealDelta(t *testing.T) {
	m := newCachedSyncModel(t, nil, 100, 24)
	m.syncDoneVisible = true
	m.syncDoneCounted = true
	m.syncDoneDelta = 12
	m.refreshListSize()
	out := m.View()
	if !strings.Contains(out, "12") || !strings.Contains(out, "sessions added/updated") {
		t.Errorf("完成提示应含真实数量, got: %q", out)
	}
	if !strings.Contains(out, "╭") {
		t.Errorf("完成提示应为带边框状态块, got: %q", out)
	}
	if hasSpinnerFrame(out) {
		t.Errorf("完成提示不得渲染 spinner, got: %q", out)
	}
	if got := countViewLines(out); got > m.height {
		t.Errorf("完成提示视图 %d 行超过高度 %d", got, m.height)
	}
}

// TestSyncDoneNoticeZeroChange 4.3：无变化时显示无变化文案，不得出现虚假增长。
func TestSyncDoneNoticeZeroChange(t *testing.T) {
	m := newCachedSyncModel(t, nil, 100, 24)
	m.syncDoneVisible = true
	m.syncDoneCounted = true
	m.syncDoneDelta = 0
	m.refreshListSize()
	out := m.View()
	if !strings.Contains(out, "Session index is up to date") {
		t.Errorf("零变化应显示无变化文案, got: %q", out)
	}
	for _, bad := range []string{"sessions added/updated", "0 sessions"} {
		if strings.Contains(out, bad) {
			t.Errorf("零变化不得出现 %q, got: %q", bad, out)
		}
	}
}

// TestSyncDoneNoticeAutoHideRestoresListHeight 4.3：同步完成后展示完成提示，
// 到时自动隐藏，隐藏后不再渲染且 listHeight 恢复到同步前基线。
func TestSyncDoneNoticeAutoHideRestoresListHeight(t *testing.T) {
	db := setupSyncDB(t, mkTuiSession("cached", "/tmp"))
	m := newCachedSyncModel(t, db, 100, 24)
	base := m.listHeight(m.height, m.width)

	m.syncing = true
	nm, cmd := m.Update(indexedMsg{delta: 5, countsOK: true})
	mm := nm.(*mainModel)
	if !mm.syncDoneVisible {
		t.Fatal("后台同步完成后应显示完成提示")
	}
	if cmd == nil {
		t.Fatal("完成提示应注册自动隐藏定时命令")
	}
	shown := mm.listHeight(mm.height, mm.width)
	wantShown := base - (lipgloss.Height(mm.syncStatusView()) + 1)
	if shown != wantShown {
		t.Fatalf("提示展示期间 listHeight = %d, want %d", shown, wantShown)
	}
	if out := mm.View(); !strings.Contains(out, "sessions added/updated") || !strings.Contains(out, "5") {
		t.Fatalf("展示期间应含真实数量, got: %q", out)
	}

	nm2, _ := mm.Update(hideSyncDoneMsg{})
	mm2 := nm2.(*mainModel)
	if mm2.syncDoneVisible {
		t.Fatal("到时后完成提示应隐藏")
	}
	out := mm2.View()
	if strings.Contains(out, "sessions added/updated") {
		t.Errorf("隐藏后不得再渲染完成提示, got: %q", out)
	}
	if got := mm2.listHeight(mm2.height, mm2.width); got != base {
		t.Errorf("隐藏后 listHeight 应恢复 %d, got %d（不得残留占位空行）", base, got)
	}
	if got := countViewLines(out); got > mm2.height {
		t.Errorf("隐藏后视图 %d 行超过高度 %d", got, mm2.height)
	}
}

// TestSyncFailureDistinctFromInProgressAndDone 4.4：失败态与进行中/完成态
// 在文案与视觉上可区分，且不得同时渲染 spinner 或完成提示。
func TestSyncFailureDistinctFromInProgressAndDone(t *testing.T) {
	m := newCachedSyncModel(t, nil, 100, 24)
	// 人为残留完成提示与进行中文案来源，验证失败态渲染优先级与互斥
	m.syncDoneVisible = true
	m.syncDoneCounted = true
	m.syncDoneDelta = 12
	m.syncErr = errors.New("boom")
	m.refreshListSize()
	out := m.View()
	if !strings.Contains(out, "Showing cached sessions") || !strings.Contains(out, "boom") {
		t.Errorf("失败态应含缓存语义与错误信息, got: %q", out)
	}
	for _, bad := range []string{"sessions added/updated", "Updating session index", "up to date"} {
		if strings.Contains(out, bad) {
			t.Errorf("失败态不得同时显示其他状态文案 %q, got: %q", bad, out)
		}
	}
	if hasSpinnerFrame(out) {
		t.Errorf("失败态不得渲染 spinner, got: %q", out)
	}
	if got := countViewLines(out); got > m.height {
		t.Errorf("失败态视图 %d 行超过高度 %d", got, m.height)
	}
	// 无 TTY 下 lipgloss 不输出 ANSI：断言三态边框/文案样式配置本身可区分
	if syncFailBorder == syncDoneBorder || syncFailBorder == syncDoingBorder || syncDoneBorder == syncDoingBorder {
		t.Fatal("进行中/完成/失败必须使用可区分的边框颜色")
	}
	if !syncDoneNoticeStyle.GetBold() || !errorStyle.GetBold() {
		t.Fatal("完成与失败文案应加粗突出")
	}

	// finishSyncError 必须清理残留完成提示并停止 syncing
	m2 := newCachedSyncModel(t, nil, 100, 24)
	m2.syncing = true
	m2.syncDoneVisible = true
	m2.finishSyncError(errors.New("boom"))
	if m2.syncDoneVisible || m2.syncing || m2.syncErr == nil {
		t.Fatalf("失败后状态: notice=%v syncing=%v err=%v",
			m2.syncDoneVisible, m2.syncing, m2.syncErr)
	}
}

// TestSyncStatusFitsNarrowTerminal 窄终端（40 列）三种状态均不横向溢出，
// 且列表与快捷键提示保持在视口内。
func TestSyncStatusFitsNarrowTerminal(t *testing.T) {
	const width, height = 40, 24
	for _, tc := range []struct {
		name  string
		setup func(*mainModel)
	}{
		{name: "syncing", setup: func(m *mainModel) {
			m.syncing = true
			m.loadingStage = syncer.StageSyncing
		}},
		{name: "done", setup: func(m *mainModel) {
			m.syncDoneVisible = true
			m.syncDoneCounted = true
			m.syncDoneDelta = 12
		}},
		{name: "failure", setup: func(m *mainModel) {
			m.syncErr = errors.New(strings.Repeat("boom-", 30))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newCachedSyncModel(t, nil, width, height)
			tc.setup(m)
			m.refreshListSize()
			out := m.View()
			assertLinesFit(t, out, width)
			if got := countViewLines(out); got > height {
				t.Errorf("%s: 视图 %d 行超过高度 %d", tc.name, got, height)
			}
			if !strings.Contains(out, "[q]") {
				t.Errorf("%s: 快捷键提示被推出视口: %q", tc.name, out)
			}
		})
	}
}

// TestSyncStatusChineseCopy 中文文案覆盖：进行中 / 完成（有变化与零变化）/ 失败。
func TestSyncStatusChineseCopy(t *testing.T) {
	i18n.Set(i18n.LangZh)
	t.Cleanup(func() { i18n.Set(i18n.LangEn) })

	m := newCachedSyncModel(t, nil, 100, 24)
	m.syncing = true
	m.loadingStage = syncer.StageSyncing
	out := m.View()
	for _, want := range []string{
		"正在后台更新会话索引", "检查本地 Agent", "同步会话记录", "准备会话列表", "正在同步会话记录…",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("中文进行中状态块缺少 %q, got: %q", want, out)
		}
	}

	m.syncing = false
	m.syncDoneVisible = true
	m.syncDoneCounted = true
	m.syncDoneDelta = 12
	if out = m.View(); !strings.Contains(out, "索引已更新 · 新增/更新 12 个会话") {
		t.Errorf("中文完成提示缺少真实数量, got: %q", out)
	}
	m.syncDoneDelta = 0
	if out = m.View(); !strings.Contains(out, "会话索引已是最新") {
		t.Errorf("中文零变化提示缺失, got: %q", out)
	}

	m.syncDoneVisible = false
	m.syncErr = errors.New("boom")
	out = m.View()
	for _, want := range []string{"当前显示缓存会话", "后台同步失败", "boom"} {
		if !strings.Contains(out, want) {
			t.Errorf("中文失败提示缺少 %q, got: %q", want, out)
		}
	}
}

// TestSpinnerStopsTickingAfterSyncDone 完成提示与空闲状态不再续订 spinner tick。
func TestSpinnerStopsTickingAfterSyncDone(t *testing.T) {
	m := newCachedSyncModel(t, nil, 100, 24)
	tick := spinner.TickMsg{Time: time.Now(), ID: m.spinner.ID()}

	m.syncing = true
	if _, cmd := m.Update(tick); cmd == nil {
		t.Fatal("同步进行中应续订 spinner tick")
	}
	m.syncing = false
	m.syncDoneVisible = true
	if _, cmd := m.Update(tick); cmd != nil {
		t.Fatal("完成提示期间不得续订 spinner tick")
	}
	m.syncDoneVisible = false
	if _, cmd := m.Update(tick); cmd != nil {
		t.Fatal("空闲状态不得续订 spinner tick")
	}
}

// TestRunIndexReportsRealCountDelta 完成提示的数量必须来自 db.Count 前后对比：
// 同步期间真实新增会话时 delta 反映真实差值；数据库不可用时计数标记不可用。
func TestRunIndexReportsRealCountDelta(t *testing.T) {
	ctx := context.Background()
	db := setupSyncDB(t) // 空库；空注册表 syncer 不会发现真实 Agent 数据
	m := newMain(ctx, newSyncTestApp(), nil, nil, db, "", "")
	var insertErr error
	m.send = func(msg tea.Msg) {
		// 进入准备阶段（索引阶段已结束）时写入一个新会话，模拟同步期间增长
		if st, ok := msg.(loadingStageMsg); ok && st.stage == syncer.StagePreparing && insertErr == nil {
			insertErr = db.UpsertSession(ctx, mkTuiSession("inserted-after-sync", "/tmp"))
		}
	}
	msg, ok := m.runIndex().(indexedMsg)
	if !ok {
		t.Fatal("runIndex 应返回 indexedMsg")
	}
	if insertErr != nil {
		t.Fatalf("写入测试会话失败: %v", insertErr)
	}
	if !msg.countsOK {
		t.Fatal("真实数据库下计数应可用")
	}
	if msg.delta != 1 {
		t.Fatalf("计数差 = %d, want 1（必须来自 db.Count 前后对比）", msg.delta)
	}

	mNil := newMain(ctx, newSyncTestApp(), nil, nil, nil, "", "")
	if _, ok := mNil.sessionCount(); ok {
		t.Fatal("无数据库时计数应标记为不可用")
	}
}
