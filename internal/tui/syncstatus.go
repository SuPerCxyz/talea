package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/talea/talea/internal/i18n"
)

// 同步状态块与完成提示的展示参数（时长/宽度均用具名常量，避免魔法数字）。
const (
	// syncDoneHideDelay 完成提示展示时长，到时自动隐藏并恢复列表高度。
	syncDoneHideDelay = 4 * time.Second
	// syncStatusCardMaxWidth 状态块整体最大宽度，宽屏下避免文案过散。
	syncStatusCardMaxWidth = 64
	// syncStatusBorderWidth 状态块左右边框合计占位列数。
	syncStatusBorderWidth = 2
	// syncStatusPadX 状态块内容左右内边距（列数）。
	syncStatusPadX = 1
)

// 状态块边框与文案颜色：进行中 / 完成 / 失败三态互斥且视觉可区分，
// 统一 AdaptiveColor 适配深浅终端背景；阶段语义另用 ○/●/✓ 标记，不只靠颜色。
var (
	syncDoingBorder = lipgloss.AdaptiveColor{Light: "#7e57c2", Dark: "#ffd166"}
	syncDoneBorder  = lipgloss.AdaptiveColor{Light: "#2E7D32", Dark: "#9BE564"}
	syncFailBorder  = lipgloss.AdaptiveColor{Light: "#B00020", Dark: "#FF6E6E"}
	// syncDoneNoticeStyle 完成提示主文案：加粗高对比，显著性高于普通列表文本。
	syncDoneNoticeStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.AdaptiveColor{Light: "#2E7D32", Dark: "#9BE564"})
)

// hideSyncDoneMsg 触发完成提示自动隐藏。
type hideSyncDoneMsg struct{}

// syncStatusView 渲染顶部同步状态块；失败 / 进行中 / 完成提示互斥，同一时刻只显示一种。
func (m *mainModel) syncStatusView() string {
	return m.syncStatusViewWidth(m.width)
}

// syncStatusViewWidth 按指定终端宽度渲染状态块；listHeight 用同一入口计算真实行数。
func (m *mainModel) syncStatusViewWidth(width int) string {
	switch {
	case m.syncErr != nil:
		return syncStatusCard(width, syncFailBorder, m.syncFailureContent())
	case m.syncing:
		return syncStatusCard(width, syncDoingBorder, m.syncDoingContent())
	case m.syncDoneVisible:
		return syncStatusCard(width, syncDoneBorder, syncDoneNoticeStyle.Render(m.syncDoneText()))
	default:
		return ""
	}
}

// syncDoingContent 生成进行中内容：spinner + 真实同步阶段（○/●/✓ 三态标记）+ 当前阶段说明。
func (m *mainModel) syncDoingContent() string {
	header := m.spinner.View() + " " +
		loadingStyle.Render(i18n.Tr("Updating session index…", "正在后台更新会话索引…"))
	return strings.Join([]string{
		header,
		loadingStagesView(m.loadingStage),
		loadingStyle.Render(loadingStageMessage(m.loadingStage)),
	}, "\n")
}

// syncFailureContent 生成失败内容：错误信息 + 当前显示缓存会话语义（不含 spinner 与完成提示）。
func (m *mainModel) syncFailureContent() string {
	return errorStyle.Render(i18n.Trf(
		"Showing cached sessions; sync failed: %v",
		"当前显示缓存会话；后台同步失败：%v", m.syncErr))
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

// syncStatusCard 渲染带圆角边框的同步状态块；宽度随终端收缩，窄终端不横向溢出。
func syncStatusCard(termWidth int, border lipgloss.TerminalColor, content string) string {
	style := lipgloss.NewStyle().
		Width(syncStatusInnerWidth(termWidth)).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(0, syncStatusPadX)
	return style.Render(content)
}

// syncStatusInnerWidth 返回状态块内容区宽度（含内边距、不含左右边框）。
func syncStatusInnerWidth(termWidth int) int {
	inner := syncStatusCardMaxWidth - syncStatusBorderWidth
	if termWidth > 0 {
		inner = termWidth - syncStatusBorderWidth
		if cap := syncStatusCardMaxWidth - syncStatusBorderWidth; inner > cap {
			inner = cap
		}
	}
	if inner < 1 {
		inner = 1
	}
	return inner
}

// statusBlockLines 返回状态块（含其后分隔空行）的实际渲染行数；无状态块时为 0。
func (m *mainModel) statusBlockLines(termWidth int) int {
	status := m.syncStatusViewWidth(termWidth)
	if status == "" {
		return 0
	}
	return lipgloss.Height(status) + 1 // +1 为 View 中状态块后的分隔空行
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

// refreshListSize 依据状态块实际行数重设列表尺寸，
// 保证状态块出现/消失时列表与底部快捷键提示不被推出可视区。
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
