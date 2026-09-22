package tui

import (
	"context"
	"errors"
	"math"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

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

// minContrastRatio 状态条前景/背景的最低 WCAG 对比度（AA 正文阈值）。
const minContrastRatio = 4.5

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

// sgrPattern 匹配 ANSI SGR 序列（CSI … m）的参数串。
var sgrPattern = regexp.MustCompile(`\x1b\[([0-9;]+)m`)

// sgrTokens 返回字符串中全部 SGR 序列的 ;-分隔参数组。
func sgrTokens(s string) [][]string {
	var out [][]string
	for _, match := range sgrPattern.FindAllStringSubmatch(s, -1) {
		out = append(out, strings.Split(match[1], ";"))
	}
	return out
}

// hasBoldToken 判断 SGR 参数中是否含加粗指令（参数 1）。
func hasBoldToken(groups [][]string) bool {
	for _, group := range groups {
		for _, param := range group {
			if param == "1" {
				return true
			}
		}
	}
	return false
}

// hasBackgroundToken 判断 SGR 参数中是否含背景色指令：
// 基础背景 40-47、高亮背景 100-107 或真彩背景 48;2;R;G;B。
func hasBackgroundToken(groups [][]string) bool {
	for _, group := range groups {
		for _, param := range group {
			if len(param) == 2 && param[0] == '4' && param[1] >= '0' && param[1] <= '7' {
				return true
			}
			if len(param) == 3 && strings.HasPrefix(param, "10") &&
				param[2] >= '0' && param[2] <= '7' {
				return true
			}
			if param == "48" {
				return true
			}
		}
	}
	return false
}

// srgbLuminance 解析 #RRGGBB 并计算 WCAG 相对亮度。
func srgbLuminance(hex string) (float64, bool) {
	v, err := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 32)
	if err != nil {
		return 0, false
	}
	r, g, b := float64((v>>16)&0xff)/255, float64((v>>8)&0xff)/255, float64(v&0xff)/255
	lin := func(c float64) float64 {
		if c <= 0.03928 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b), true
}

// contrastRatio 返回两颜色的 WCAG 对比度（1~21）。
func contrastRatio(t *testing.T, hexA, hexB string) float64 {
	t.Helper()
	la, okA := srgbLuminance(hexA)
	lb, okB := srgbLuminance(hexB)
	if !okA || !okB {
		t.Fatalf("颜色解析失败: %q %q", hexA, hexB)
	}
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// TestSyncStatusBackgroundHighlightSGR 状态条以背景色块 + 加粗承担醒目性：
// 强制 ANSI 色彩档位后，三态渲染均输出背景 SGR 与加粗指令，且仍为单行。
func TestSyncStatusBackgroundHighlightSGR(t *testing.T) {
	saved := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(saved) })
	lipgloss.SetColorProfile(termenv.ANSI)

	for _, tc := range []struct {
		name  string
		setup func(*mainModel)
	}{
		{name: "doing", setup: func(m *mainModel) {
			m.syncing = true
			m.loadingStage = syncer.StageSyncing
		}},
		{name: "done", setup: func(m *mainModel) {
			m.syncDoneVisible = true
			m.syncDoneCounted = true
			m.syncDoneDelta = 12
		}},
		{name: "failure", setup: func(m *mainModel) { m.syncErr = errors.New("boom") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newCachedSyncModel(t, nil, 140, 24)
			tc.setup(m)
			status := m.syncStatusView()
			if got := lipgloss.Height(status); got != 1 {
				t.Fatalf("状态条应为单行, got %d 行: %q", got, status)
			}
			tokens := sgrTokens(status)
			if len(tokens) == 0 {
				t.Fatalf("状态条应输出 ANSI SGR（背景色块未生效）: %q", status)
			}
			if !hasBoldToken(tokens) {
				t.Errorf("状态条应加粗（SGR 参数 1）, got: %q", status)
			}
			if !hasBackgroundToken(tokens) {
				t.Errorf("状态条应含背景色 SGR, got: %q", status)
			}
		})
	}
}

