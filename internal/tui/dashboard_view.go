package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/talea/talea/internal/i18n"
	"github.com/talea/talea/internal/status"
)

// boardGroup 是看板中的一个状态分组。
type boardGroup struct {
	state    status.DisplayState
	title    string
	sessions []status.SessionStatus
}

// boardRow 是扁平化的看板行：分组标题或会话卡片。
type boardRow struct {
	header  bool
	title   string
	session status.SessionStatus
}

// boardGroupOrder 是固定分组顺序：需要你 → 进行中 → 已完成 → 失败/中断 → 空闲 → 未知。
var boardGroupOrder = []status.DisplayState{
	status.StateWaiting,
	status.StateWorking,
	status.StateCompleted,
	status.StateFailed,
	status.StateIdle,
	status.StateUnknown,
}

// groupStatuses 按状态分组，并在组内按最近活动倒序排列。
func groupStatuses(sessions []status.SessionStatus, filter string) []boardGroup {
	filter = strings.ToLower(strings.TrimSpace(filter))
	groups := make([]boardGroup, 0, len(boardGroupOrder))
	index := make(map[status.DisplayState]int, len(boardGroupOrder))
	for _, state := range boardGroupOrder {
		index[state] = len(groups)
		groups = append(groups, boardGroup{state: state, title: status.GroupTitle(state)})
	}
	for _, ss := range sessions {
		if !matchesBoardFilter(ss, filter) {
			continue
		}
		idx, ok := index[ss.State]
		if !ok {
			idx = index[status.StateUnknown]
		}
		groups[idx].sessions = append(groups[idx].sessions, ss)
	}
	for i := range groups {
		sortBoardSessions(groups[i].sessions)
	}
	return groups
}

func matchesBoardFilter(ss status.SessionStatus, filter string) bool {
	if filter == "" {
		return true
	}
	haystack := strings.ToLower(strings.Join([]string{
		string(ss.AgentID), ss.SessionID, ss.ProjectName,
		ss.WorkingDirectory, sessionQuestion(ss),
	}, " "))
	return strings.Contains(haystack, filter)
}

func sessionQuestion(ss status.SessionStatus) string {
	if ss.Session == nil {
		return ""
	}
	return ss.Session.FirstQuestion
}

func sortBoardSessions(sessions []status.SessionStatus) {
	// 插入排序：分组内通常只有少量会话，稳定且无需额外依赖。
	for i := 1; i < len(sessions); i++ {
		for j := i; j > 0 && boardSessionAfter(sessions[j], sessions[j-1]); j-- {
			sessions[j], sessions[j-1] = sessions[j-1], sessions[j]
		}
	}
}

func boardSessionAfter(a, b status.SessionStatus) bool {
	if a.LastActivityAt == nil {
		return false
	}
	if b.LastActivityAt == nil {
		return true
	}
	if !a.LastActivityAt.Equal(*b.LastActivityAt) {
		return a.LastActivityAt.After(*b.LastActivityAt)
	}
	return a.SessionID < b.SessionID
}

// buildBoardRows 把分组展开为分组标题 + 会话卡片行。
func buildBoardRows(groups []boardGroup) []boardRow {
	var rows []boardRow
	for _, group := range groups {
		if len(group.sessions) == 0 {
			continue
		}
		rows = append(rows, boardRow{
			header: true,
			title:  fmt.Sprintf("%s (%d)", group.title, len(group.sessions)),
		})
		for _, ss := range group.sessions {
			rows = append(rows, boardRow{session: ss})
		}
	}
	return rows
}

func selectableSessions(rows []boardRow) []status.SessionStatus {
	out := make([]status.SessionStatus, 0, len(rows))
	for _, row := range rows {
		if !row.header {
			out = append(out, row.session)
		}
	}
	return out
}

func boardSessionKey(ss status.SessionStatus) string {
	return ss.AgentInstanceID + "\x00" + ss.SessionID
}

func selectedBoardSession(rows []boardRow, cursor int) (status.SessionStatus, bool) {
	sessions := selectableSessions(rows)
	if cursor < 0 || cursor >= len(sessions) {
		return status.SessionStatus{}, false
	}
	return sessions[cursor], true
}

