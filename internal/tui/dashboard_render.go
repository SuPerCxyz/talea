package tui

import (
	"strings"

	"github.com/talea/talea/internal/i18n"
	"github.com/talea/talea/internal/model"
)

func (m *dashboardModel) View() string {
	if m.detail != nil {
		return m.detail.View()
	}
	if m.loading && len(m.rows) == 0 {
		return m.spinner.View() + " " + i18n.Tr("Loading agent activity…", "正在加载 Agent 活动…")
	}
	var sb strings.Builder
	sb.WriteString(titleStyle.Render(i18n.Tr("Talea · Agent Activity", "Talea · Agent 活动看板")) + "\n")
	sb.WriteString(dashboardSummary(m.snapshot.Counts) + "\n")
	if m.refreshErr != nil {
		sb.WriteString(errorStyle.Render(i18n.Tr("sync: ", "同步：")+m.refreshErr.Error()) + "\n")
	}
	if m.loadErr != nil && len(m.snapshot.Sessions) == 0 {
		sb.WriteString(errorStyle.Render(m.loadErr.Error()) + "\n")
	}
	for i, warning := range m.snapshot.Warnings {
		if i >= 2 {
			break
		}
		sb.WriteString(dimStyle.Render(i18n.Tr("warning: ", "警告：")+warning) + "\n")
	}
	if m.filtering || m.filter.Value() != "" {
		if m.filtering {
			sb.WriteString(m.filter.View() + "\n")
		} else {
			sb.WriteString(dimStyle.Render("/ "+m.filter.Value()) + "\n")
		}
	}
	sb.WriteString("\n")
	if len(m.rows) == 0 {
		sb.WriteString(dimStyle.Render(i18n.Tr("No sessions to show", "没有可展示的会话")) + "\n")
	} else {
		sb.WriteString(m.rowsView())
	}
	sb.WriteString("\n" + renderKeyHelp(m.keys.ShortHelp(), dashboardWidth(m.width)))
	return sb.String()
}

func (m *dashboardModel) rowsView() string {
	available := m.boardViewHeight()
	width := dashboardWidth(m.width)
	var sb strings.Builder
	lines := 0
	for i := m.offset; i < len(m.rows) && lines < available; i++ {
		row := m.rows[i]
		selected := false
		if !row.header {
			if ss, ok := selectedBoardSession(m.rows, m.cursor); ok {
				selected = boardSessionKey(ss) == boardSessionKey(row.session)
			}
		}
		for _, line := range boardRowLines(row, width, selected) {
			if lines >= available {
				break
			}
			sb.WriteString(line + "\n")
			lines++
		}
	}
	return sb.String()
}

func (m *dashboardModel) boardViewHeight() int {
	// 标题、汇总、空行、最多两行警告、过滤行、页脚
	h := m.height - 8
	if h < 2 {
		h = 2
	}
	return h
}

func (m *dashboardModel) rebuild() {
	groups := groupStatuses(m.snapshot.Sessions, m.filter.Value())
	m.rows = buildBoardRows(groups)
	if max := maxCursor(m.rows); m.cursor > max {
		m.cursor = max
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.ensureVisible()
}

func (m *dashboardModel) selectByKey(key string) {
	if key == "" {
		return
	}
	for i, ss := range selectableSessions(m.rows) {
		if boardSessionKey(ss) == key {
			m.cursor = i
			m.ensureVisible()
			return
		}
	}
}

func (m *dashboardModel) ensureVisible() {
	if len(m.rows) == 0 {
		m.offset = 0
		return
	}
	target := flatIndex(m.rows, m.cursor)
	if target < 0 {
		m.offset = 0
		return
	}
	if m.offset > target {
		m.offset = target
	}
	avail := m.boardViewHeight()
	for m.offset < target && flatLines(m.rows, m.offset, target) > avail {
		m.offset++
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func flatIndex(rows []boardRow, cursor int) int {
	n := 0
	for i, row := range rows {
		if row.header {
			continue
		}
		if n == cursor {
			return i
		}
		n++
	}
	return -1
}

func flatLines(rows []boardRow, from, to int) int {
	lines := 0
	for i := from; i <= to && i < len(rows); i++ {
		if rows[i].header {
			lines++
		} else {
			lines += 2
		}
	}
	return lines
}

func (m *dashboardModel) showDetail(sess *model.Session) {
	w := dashboardWidth(m.width)
	h := m.height
	if h <= 0 {
		h = 30
	}
	m.detail = &detailModel{ctx: m.ctx, app: m.app, db: m.db, sess: sess, width: w, height: h - 2}
}