// TestSyncStatusColorContrast 状态条前景/背景在深浅终端取值下均满足
// WCAG AA 对比度（4.5:1），保证两种背景下都可读。
func TestSyncStatusColorContrast(t *testing.T) {
	for _, tc := range []struct {
		name string
		bg   lipgloss.Color
		fg   lipgloss.AdaptiveColor
	}{
		{name: "doing", bg: syncDoingBG, fg: syncDoingFG},
		{name: "done", bg: syncDoneBG, fg: syncDoneFG},
		{name: "failure", bg: syncFailBG, fg: syncFailFG},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, fgHex := range []string{tc.fg.Light, tc.fg.Dark} {
				ratio := contrastRatio(t, string(tc.bg), fgHex)
				if ratio < minContrastRatio {
					t.Errorf("bg=%s fg=%s 对比度 %.2f < %.1f（深浅终端取值均需达标）",
						tc.bg, fgHex, ratio, minContrastRatio)
				}
			}
		})
	}
}

// TestSyncInProgressRendersSingleLineStatus 进行中渲染背景高亮的单行状态条：
// 无边框，含 spinner、总标题、三阶段横向标记与当前阶段说明，列表与快捷键提示仍在视口内。
func TestSyncInProgressRendersSingleLineStatus(t *testing.T) {
	m := newCachedSyncModel(t, nil, 140, 24)
	m.syncing = true
	m.loadingStage = syncer.StageSyncing
	m.refreshListSize() // 真实流程由 Update 转换路径重设列表尺寸
	status := m.syncStatusView()
	if got := lipgloss.Height(status); got != 1 {
		t.Fatalf("状态条内容应为 1 行, got %d 行: %q", got, status)
	}
	for _, border := range []string{"╭", "╮", "╰", "╯"} {
		if strings.Contains(status, border) {
			t.Errorf("状态条不应再渲染边框 %q, got: %q", border, status)
		}
	}
	out := m.View()
	for _, want := range []string{
		"⣾",                        // spinner（首帧）
		"Updating session index",   // 总标题
		"████▓▓▓▓░░░░",             // 分段进度条（StageSyncing：完成/当前/未到各 4 格）
		"Syncing session history…", // 当前阶段说明
	} {
		if !strings.Contains(out, want) {
			t.Errorf("进行中状态条缺少 %q, got: %q", want, out)
		}
	}
	// 三段阶段名文字与 ○/●/✓ 标记已被分段进度条取代，不得再出现
	for _, gone := range []string{"✓", "●", "○", "Check local agents", "Prepare session list"} {
		if strings.Contains(status, gone) {
			t.Errorf("阶段标记/阶段名 %q 应被进度条取代, got: %q", gone, status)
		}
	}
	if !strings.Contains(out, "cached") {
		t.Errorf("状态条不得遮挡会话列表, got: %q", out)
	}
	if !strings.Contains(out, "[q]") {
		t.Errorf("状态条不得把快捷键提示推出视口, got: %q", out)
	}
	if got := countViewLines(out); got > m.height {
		t.Errorf("视图 %d 行超过终端高度 %d", got, m.height)
	}
}

