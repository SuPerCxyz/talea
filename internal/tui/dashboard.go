package tui

import (
	"context"
	"path/filepath"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/i18n"
	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
	"github.com/talea/talea/internal/status"
	"github.com/talea/talea/internal/syncer"
)

const dashboardRefreshInterval = 3 * time.Second

// dashboardModel 是常驻状态看板。
type dashboardModel struct {
	ctx        context.Context
	app        *app.App
	db         *index.DB
	eventsPath string
	snapshot   status.Snapshot
	rows       []boardRow

	cursor    int
	offset    int
	width     int
	height    int
	filter    textinput.Model
	filtering bool

	detail *detailModel
	picked *model.Session

	refreshing bool
	loading    bool
	loadErr    error
	refreshErr error
	spinner    spinner.Model
	keys       dashKeys
}

type dashKeys struct {
	Quit   key.Binding
	Enter  key.Binding
	Detail key.Binding
	Open   key.Binding
	Back   key.Binding
	Filter key.Binding
	Up     key.Binding
	Down   key.Binding
	PgUp   key.Binding
	PgDn   key.Binding
	Home   key.Binding
	End    key.Binding
}

func newDashKeys() dashKeys {
	return dashKeys{
		Quit:   key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", i18n.Tr("quit", "退出"))),
		Enter:  key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", i18n.Tr("details", "详情"))),
		Detail: key.NewBinding(key.WithKeys("d"), key.WithHelp("d", i18n.Tr("details", "详情"))),
		Open:   key.NewBinding(key.WithKeys("o"), key.WithHelp("o", i18n.Tr("resume", "恢复"))),
		Back:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", i18n.Tr("back/clear", "返回/清除"))),
		Filter: key.NewBinding(key.WithKeys("/"), key.WithHelp("/", i18n.Tr("filter", "过滤"))),
		Up:     key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", i18n.Tr("up", "上移"))),
		Down:   key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", i18n.Tr("down", "下移"))),
		PgUp:   key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup", i18n.Tr("page up", "上页"))),
		PgDn:   key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgdn", i18n.Tr("page down", "下页"))),
		Home:   key.NewBinding(key.WithKeys("home", "g"), key.WithHelp("home", i18n.Tr("top", "顶部"))),
		End:    key.NewBinding(key.WithKeys("end", "G"), key.WithHelp("end", i18n.Tr("bottom", "底部"))),
	}
}

func (k dashKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Enter, k.Detail, k.Open, k.Filter, k.Up, k.Down, k.Back, k.Quit}
}

type dashboardTickMsg struct{}
type dashboardRefreshedMsg struct {
	snapshot *status.Snapshot
	err      error
}

// RunDashboard 启动常驻看板；退出后按选择执行恢复流程。
func RunDashboard(ctx context.Context) error {
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
	eventsPath := filepath.Join(a.Paths.DataDir, status.EventFileName)
	snap, buildErr := status.Build(ctx, db, status.Options{
		EventsPath:       eventsPath,
		IncludeSubagents: includeSubagents(a),
	})
	m := newDashboard(ctx, a, db, eventsPath, snap, buildErr)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, runErr := p.Run()
	if m.picked != nil {
		if rerr := resumeSession(ctx, a, m.picked, "", false); rerr != nil {
			if runErr == nil {
				runErr = rerr
			}
		}
	}
	return runErr
}

func newDashboard(ctx context.Context, a *app.App, db *index.DB, eventsPath string, snap status.Snapshot, loadErr error) *dashboardModel {
	ti := textinput.New()
	ti.Prompt = "/ "
	ti.Placeholder = i18n.Tr("filter agent/project/session", "过滤 Agent/项目/会话")
	ti.CharLimit = 64
	ti.Width = 74

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#5b2a86", Dark: "#ffd166"})

	m := &dashboardModel{
		ctx:        ctx,
		app:        a,
		db:         db,
		eventsPath: eventsPath,
		snapshot:   snap,
		width:      100,
		height:     30,
		filter:     ti,
		spinner:    sp,
		keys:       newDashKeys(),
		loadErr:    loadErr,
	}
	m.rebuild()
	return m
}

// Init 立即触发一次增量同步，并按固定周期续订刷新。
func (m *dashboardModel) Init() tea.Cmd {
	m.loading = len(m.snapshot.Sessions) == 0
	m.refreshing = true
	return tea.Batch(m.spinner.Tick, m.refreshCmd(), dashboardTickCmd())
}

func dashboardTickCmd() tea.Cmd {
	return tea.Tick(dashboardRefreshInterval, func(time.Time) tea.Msg {
		return dashboardTickMsg{}
	})
}

// refreshCmd 执行增量同步并重建快照；刷新期间的重叠 tick 由 refreshing 跳过。
func (m *dashboardModel) refreshCmd() tea.Cmd {
	return func() tea.Msg {
		report, err := syncer.SyncWithReport(m.ctx, m.app, m.db, nil)
		if err != nil {
			return dashboardRefreshedMsg{err: err}
		}
		snap, berr := status.Build(m.ctx, m.db, status.Options{
			EventsPath:       m.eventsPath,
			IncludeSubagents: includeSubagents(m.app),
		})
		if berr != nil {
			return dashboardRefreshedMsg{err: berr}
		}
		snap.Warnings = append(snap.Warnings, report.Warnings()...)
		return dashboardRefreshedMsg{snapshot: &snap}
	}
}

// Update 处理看板消息。
func (m *dashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			m.width = msg.Width
		}
		if msg.Height > 0 {
			m.height = msg.Height
		}
		if m.width > 8 {
			m.filter.Width = m.width - 6
		}
		if m.detail != nil {
			m.detail.width = m.width
			m.detail.height = m.height - 2
			m.detail.view.Width = m.width
			m.detail.view.Height = m.height - 2
			m.detail.contentValid = false
		}
		m.ensureVisible()
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if m.refreshing || m.loading {
			return m, cmd
		}
		return m, nil
	case dashboardTickMsg:
		var cmd tea.Cmd
		if !m.refreshing {
			m.refreshing = true
			cmd = m.refreshCmd()
		}
		return m, tea.Batch(cmd, dashboardTickCmd())
	case dashboardRefreshedMsg:
		return m, m.handleRefreshed(msg)
	case tea.KeyMsg:
		if m.detail != nil {
			return m.handleDetailKey(msg)
		}
		if m.filtering {
			return m.handleFilterKey(msg)
		}
		return m.handleBoardKey(msg)
	}
	return m, nil
}

func (m *dashboardModel) handleRefreshed(msg dashboardRefreshedMsg) tea.Cmd {
	m.refreshing = false
	m.loading = false
	if msg.err != nil {
		if len(m.snapshot.Sessions) == 0 {
			m.loadErr = msg.err
		} else {
			m.refreshErr = msg.err
		}
		return nil
	}
	key := ""
	if ss, ok := selectedBoardSession(m.rows, m.cursor); ok {
		key = boardSessionKey(ss)
	}
	m.snapshot = *msg.snapshot
	m.loadErr = nil
	m.refreshErr = nil
	m.rebuild()
	m.selectByKey(key)
	return nil
}
