package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/talea/talea/internal/i18n"
	"github.com/talea/talea/internal/syncer"
)

// 同步状态条与完成提示的展示参数（时长/列数用具名常量，避免魔法数字）。
const (
	// syncDoneHideDelay 完成提示展示时长，到时自动隐藏并恢复列表高度。
	syncDoneHideDelay = 4 * time.Second
	// syncStatusPadX 状态条内容左右内衬（列数），避免文字贴边。
	syncStatusPadX = 1
	// progressBarSegments 进度条分段数 = 真实同步阶段数
	//（StageDetecting/StageSyncing/StagePreparing，见 internal/syncer）。
	progressBarSegments = 3
	// progressBarSegmentCells 常规档每段格数（整条 12 格，便于视认）。
	progressBarSegmentCells = 4
	// progressBarNarrowSegmentCells 极窄档每段格数：
	// 核心信息在常规档放不下时启用，三段结构保留、仍不做阶段内部分填充。
	progressBarNarrowSegmentCells = 2
)

// 状态条三态配色：去掉边框后由背景色块 + 加粗承担醒目性。
// 背景色三态区分（黄/绿/红系），前景按深浅终端自适应并与背景保持高对比
// （测试断言 WCAG 对比度）；阶段语义由进度条分段字形（█/▓/░）与右侧当前阶段名文字呈现，不只依赖颜色。
var (
	// syncDoingBG 进行中背景（黄系）。
	syncDoingBG = lipgloss.Color("#ffd166")
	// syncDoneBG 完成提示背景（绿系）。
	syncDoneBG = lipgloss.Color("#9BE564")
	// syncFailBG 失败提示背景（红系）。
	syncFailBG = lipgloss.Color("#FF6E6E")
	// 前景色：深浅终端取值不同，但均与各自背景色块高对比。
	syncDoingFG = lipgloss.AdaptiveColor{Light: "#3A2A00", Dark: "#221A00"}
	syncDoneFG  = lipgloss.AdaptiveColor{Light: "#133007", Dark: "#0E2406"}
	syncFailFG  = lipgloss.AdaptiveColor{Light: "#3D0009", Dark: "#2C0006"}
)

// hideSyncDoneMsg 触发完成提示自动隐藏。
type hideSyncDoneMsg struct{}

// syncStatusView 渲染顶部同步状态条；失败 / 进行中 / 完成提示互斥，同一时刻只显示一种。
func (m *mainModel) syncStatusView() string {
	return m.syncStatusViewWidth(m.width)
}

// syncStatusViewWidth 按指定终端宽度渲染单行状态条（背景高亮、无边框、与列表同宽）；
// listHeight 通过 statusBlockLines 用同一入口从实际渲染结果推导行数。
func (m *mainModel) syncStatusViewWidth(width int) string {
	switch {
	case m.syncErr != nil:
		return renderStatusLine(width, syncFailFG, syncFailBG, m.syncFailureText())
	case m.syncing:
		return renderStatusLine(width, syncDoingFG, syncDoingBG, m.doingText(width))
	case m.syncDoneVisible:
		return renderStatusLine(width, syncDoneFG, syncDoneBG, m.syncDoneText())
	default:
		return ""
	}
}

// renderStatusLine 渲染单行状态条：背景色块 + 加粗保证醒目，左右保留内衬。
// 文本先按可用宽度截断，保证内容恰好一行——不换行、不横向溢出。
func renderStatusLine(termWidth int, fg, bg lipgloss.TerminalColor, text string) string {
	style := lipgloss.NewStyle().
		Bold(true).
		Foreground(fg).
		Background(bg).
		Padding(0, syncStatusPadX)
	if termWidth <= 0 {
		return style.Render(text)
	}
	// Width 含内衬：先截到内容宽度，避免 lipgloss 按宽度换行破坏单行
	return style.Width(termWidth).Render(fitStatusText(text, termWidth))
}

// fitStatusText 将文本截断到可用内容宽度（超宽以省略号收尾），保证单行；
// termWidth<=0 时不截断。
func fitStatusText(text string, termWidth int) string {
	inner := termWidth - 2*syncStatusPadX
	if inner < 1 {
		inner = 1
	}
	return truncPad(text, inner)
}

// doingText 返回进行中状态条文本：spinner + 总标题（可裁）+ 分段进度条 + 当前阶段说明。
// 保留优先级：spinner+进度条+当前阶段说明（核心，进度条不参与裁剪）> 总标题 > 截断；
// 核心在常规档放不下时（如 40 列英文）进度条切换极窄档缩格，
// 但三段结构保留、填充仍严格等于阶段粒度。任何档位都只产出单行
// （超宽/超长兜底由 fitStatusText 截断）。
func (m *mainModel) doingText(width int) string {
	spinnerView := strings.TrimRight(rawSpinnerFrame(m.spinner), " ")
	title := i18n.Tr("Updating session index…", "正在后台更新会话索引…")
	msg := loadingStageMessage(m.loadingStage)
	segmentCells := progressBarSegmentCells
	bar := progressBar(m.loadingStage, segmentCells)
	core := spinnerView + " " + bar + " " + msg
	inner := width - 2*syncStatusPadX
	if width > 0 && runewidth.StringWidth(core) > inner {
		segmentCells = progressBarNarrowSegmentCells
		bar = progressBar(m.loadingStage, segmentCells)
		core = spinnerView + " " + bar + " " + msg
	}
	head := spinnerView + " " + title + " " + bar + " " + msg
	if width > 0 && runewidth.StringWidth(head) > inner {
		return core
	}
	return head
}

