package cli

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/cli/output"
	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
	"github.com/talea/talea/internal/search"
)

func newGoCmd() *cobra.Command {
	var (
		agentFlag  string
		cwdFlag    string
		dryRunFlag bool
	)
	cmd := &cobra.Command{
		Use:   "go [session-id]",
		Short: "交互式选择会话并进入",
		Long:  "不带会话 ID 时进入交互式表格选择器，选中后 Enter 恢复；带会话 ID 时直接恢复。",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			a, err := app.New(ctx)
			if err != nil {
				return err
			}
			if len(args) == 1 {
				sess, err := findSession(ctx, a, args[0], agentFlag)
				if err != nil {
					return err
				}
				return resumeSession(ctx, a, sess, cwdFlag, dryRunFlag)
			}
			if !isTTY(os.Stdin) {
				return exitError{code: ExitUsage, msg: "交互式选择需要终端，请指定会话 ID：talea go <session-id>"}
			}
			return goSelect(ctx, a)
		},
	}
	cmd.Flags().StringVar(&agentFlag, "agent", "", "Agent 标识")
	cmd.Flags().StringVar(&cwdFlag, "cwd", "", "目标目录（覆盖原目录）")
	cmd.Flags().BoolVar(&dryRunFlag, "dry-run", false, "仅打印恢复命令")
	return cmd
}

// goModel 是交互式会话选择器。
type goModel struct {
	ctx   context.Context
	app   *app.App
	sess  []*model.Session
	table table.Model
	help  help.Model
	keys  goKeys
	width int
}

type goKeys struct {
	Quit  key.Binding
	Enter key.Binding
}

func (k goKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Enter, k.Quit}
}

func (k goKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Enter, k.Quit}}
}

// goSelect 启动交互式选择器，选中后恢复会话。
func goSelect(ctx context.Context, a *app.App) error {
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
	results, err := search.List(ctx, db, search.Query{Limit: 200})
	if err != nil {
		return err
	}
	sessions := make([]*model.Session, 0, len(results))
	for i := range results {
		sessions = append(sessions, &results[i].Session)
	}
	a.ResolveWorkingDirs(ctx, sessions)
	a.SortSessions(sessions)

	m := newGoModel(ctx, a, sessions)
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		return err
	}
	return nil
}

// goRow 构造选择器表格行。
func goRow(s *model.Session) table.Row {
	return table.Row{
		string(s.AgentID),
		s.SessionID,
		goTime(s.StartedAt),
		output.FormatTokens(s.TokenUsage),
		shortHome(s.WorkingDirectory),
		firstLine(s.FirstQuestion),
	}
}

func newGoModel(ctx context.Context, a *app.App, sessions []*model.Session) *goModel {
	columns := []table.Column{
		{Title: "Agent", Width: 12},
		{Title: "Session ID", Width: 30},
		{Title: "Time", Width: 12},
		{Title: "Tokens", Width: 9},
		{Title: "CWD", Width: 24},
		{Title: "First Question", Width: 80},
	}
	rows := make([]table.Row, 0, len(sessions))
	for _, s := range sessions {
		rows = append(rows, goRow(s))
	}
	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(10),
	)
	s := table.DefaultStyles()
	s.Header = s.Header.BorderStyle(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("240")).BorderBottom(true).Bold(false)
	s.Selected = s.Selected.Foreground(lipgloss.Color("229")).Background(lipgloss.Color("57")).Bold(false)
	t.SetStyles(s)

	km := goKeys{
		Quit:  key.NewBinding(key.WithKeys("q", "esc", "ctrl+c"), key.WithHelp("q", "退出")),
		Enter: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "进入会话")),
	}
	return &goModel{
		ctx:   ctx,
		app:   a,
		sess:  sessions,
		table: t,
		help:  help.New(),
		keys:  km,
	}
}

func (m *goModel) Init() tea.Cmd { return nil }

func (m *goModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.table.SetWidth(msg.Width)
		m.table.SetHeight(msg.Height - 4)
		m.help.Width = msg.Width
		return m, nil
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Enter):
			idx := m.table.Cursor()
			if idx >= 0 && idx < len(m.sess) {
				return m, func() tea.Msg {
					if err := resumeSession(m.ctx, m.app, m.sess[idx], "", false); err != nil {
						return goErrMsg{err: err}
					}
					return goDoneMsg{}
				}
			}
		}
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

type goDoneMsg struct{}

type goErrMsg struct{ err error }

func (m *goModel) View() string {
	var sb strings.Builder
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).Render("Talea · 选择会话后进入（Enter）") + "\n\n")
	sb.WriteString(m.table.View())
	sb.WriteString("\n" + m.help.View(m.keys))
	return sb.String()
}

func goTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("01-02 15:04")
}

func shortHome(d string) string {
	if d == "" {
		return ""
	}
	if strings.HasPrefix(d, "/home/") {
		rest := strings.TrimPrefix(d, "/home/")
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) == 2 {
			return "~/" + parts[1]
		}
		return "~"
	}
	return d
}