// TestSyncStatusBlockLineAccounting 状态条固定 2 行（1 内容 + 1 分隔空行）：
// listHeight 按实际渲染行数扣减；三态行数一致，切换状态时列表高度不跳动。
func TestSyncStatusBlockLineAccounting(t *testing.T) {
	m := newCachedSyncModel(t, nil, 140, 24)
	base := m.listHeight(m.height, m.width) // 无状态条基线

	measure := func(name string) int {
		m.refreshListSize()
		status := m.syncStatusView()
		if got := lipgloss.Height(status); got != 1 {
			t.Fatalf("%s: 状态条内容应为 1 行, got %d 行: %q", name, got, status)
		}
		if got := m.statusBlockLines(m.width); got != 2 {
			t.Fatalf("%s: statusBlockLines = %d, want 2（1 内容 + 1 分隔空行）", name, got)
		}
		want := base - 2
		if got := m.listHeight(m.height, m.width); got != want {
			t.Fatalf("%s: listHeight = %d, want %d（按实际行数扣减）", name, got, want)
		}
		return m.listHeight(m.height, m.width)
	}

	m.syncing = true
	m.loadingStage = syncer.StageSyncing
	doing := measure("进行中")

	m.syncing = false
	m.syncDoneVisible = true
	m.syncDoneCounted = true
	m.syncDoneDelta = 12
	done := measure("完成")

	m.syncDoneVisible = false
	m.syncErr = errors.New(strings.Repeat("boom-", 30)) // 长错误截断后仍单行
	fail := measure("失败")

	if doing != done || done != fail {
		t.Fatalf("三态行数必须一致（切换状态列表不跳动）: doing=%d done=%d fail=%d", doing, done, fail)
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

// TestSyncStatusDegradationKeepsCoreFirst 单行放不下时按优先级降级且始终单行：
// 核心（spinner+进度条+当前说明，进度条不参与裁剪）→ 总标题 → 省略号截断；
// 极窄档进度条可缩格，但三段结构保留、不做阶段内部分填充。
func TestSyncStatusDegradationKeepsCoreFirst(t *testing.T) {
	atWidth := func(width int) *mainModel {
		m := newCachedSyncModel(t, nil, width, 24)
		m.syncing = true
		m.loadingStage = syncer.StageSyncing
		return m
	}
	assertSingleLine := func(name string, width int, status string) {
		t.Helper()
		if got := lipgloss.Height(status); got != 1 {
			t.Fatalf("%s: 降级后仍应为 1 行, got %d 行: %q", name, got, status)
		}
		assertLinesFit(t, status, width)
	}
	const (
		fullBar   = "████▓▓▓▓░░░░" // 常规档 12 格（4/段）
		narrowBar = "██▓▓░░"       // 极窄档 6 格（2/段），三段结构保留
		msg       = "Syncing session history…"
	)

	head := atWidth(140).syncStatusView()
	assertSingleLine("含总标题档", 140, head)
	for _, want := range []string{"Updating session index", fullBar, msg} {
		if !strings.Contains(head, want) {
			t.Errorf("宽终端应显示总标题+完整进度条+当前说明, 缺少 %q: %q", want, head)
		}
	}

	core := atWidth(50).syncStatusView()
	assertSingleLine("核心档", 50, core)
	if !hasSpinnerFrame(core) || !strings.Contains(core, fullBar) || !strings.Contains(core, msg) {
		t.Errorf("核心档必须保留 spinner+完整进度条+当前说明: %q", core)
	}
	if strings.Contains(core, "Updating session index") {
		t.Errorf("核心档应先裁掉总标题: %q", core)
	}

	narrow := atWidth(40).syncStatusView()
	assertSingleLine("极窄核心档", 40, narrow)
	if !strings.Contains(narrow, narrowBar) || !strings.Contains(narrow, msg) {
		t.Errorf("极窄档应缩格进度条并保留三段结构与当前说明: %q", narrow)
	}
	if strings.Contains(narrow, fullBar) || strings.Contains(narrow, "Updating session index") {
		t.Errorf("极窄档不应含常规档进度条或总标题: %q", narrow)
	}

	trimmed := atWidth(20).syncStatusView()
	assertSingleLine("截断档", 20, trimmed)
	if !strings.Contains(trimmed, "…") {
		t.Errorf("连核心都放不下时应以省略号截断: %q", trimmed)
	}
}

// TestProgressBarFillsPerStageExactly 进度条分段填充锁死：
// 填充粒度严格等于阶段粒度（每段内字形单一，绝无阶段内部分填充），
// 分段数等于真实阶段数；极窄档缩格后三段结构保留。
func TestProgressBarFillsPerStageExactly(t *testing.T) {
	if int(syncer.StageDetecting) != 0 || int(syncer.StagePreparing)+1 != progressBarSegments {
		t.Fatalf("进度条分段数必须等于真实阶段数: StageDetecting=%d StagePreparing=%d segments=%d",
			int(syncer.StageDetecting), int(syncer.StagePreparing), progressBarSegments)
	}
	for _, tc := range []struct {
		name         string
		stage        syncer.Stage
		full, narrow string
	}{
		{name: "detecting", stage: syncer.StageDetecting, full: "▓▓▓▓░░░░░░░░", narrow: "▓▓░░░░"},
		{name: "syncing", stage: syncer.StageSyncing, full: "████▓▓▓▓░░░░", narrow: "██▓▓░░"},
		{name: "preparing", stage: syncer.StagePreparing, full: "████████▓▓▓▓", narrow: "████▓▓"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			full := progressBar(tc.stage, progressBarSegmentCells)
			if full != tc.full {
				t.Fatalf("常规档进度条 = %q, want %q", full, tc.full)
			}
			narrow := progressBar(tc.stage, progressBarNarrowSegmentCells)
			if narrow != tc.narrow {
				t.Fatalf("极窄档进度条 = %q, want %q", narrow, tc.narrow)
			}
			assertSegmentsUniform(t, full, progressBarSegmentCells)
			assertSegmentsUniform(t, narrow, progressBarNarrowSegmentCells)
		})
	}
}

// assertSegmentsUniform 断言进度条总格数正确且每段内字形完全一致
// （锁死“填充粒度 = 阶段粒度，不做阶段内部分填充”）。
func assertSegmentsUniform(t *testing.T, bar string, cells int) {
	t.Helper()
	runes := []rune(bar)
	want := progressBarSegments * cells
	if len(runes) != want {
		t.Fatalf("进度条应为 %d 格, got %d 格: %q", want, len(runes), bar)
	}
	for seg := 0; seg < progressBarSegments; seg++ {
		first := runes[seg*cells]
		for c := 1; c < cells; c++ {
			if runes[seg*cells+c] != first {
				t.Fatalf("第 %d 段内字形不一致（阶段内部分填充）: %q", seg+1, bar)
			}
		}
	}
}

// 数字进度探测：百分比（如 33%）与 x/y 形式（如 1/3）。
var (
	percentPattern = regexp.MustCompile(`\d+(\.\d+)?%`)
	ratioPattern   = regexp.MustCompile(`\d+\s*/\s*\d+`)
)

// TestSyncStatusNoNumericProgress 进行中状态条不得出现百分比或数字进度：
// 同步流程只有离散阶段、没有可显示的真实百分比，进度只以分段字形表达。
func TestSyncStatusNoNumericProgress(t *testing.T) {
	for _, width := range []int{140, 60, 40} {
		m := newCachedSyncModel(t, nil, width, 24)
		m.syncing = true
		m.loadingStage = syncer.StageSyncing
		status := m.syncStatusView()
		if !strings.ContainsAny(status, "█▓░") {
			t.Errorf("width=%d: 状态条应含分段进度条: %q", width, status)
		}
		if strings.Contains(status, "%") || percentPattern.MatchString(status) {
			t.Errorf("width=%d: 状态条不得显示百分比进度: %q", width, status)
		}
		if ratioPattern.MatchString(status) {
			t.Errorf("width=%d: 状态条不得显示 x/y 数字进度: %q", width, status)
		}
	}
}

// TestSyncDoneNoticeShowsRealDelta 存在变化时展示真实数量的完成提示（无边框单行）。
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
	status := m.syncStatusView()
	if lipgloss.Height(status) != 1 || strings.Contains(status, "╭") {
		t.Errorf("完成提示应为无边框单行状态条, got: %q", status)
	}
	if hasSpinnerFrame(out) {
		t.Errorf("完成提示不得渲染 spinner, got: %q", out)
	}
	if got := countViewLines(out); got > m.height {
		t.Errorf("完成提示视图 %d 行超过高度 %d", got, m.height)
	}
}