// rawSpinnerFrame 返回不带自身着色的 spinner 字形：
// 状态条整行统一样式（背景色块），嵌套 SGR 会打断整行背景。
func rawSpinnerFrame(sp spinner.Model) string {
	sp.Style = lipgloss.NewStyle() // Model 为值类型，拷贝不修改原模型
	return sp.View()
}

// progressBar 渲染按真实阶段分段的进度条：已完成段全 █、当前段全 ▓、未到段全 ░。
// 填充粒度严格等于阶段粒度（Stage 是离散值、没有阶段内进度），
// 绝不做阶段内部分填充；不显示任何百分比数字。
// █/▓/░ 字形本身不同且右侧伴随当前阶段名，阶段语义不只依赖颜色；
// 与状态条整行统一样式着色（不单独套 SGR，避免打断整行背景色块）。
// 分段下标与 Stage 枚举值一一对应（StageDetecting=0 起连续，测试锁死映射）。
func progressBar(current syncer.Stage, segmentCells int) string {
	var bar strings.Builder
	for seg := 0; seg < progressBarSegments; seg++ {
		fill := '░'
		switch stage := syncer.Stage(seg); {
		case stage < current:
			fill = '█'
		case stage == current:
			fill = '▓'
		}
		bar.WriteString(strings.Repeat(string(fill), segmentCells))
	}
	return bar.String()
}

// syncFailureText 生成失败文案：错误信息 + 当前显示缓存会话语义（不含 spinner 与完成提示）。
func (m *mainModel) syncFailureText() string {
	return i18n.Trf("Showing cached sessions; sync failed: %v",
		"当前显示缓存会话；后台同步失败：%v", m.syncErr)
}

// syncDoneText 生成完成提示文案：优先展示真实会话计数差，计数不可用时仅提示完成。
func (m *mainModel) syncDoneText() string {
	switch {
	case !m.syncDoneCounted:
		return i18n.Tr("Background sync finished", "后台同步已完成")
	case m.syncDoneDelta == 0:
		return i18n.Tr("Session index is up to date", "会话索引已是最新")
	case m.syncDoneDelta > 0:
		return i18n.Trf("Index updated · %d sessions added/updated",
			"索引已更新 · 新增/更新 %d 个会话", m.syncDoneDelta)
	default:
		return i18n.Trf("Index updated · session count changed by %d",
			"索引已更新 · 会话数变化 %d", m.syncDoneDelta)
	}
}

// statusBlockLines 返回状态条（含其后分隔空行）的实际渲染行数；无状态条时为 0。
// 不硬编码行数：始终由 lipgloss.Height 实际渲染结果推导。
func (m *mainModel) statusBlockLines(termWidth int) int {
	status := m.syncStatusViewWidth(termWidth)
	if status == "" {
		return 0
	}
	return lipgloss.Height(status) + 1 // +1 为 View 中状态条后的分隔空行
}

// applySyncDone 在后台同步完成后依据真实计数差设置完成提示；
// 无同步在进行（如测试直接投递 indexedMsg）时不展示提示。
func (m *mainModel) applySyncDone(msg indexedMsg, background bool) {
	if !background {
		m.syncDoneVisible = false
		return
	}
	m.syncDoneDelta = msg.delta
	m.syncDoneCounted = msg.countsOK
	m.syncDoneVisible = true
}

// scheduleSyncDoneHide 完成提示展示时启动一次性定时，到时隐藏提示并恢复列表高度。
func (m *mainModel) scheduleSyncDoneHide() tea.Cmd {
	if !m.syncDoneVisible {
		return nil
	}
	return tea.Tick(syncDoneHideDelay, func(time.Time) tea.Msg {
		return hideSyncDoneMsg{}
	})
}

// allowSpinnerTick 仅在加载/同步进行中续订 spinner tick；
// 完成提示与空闲状态停止 tick，避免无意义的定时唤醒（隐藏后也不恢复）。
func (m *mainModel) allowSpinnerTick(cmd tea.Cmd) tea.Cmd {
	if m.loading || m.syncing {
		return cmd
	}
	return nil
}

// refreshListSize 依据状态条实际行数重设列表尺寸，
// 保证状态条出现/消失时列表与底部快捷键提示不被推出可视区。
func (m *mainModel) refreshListSize() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	m.list.SetSize(m.width, m.listHeight(m.height, m.width))
}

// sessionCount 返回索引库会话总数；数据库不可用或查询失败时返回 false，
// 完成提示退化为不带数字的通用文案，不编造计数。
func (m *mainModel) sessionCount() (int, bool) {
	if m.db == nil {
		return 0, false
	}
	n, err := m.db.Count(m.ctx)
	if err != nil {
		return 0, false
	}
	return n, true
}
