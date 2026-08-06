package cli

import (
	"context"
	"os"
	"sort"
	"strings"

	"github.com/talea/talea/internal/i18n"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"golang.org/x/term"

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
		Short: i18n.Tr("interactively pick a session and enter it", "交互式选择会话并进入"),
		Long:  i18n.Tr("Without a session ID, opens an interactive table picker; select and press Enter to resume. With a session ID, resumes directly.", "不带会话 ID 时进入交互式表格选择器，选中后 Enter 恢复；带会话 ID 时直接恢复。"),
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
				return exitError{code: ExitUsage, msg: i18n.Tr("interactive picker needs a terminal; specify a session ID: talea go <session-id>", "交互式选择需要终端，请指定会话 ID：talea go <session-id>")}
			}
			return goSelect(ctx, a)
		},
	}
	cmd.Flags().StringVar(&agentFlag, "agent", "", i18n.Tr("agent ID", "Agent 标识"))
	cmd.Flags().StringVar(&cwdFlag, "cwd", "", i18n.Tr("target directory (override original)", "目标目录（覆盖原目录）"))
	cmd.Flags().BoolVar(&dryRunFlag, "dry-run", false, i18n.Tr("print resume command only", "仅打印恢复命令"))
	return cmd
}

// goModel 是交互式会话选择器。
type goModel struct {
	sess   []*model.Session
	table  table.Model
	help   help.Model
	keys   goKeys
	width  int
	picked int
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
	if err := autoIndex(ctx, a, db); err != nil {
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
	// 选择器按会话结束时间降序排序（最新结束在前）
	sort.SliceStable(sessions, func(i, j int) bool {
		return endTs(sessions[i]) > endTs(sessions[j])
	})

	m := newGoModel(ctx, a, sessions)
	// alt screen：界面全屏且退出时自动恢复终端，避免残留
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return err
	}
	// Bubble Tea 已退出并恢复终端，此时执行选中会话的恢复
	if m.picked >= 0 && m.picked < len(m.sess) {
		return resumeSession(ctx, a, m.sess[m.picked], "", false)
	}
	return nil
}

// goRow 构造选择器表格行（与 list 共享列定义）。
func goRow(s *model.Session) table.Row {
	v := output.ViewOf(s)
	return table.Row(output.SessionRow(v))
}

func newGoModel(ctx context.Context, a *app.App, sessions []*model.Session) *goModel {
	rows := make([][]string, 0, len(sessions))
	for _, s := range sessions {
		rows = append(rows, output.SessionRow(output.ViewOf(s)))
	}
	// 与 list 一致的列宽：bubbles 按固定宽度截断，这里按终端宽度计算
	termW := 120
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 40 {
		termW = w
	}
	widths := output.ColumnWidths(rows, termW)
	cols := output.SessionColumns()
	columns := make([]table.Column, 0, len(cols))
	for i, c := range cols {
		columns = append(columns, table.Column{Title: c.Title, Width: widths[i]})
	}
	tr := make([]table.Row, 0, len(rows))
	for _, r := range rows {
		tr = append(tr, table.Row(r))
	}
	t := table.New(
		table.WithColumns(columns),
		table.WithRows(tr),
		table.WithFocused(true),
		table.WithHeight(10),
	)
	s := table.DefaultStyles()
	s.Header = s.Header.BorderStyle(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("240")).BorderBottom(true).Bold(false)
	s.Selected = s.Selected.Foreground(lipgloss.Color("229")).Background(lipgloss.Color("57")).Bold(false)
	t.SetStyles(s)

	km := goKeys{
		Quit:  key.NewBinding(key.WithKeys("q", "esc", "ctrl+c"), key.WithHelp("q", i18n.Tr("quit", "退出"))),
		Enter: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", i18n.Tr("enter session", "进入会话"))),
	}
	return &goModel{
		sess:   sessions,
		table:  t,
		help:   help.New(),
		keys:   km,
		picked: -1,
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
				m.picked = idx
				return m, tea.Quit
			}
		}
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m *goModel) View() string {
	var sb strings.Builder
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).Render(i18n.Tr("Talea · pick a session and press Enter", "Talea · 选择会话后进入（Enter）")) + "\n\n")
	sb.WriteString(m.table.View())
	sb.WriteString("\n" + m.help.View(m.keys))
	return sb.String()
}

// endTs 返回会话结束时间的 Unix 秒（无结束时间用开始时间兜底）。
func endTs(s *model.Session) int64 {
	if s.EndedAt != nil {
		return s.EndedAt.Unix()
	}
	if s.StartedAt != nil {
		return s.StartedAt.Unix()
	}
	if s.LastActivityAt != nil {
		return s.LastActivityAt.Unix()
	}
	return 0
}
