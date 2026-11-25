package tui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *dashboardModel) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.filtering = false
		m.filter.Blur()
		m.filter.SetValue("")
		m.cursor = 0
		m.rebuild()
		return m, nil
	case "enter":
		m.filtering = false
		m.filter.Blur()
		m.cursor = 0
		m.rebuild()
		return m, nil
	}
	var cmd tea.Cmd
	m.filter, cmd = m.filter.Update(msg)
	m.cursor = 0
	m.rebuild()
	return m, cmd
}

func (m *dashboardModel) handleBoardKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	page := m.boardViewHeight() / 2
	if page < 1 {
		page = 1
	}
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Filter):
		m.filtering = true
		return m, m.filter.Focus()
	case key.Matches(msg, m.keys.Enter), key.Matches(msg, m.keys.Detail):
		if ss, ok := selectedBoardSession(m.rows, m.cursor); ok && ss.Session != nil {
			m.showDetail(ss.Session)
		}
		return m, nil
	case key.Matches(msg, m.keys.Open):
		if ss, ok := selectedBoardSession(m.rows, m.cursor); ok && ss.Session != nil {
			m.picked = ss.Session
			return m, tea.Quit
		}
		return m, nil
	case key.Matches(msg, m.keys.Up):
		m.moveCursor(-1)
	case key.Matches(msg, m.keys.Down):
		m.moveCursor(1)
	case key.Matches(msg, m.keys.PgUp):
		m.moveCursor(-page)
	case key.Matches(msg, m.keys.PgDn):
		m.moveCursor(page)
	case key.Matches(msg, m.keys.Home):
		m.cursor = 0
		m.ensureVisible()
	case key.Matches(msg, m.keys.End):
		m.cursor = maxCursor(m.rows)
		m.ensureVisible()
	case key.Matches(msg, m.keys.Back):
		if m.filter.Value() != "" {
			m.filter.SetValue("")
			m.cursor = 0
			m.rebuild()
		}
	}
	return m, nil
}

func (m *dashboardModel) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Back), key.Matches(msg, m.keys.Quit):
		m.detail = nil
		return m, nil
	case key.Matches(msg, m.keys.Open):
		if m.detail != nil && m.detail.sess != nil {
			m.picked = m.detail.sess
			return m, tea.Quit
		}
	case key.Matches(msg, key.NewBinding(key.WithKeys("t"))):
		if m.detail != nil {
			m.detail.turnsVisible = !m.detail.turnsVisible
			m.detail.contentValid = false
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.detail.view, cmd = m.detail.view.Update(msg)
	return m, cmd
}

func (m *dashboardModel) moveCursor(delta int) {
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if max := maxCursor(m.rows); m.cursor > max {
		m.cursor = max
	}
	m.ensureVisible()
}

func maxCursor(rows []boardRow) int {
	n := len(selectableSessions(rows))
	if n == 0 {
		return 0
	}
	return n - 1
}
