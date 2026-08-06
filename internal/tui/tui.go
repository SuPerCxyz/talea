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
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
	"github.com/talea/talea/internal/resume"
	"github.com/talea/talea/internal/search"
)

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
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
	if err := search.Ensure(ctx, db); err != nil {
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
	if _, err := p.Run(); err != nil {
		return err
	}
	// Bubble Tea 已退出并恢复终端，此时执行选中的会话恢复
	if m.picked != nil {
		return resumeSession(ctx, a, m.picked, "", false)
	}
	return nil
}

// resumeSession 执行会话恢复（复用 cli 包逻辑）。
func resumeSession(ctx context.Context, a *app.App, s *model.Session, cwd string, dryRun bool) error {
	plan, err := resume.Build(*s, cwd, a.Config.PathMapping)
	if err != nil {
		return err
	}
	if !plan.DirExists {
		return fmt.Errorf("原工作目录不存在：%s\n可以执行：talea go %s --cwd <新目录>", plan.TargetDir, s.SessionID)
	}
	ad, ok := a.Registry.Get(s.AgentID)
	if !ok {
		return errors.New("会话格式不支持")
	}
	resumer, ok := adapters.As[adapters.Resumer](ad)
	if !ok {
		return errors.New("agent 不支持恢复能力")
	}
	cmd2, err := resumer.BuildResumeCommand(*s, plan.TargetDir)
	if err != nil {
		return err
	}
	plan.Command = cmd2
	return resume.Exec(plan)
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
	picked   *model.Session
}

type keyMap struct {
	Open     key.Binding
	Quit     key.Binding
	Enter    key.Binding
	Back     key.Binding
	Timeline key.Binding
	Context  key.Binding
	Model    key.Binding
	Turns    key.Binding
	Charts   key.Binding
	Usage    key.Binding
	Subags   key.Binding
	Preview  key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Enter, k.Timeline, k.Charts, k.Usage, k.Subags, k.Back, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Enter, k.Timeline, k.Context, k.Model, k.Turns, k.Charts, k.Usage, k.Subags, k.Preview, k.Back, k.Quit}}
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
		Open:     key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "恢复会话")),
		Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "退出")),
		Enter:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "详情")),
		Back:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "返回列表")),
		Timeline: key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "Token 时间线")),
		Context:  key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "上下文曲线")),
		Model:    key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "模型汇总")),
		Turns:    key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "用户轮次")),
		Charts:   key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "图表")),
		Usage:    key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "Token 汇总")),
		Subags:   key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "子 Agent")),
		Preview:  key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "对话预览")),
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
	// 固定 agent 列（8 宽左对齐）与时间列（11 宽），保证各行对齐
	return fmt.Sprintf("[%-8s] %s  %s", agent, timeStr, q)
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
	// 每段前缀固定 8 显示宽度（左对齐），时长/Token/目录等值列对齐
	var sb strings.Builder
	for i, p := range parts {
		if i > 0 {
			sb.WriteString("  ")
		}
		colon := strings.IndexByte(p, ' ')
		if colon > 0 {
			prefix := p[:colon+1]
			sb.WriteString(prefix)
			pad := 8 - runewidth.StringWidth(prefix)
			if pad > 0 {
				sb.WriteString(strings.Repeat(" ", pad))
			}
			sb.WriteString(p[colon+1:])
		} else {
			sb.WriteString(p)
		}
	}
	return sb.String()
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
			if it, ok := m.list.SelectedItem().(item); ok {
				m.picked = it.sess
				return m, tea.Quit
			}
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
		if m.detail != nil {
			m.picked = m.detail.sess
			return m, tea.Quit
		}
	case key.Matches(msg, m.keys.Timeline):
		m.detail.tab = "timeline"
		return m, nil
	case key.Matches(msg, m.keys.Context):
		m.detail.tab = "context"
		return m, nil
	case key.Matches(msg, m.keys.Model):
		m.detail.tab = "model"
		return m, nil
	case key.Matches(msg, m.keys.Turns):
		m.detail.tab = "turns"
		return m, nil
	case key.Matches(msg, m.keys.Charts):
		m.detail.tab = "charts"
		return m, nil
	case key.Matches(msg, m.keys.Usage):
		m.detail.tab = "usage"
		return m, nil
	case key.Matches(msg, m.keys.Subags):
		m.detail.tab = "subagents"
		return m, nil
	case key.Matches(msg, m.keys.Preview):
		m.detail.tab = "preview"
		return m, nil
	case key.Matches(msg, key.NewBinding(key.WithKeys("h"))):
		m.detail.tab = ""
		return m, nil
	}
	var cmd tea.Cmd
	m.detail.view, cmd = m.detail.view.Update(msg)
	return m, cmd
}

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
	m.detail = &detailModel{ctx: m.ctx, app: m.app, db: m.db, sess: s}
}
