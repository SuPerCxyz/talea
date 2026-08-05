// Package tui 实现 Bubble Tea 终端界面。
package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
	"github.com/talea/talea/internal/resume"
	"github.com/talea/talea/internal/search"
)

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	itemStyle   = lipgloss.NewStyle().PaddingLeft(2)
	detailStyle = lipgloss.NewStyle().Padding(1, 2)
)

// Run 启动 TUI。
func Run(ctx context.Context) error {
	a, err := app.New(ctx)
	if err != nil {
		return err
	}
	db, err := index.Open(a.Paths.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		return err
	}
	results, err := search.List(ctx, db, search.Query{Limit: 500})
	if err != nil {
		return err
	}
	var sessions []*model.Session
	for i := range results {
		sessions = append(sessions, &results[i].Session)
	}
	a.ResolveWorkingDirs(ctx, sessions)
	a.SortSessions(sessions)

	m := newMain(ctx, a, sessions, db)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

// mainModel 是主 TUI 模型。
type mainModel struct {
	ctx      context.Context
	app      *app.App
	db       *index.DB
	sessions []*model.Session
	list     list.Model
	detail   *detailModel
	keys     keyMap
	help     help.Model
	width    int
	height   int
}

type keyMap struct {
	Open  key.Binding
	Quit  key.Binding
	Enter key.Binding
	Back  key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Enter, k.Back, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Enter, k.Back, k.Quit}}
}

type item struct {
	title string
	desc  string
	sess  *model.Session
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string {
	return i.sess.FirstQuestion + " " + string(i.sess.AgentID) + " " + i.sess.SessionID
}

func newMain(ctx context.Context, a *app.App, sessions []*model.Session, db *index.DB) *mainModel {
	items := make([]list.Item, 0, len(sessions))
	for _, s := range sessions {
		items = append(items, item{
			title: sessionTitle(s),
			desc:  sessionDesc(s),
			sess:  s,
		})
	}
	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Talea · Agent Sessions"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)

	km := keyMap{
		Open:  key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "恢复会话")),
		Quit:  key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "退出")),
		Enter: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "详情")),
		Back:  key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "返回列表")),
	}
	return &mainModel{
		ctx:      ctx,
		app:      a,
		db:       db,
		sessions: sessions,
		list:     l,
		keys:     km,
		help:     help.New(),
	}
}

func sessionTitle(s *model.Session) string {
	agent := displayAgent(s.AgentID)
	timeStr := ""
	if s.StartedAt != nil {
		timeStr = s.StartedAt.Format("01-02 15:04")
	}
	q := firstLine(s.FirstQuestion)
	if q == "" {
		q = "未识别到有效用户提问"
	}
	return fmt.Sprintf("[%s] %s  %s", agent, timeStr, q)
}

func sessionDesc(s *model.Session) string {
	var parts []string
	if s.Duration != nil {
		parts = append(parts, "时长 "+humanDur(*s.Duration))
	}
	if s.TokenUsage != nil && s.TokenUsage.TotalTokens != nil {
		parts = append(parts, fmt.Sprintf("Token %s", humanNum(*s.TokenUsage.TotalTokens)))
	}
	if s.WorkingDirectory != "" {
		parts = append(parts, "目录 "+shortHome(s.WorkingDirectory))
	}
	if s.GitBranch != "" {
		parts = append(parts, "分支 "+s.GitBranch)
	}
	if s.Activity == model.ActivityActive {
		parts = append(parts, "进行中")
	}
	if len(parts) == 0 {
		return s.SessionID
	}
	return strings.Join(parts, "  ·  ")
}

func displayAgent(a model.AgentID) string {
	switch a {
	case model.AgentClaudeCode:
		return "Claude"
	case model.AgentCodexCLI:
		return "Codex"
	case model.AgentOpenCode:
		return "OpenCode"
	default:
		return string(a)
	}
}

// detailModel 是会话详情视图。
type detailModel struct {
	ctx   context.Context
	app   *app.App
	sess  *model.Session
	view  viewport.Model
	ready bool
}

// Init 初始化模型。
func (m *mainModel) Init() tea.Cmd {
	return nil
}

// Update 处理消息。
func (m *mainModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		h := m.height - 4
		m.list.SetSize(msg.Width, h)
		m.help.Width = msg.Width
		if m.detail != nil {
			m.detail.view.Width = msg.Width
			m.detail.view.Height = msg.Height - 2
		}
		return m, nil
	case tea.KeyMsg:
		if m.detail != nil {
			return m.handleDetailKey(msg)
		}
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Open):
			return m, m.resumeSelected()
		case key.Matches(msg, m.keys.Enter):
			if it, ok := m.list.SelectedItem().(item); ok {
				m.showDetail(it.sess)
			}
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *mainModel) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Back), key.Matches(msg, m.keys.Quit):
		m.detail = nil
		return m, nil
	case key.Matches(msg, m.keys.Open):
		return m, m.resumeDetail()
	}
	var cmd tea.Cmd
	m.detail.view, cmd = m.detail.view.Update(msg)
	return m, cmd
}

// resumeSelected 恢复当前选中会话。
func (m *mainModel) resumeSelected() tea.Cmd {
	it, ok := m.list.SelectedItem().(item)
	if !ok {
		return nil
	}
	return m.doResume(it.sess)
}

func (m *mainModel) resumeDetail() tea.Cmd {
	if m.detail == nil {
		return nil
	}
	return m.doResume(m.detail.sess)
}

func (m *mainModel) doResume(s *model.Session) tea.Cmd {
	plan, err := resume.Build(*s, "", m.app.Config.PathMapping)
	if err != nil {
		return func() tea.Msg { return resumeErrMsg{err: err} }
	}
	if !plan.DirExists {
		return func() tea.Msg {
			return resumeErrMsg{err: fmt.Errorf("原工作目录不存在：%s\n可以执行：talea open %s --cwd <新目录>", plan.TargetDir, s.SessionID)}
		}
	}
	ad, ok := m.app.Registry.Get(s.AgentID)
	if !ok {
		return func() tea.Msg { return resumeErrMsg{err: errors.New("会话格式不支持")} }
	}
	resumer, ok := adapters.As[adapters.Resumer](ad)
	if !ok {
		return func() tea.Msg { return resumeErrMsg{err: errors.New("Agent 不支持恢复能力")} }
	}
	cmd2, err := resumer.BuildResumeCommand(*s, plan.TargetDir)
	if err != nil {
		return func() tea.Msg { return resumeErrMsg{err: err} }
	}
	plan.Command = cmd2

	return func() tea.Msg {
		if err := resume.Exec(plan); err != nil {
			return resumeErrMsg{err: err}
		}
		return nil
	}
}

type resumeErrMsg struct{ err error }

// View 渲染。
func (m *mainModel) View() string {
	if m.detail != nil {
		return m.detail.View()
	}
	var sb strings.Builder
	sb.WriteString(titleStyle.Render(m.list.Title) + "\n\n")
	sb.WriteString(m.list.View())
	sb.WriteString("\n" + m.help.View(m.keys))
	return sb.String()
}

// doResumeDetail 进入详情页。
func (m *mainModel) showDetail(s *model.Session) {
	m.detail = &detailModel{ctx: m.ctx, app: m.app, sess: s}
}