func boardRowLines(row boardRow, width int, selected bool) []string {
	if row.header {
		return []string{dashboardGroupStyle.Render(fitCell(row.title, width))}
	}
	ss := row.session
	return []string{
		dashboardSessionLine(ss, width, selected),
		dashboardTaskLine(ss, width),
	}
}

func dashboardSessionLine(ss status.SessionStatus, width int, selected bool) string {
	marker := "  "
	if selected {
		marker = "> "
	}
	label := ss.Label
	if label == "" {
		label = status.StateLabel(ss)
	}
	if ss.Confidence == status.ConfidenceInferred {
		label += " (" + i18n.Tr("inferred", "推断") + ")"
	}
	parts := []string{
		marker + dashboardIcon(ss.State),
		label,
		string(ss.AgentID),
		firstNonEmptyDashboard(ss.ProjectName, ss.WorkingDirectory, "—"),
		ss.SessionID,
		relativeBoardTime(ss.LastActivityAt),
	}
	if token := boardTokens(ss.TotalTokens); token != "" {
		parts = append(parts, token)
	}
	line := strings.Join(parts, "  ")
	line = fitCell(line, width)
	if selected {
		return dashboardSelectedStyle.Render(line)
	}
	return dashboardStateStyle(ss.State).Render(line)
}

func dashboardTaskLine(ss status.SessionStatus, width int) string {
	question := firstLine(sessionQuestion(ss))
	reason := firstLine(ss.Reason)
	text := question
	if text == "" {
		text = reason
	} else if (ss.State == status.StateWaiting || ss.State == status.StateFailed) && reason != "" {
		// 等待/失败时在任务后补充原因，避免用户只看到任务却不知道为何需要介入。
		text += " · " + reason
	}
	return dashboardTaskStyle.Render(fitCell("    "+text, width))
}

func dashboardIcon(state status.DisplayState) string {
	switch state {
	case status.StateWaiting:
		return "⏸"
	case status.StateWorking:
		return "●"
	case status.StateCompleted:
		return "✓"
	case status.StateFailed:
		return "✕"
	case status.StateIdle:
		return "·"
	default:
		return "?"
	}
}

func relativeBoardTime(t *time.Time) string {
	if t == nil {
		return i18n.Tr("no activity", "无活动")
	}
	d := time.Since(*t)
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return i18n.Tr("just now", "刚刚")
	case d < time.Hour:
		return fmt.Sprintf("%dm %s", int(d.Minutes()), i18n.Tr("ago", "前"))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh %s", int(d.Hours()), i18n.Tr("ago", "前"))
	default:
		return fmt.Sprintf("%dd %s", int(d.Hours()/24), i18n.Tr("ago", "前"))
	}
}

func boardTokens(v *int64) string {
	if v == nil {
		return ""
	}
	return humanNum(*v)
}

func firstNonEmptyDashboard(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

var (
	dashboardGroupStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.AdaptiveColor{Light: "#5b2a86", Dark: "#ffd166"})
	dashboardTaskStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#4a4a4a", Dark: "#b0b0b0"})
	dashboardSelectedStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.AdaptiveColor{Light: "#ffffff", Dark: "#000000"}).
				Background(lipgloss.AdaptiveColor{Light: "#5b2a86", Dark: "#ffd166"})
)

func dashboardStateStyle(state status.DisplayState) lipgloss.Style {
	switch state {
	case status.StateWaiting:
		return lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#B26A00", Dark: "#FFB74D"})
	case status.StateWorking:
		return lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#00695C", Dark: "#4DD0E1"})
	case status.StateCompleted:
		return lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#2E7D32", Dark: "#9BE564"})
	case status.StateFailed:
		return lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#B00020", Dark: "#FF6E6E"})
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#6b6b6b", Dark: "#9a9a9a"})
	}
}

func dashboardSummary(counts status.Counts) string {
	return fmt.Sprintf("%s %d · %s %d · %s %d · %s %d · %s %d · %s %d",
		status.GroupTitle(status.StateWaiting), counts.Waiting,
		status.GroupTitle(status.StateWorking), counts.Working,
		status.GroupTitle(status.StateCompleted), counts.Completed,
		status.GroupTitle(status.StateFailed), counts.Failed,
		status.GroupTitle(status.StateIdle), counts.Idle,
		status.GroupTitle(status.StateUnknown), counts.Unknown)
}

func dashboardWidth(width int) int {
	if width <= 0 {
		return 100
	}
	return width
}