// TestSyncDoneNoticeZeroChange 无变化时显示无变化文案，不得出现虚假增长。
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

// TestSyncDoneNoticeAutoHideRestoresListHeight 同步完成后展示完成提示，
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

// TestSyncFailureDistinctFromInProgressAndDone 失败态与进行中/完成态在文案与
// 视觉上可区分，且不得同时渲染 spinner 或完成提示。
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
	// 三态前景/背景配置可区分（加粗与背景 SGR 由 TestSyncStatusBackgroundHighlightSGR 断言）
	if syncFailBG == syncDoneBG || syncFailBG == syncDoingBG || syncDoneBG == syncDoingBG {
		t.Fatal("进行中/完成/失败必须使用可区分的背景色")
	}
	if syncFailFG == syncDoneFG || syncFailFG == syncDoingFG || syncDoneFG == syncDoingFG {
		t.Fatal("进行中/完成/失败必须使用可区分的前景色")
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

// TestSyncStatusFitsNarrowTerminal 窄终端（40 列）三种状态均不横向溢出、
// 保持单行降级，且列表与快捷键提示保持在视口内。
func TestSyncStatusFitsNarrowTerminal(t *testing.T) {
	const width, height = 40, 24
	for _, tc := range []struct {
		name       string
		setup      func(*mainModel)
		contains   []string
		notContain []string
	}{
		{name: "syncing", setup: func(m *mainModel) {
			m.syncing = true
			m.loadingStage = syncer.StageSyncing
		},
			// 40 列降级到核心档：spinner + 进度条（极窄档缩格、三段保留）+ 当前阶段说明
			contains:   []string{"██▓▓░░", "Syncing session history…"},
			notContain: []string{"Updating session index", "Check local agents"}},
		{name: "done", setup: func(m *mainModel) {
			m.syncDoneVisible = true
			m.syncDoneCounted = true
			m.syncDoneDelta = 12
		},
			contains: []string{"Index updated", "12"}},
		{name: "failure", setup: func(m *mainModel) {
			m.syncErr = errors.New(strings.Repeat("boom-", 30))
		},
			contains: []string{"sync failed", "Showing cached sessions"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newCachedSyncModel(t, nil, width, height)
			tc.setup(m)
			m.refreshListSize()
			status := m.syncStatusView()
			if got := lipgloss.Height(status); got != 1 {
				t.Errorf("%s: 状态条应保持单行, got %d 行: %q", tc.name, got, status)
			}
			if got := m.statusBlockLines(width); got != 2 {
				t.Errorf("%s: statusBlockLines = %d, want 2", tc.name, got)
			}
			for _, want := range tc.contains {
				if !strings.Contains(status, want) {
					t.Errorf("%s: 状态条缺少核心信息 %q: %q", tc.name, want, status)
				}
			}
			for _, bad := range tc.notContain {
				if strings.Contains(status, bad) {
					t.Errorf("%s: 窄终端不应包含已裁剪内容 %q: %q", tc.name, bad, status)
				}
			}
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

	m := newCachedSyncModel(t, nil, 140, 24)
	m.syncing = true
	m.loadingStage = syncer.StageSyncing
	status := m.syncStatusView()
	if got := lipgloss.Height(status); got != 1 {
		t.Fatalf("中文状态条应为单行, got %d 行: %q", got, status)
	}
	out := m.View()
	for _, want := range []string{
		"正在后台更新会话索引",   // 总标题
		"████▓▓▓▓░░░░", // 分段进度条
		"正在同步会话记录…",    // 当前阶段说明
	} {
		if !strings.Contains(out, want) {
			t.Errorf("中文进行中状态条缺少 %q, got: %q", want, out)
		}
	}
	for _, gone := range []string{"检查本地 Agent", "准备会话列表"} {
		if strings.Contains(status, gone) {
			t.Errorf("中文阶段名 %q 应被进度条取代, got: %q", gone, status)
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
	if got := lipgloss.Height(m.syncStatusView()); got != 1 {
		t.Errorf("中文失败提示应为单行, got %d 行", got)
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
