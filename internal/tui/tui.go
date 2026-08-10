// Package tui 实现 Bubble Tea 终端界面。
package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/cli/output"
	"github.com/talea/talea/internal/i18n"
	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
	"github.com/talea/talea/internal/resume"
	"github.com/talea/talea/internal/search"
	"github.com/talea/talea/internal/timeline"
)

// 高对比度配色：统一使用 AdaptiveColor 适配深浅终端背景，
// 深色背景取亮色前景，浅色背景取深色前景，避免默认灰色系对比度不足。
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#C2185B", Dark: "#FF9E80"})
)

// newListDelegate 返回高对比度的列表项样式（每项 3 行：agent+usage / 首问 / 最近消息）。
func newListDelegate() list.DefaultDelegate {
	d := list.NewDefaultDelegate()
	d.SetHeight(3)
	s := d.Styles

	normalFG := lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#f0f0f0"}
	normalDescFG := lipgloss.AdaptiveColor{Light: "#4a4a4a", Dark: "#b0b0b0"}
	selectedFG := lipgloss.AdaptiveColor{Light: "#ffffff", Dark: "#000000"}
	selectedBG := lipgloss.AdaptiveColor{Light: "#5b2a86", Dark: "#ffd166"}
	selectedDescFG := lipgloss.AdaptiveColor{Light: "#f3e8ff", Dark: "#1a1a1a"}

	s.NormalTitle = lipgloss.NewStyle().
		Foreground(normalFG).
		Padding(0, 0, 0, 2)
	s.NormalDesc = lipgloss.NewStyle().
		Foreground(normalDescFG).
		Padding(0, 0, 0, 2)
	s.SelectedTitle = lipgloss.NewStyle().
		Foreground(selectedFG).
		Background(selectedBG).
		Bold(true).
		Padding(0, 0, 0, 2)
	s.SelectedDesc = lipgloss.NewStyle().
		Foreground(selectedDescFG).
		Background(selectedBG).
		Padding(0, 0, 0, 2)
	s.DimmedTitle = lipgloss.NewStyle().
		Foreground(normalDescFG).
		Padding(0, 0, 0, 2)
	s.DimmedDesc = lipgloss.NewStyle().
		Foreground(normalDescFG).
		Padding(0, 0, 0, 2)
	s.FilterMatch = lipgloss.NewStyle().Underline(true)
	d.Styles = s
	return d
}

// newListStyles 返回高对比度的列表容器样式（标题栏 / 状态栏 / 分页）。
func newListStyles() list.Styles {
	st := list.DefaultStyles()
	subFG := lipgloss.AdaptiveColor{Light: "#2a2a2a", Dark: "#c8c8c8"}
	st.StatusBar = lipgloss.NewStyle().
		Foreground(subFG).
		Padding(0, 0, 1, 2)
	st.PaginationStyle = lipgloss.NewStyle().PaddingLeft(2)
	st.HelpStyle = lipgloss.NewStyle().Padding(1, 0, 0, 2)
	st.ActivePaginationDot = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#5b2a86", Dark: "#ffd166"}).
		SetString(bullet)
	st.InactivePaginationDot = lipgloss.NewStyle().
		Foreground(subFG).
		SetString(bullet)
	st.DividerDot = lipgloss.NewStyle().
		Foreground(subFG).
		SetString(" " + bullet + " ")
	return st
}

const bullet = "•"

// loadTuiSessions 加载 TUI 会话列表，dir 非空时仅保留该目录前缀下的会话。
// 固定按结束时间倒序排列（最新结束在前），不受配置 default_sort 影响，
// 与 talea list / talea go 保持一致。
// 返回会话列表及对应的 usage 汇总（key=agent_instance_id\x00session_id）。
func loadTuiSessions(ctx context.Context, a *app.App, db *index.DB, dir string) ([]*model.Session, map[string]timeline.SessionUsageRow, error) {
	results, err := search.List(ctx, db, search.Query{Cwd: dir, Limit: 500})
	if err != nil {
		return nil, nil, err
	}
	sessions := make([]*model.Session, 0, len(results))
	for i := range results {
		sessions = append(sessions, &results[i].Session)
	}
	a.ResolveWorkingDirs(ctx, sessions)
	sort.SliceStable(sessions, func(i, j int) bool {
		return endTs(sessions[i]) > endTs(sessions[j])
	})
	// 批量填充最近一次用户消息与 usage（供 TUI 展示）
	fillLastUserPrompts(ctx, db, sessions)
	usages := fillUsages(ctx, db, sessions)
	return sessions, usages, nil
}

// fillUsages 批量查询会话的 Token 汇总（含缓存字段）。
func fillUsages(ctx context.Context, db *index.DB, sessions []*model.Session) map[string]timeline.SessionUsageRow {
	if db == nil || len(sessions) == 0 {
		return nil
	}
	keys := make([][2]string, 0, len(sessions))
	seen := map[string]bool{}
	for _, s := range sessions {
		if s.SessionID == "" {
			continue
		}
		key := s.AgentInstanceID + "\x00" + s.SessionID
		if seen[key] {
			continue
		}
		seen[key] = true
		keys = append(keys, [2]string{s.AgentInstanceID, s.SessionID})
	}
	usages, err := timeline.UsageBySession(ctx, db, keys)
	if err != nil {
		return nil
	}
	return usages
}

// fillLastUserPrompts 批量查询会话的最后一次用户消息并写入 LastUserPrompt。
func fillLastUserPrompts(ctx context.Context, db *index.DB, sessions []*model.Session) {
	if db == nil || len(sessions) == 0 {
		return
	}
	keys := make([][2]string, 0, len(sessions))
	seen := map[string]int{}
	for _, s := range sessions {
		if s.SessionID == "" {
			continue
		}
		key := s.AgentInstanceID + "\x00" + s.SessionID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = len(keys)
		keys = append(keys, [2]string{s.AgentInstanceID, s.SessionID})
	}
	prompts, err := timeline.LastUserPromptBySession(ctx, db, keys)
	if err != nil {
		return
	}
	for _, s := range sessions {
		if s.SessionID == "" {
			continue
		}
		key := s.AgentInstanceID + "\x00" + s.SessionID
		if p, ok := prompts[key]; ok {
			s.LastUserPrompt = p
		}
	}
}

// endTs 返回会话结束时间的 Unix 秒（无结束时间依次用开始时间、最后活动时间兜底）。
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

// Run 启动 TUI。
// dir 非空时仅列出该目录前缀下的会话。
func Run(ctx context.Context, dir string) error {
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
	// FTS 表同步（快），不阻塞等待增量索引
	if err := search.Ensure(ctx, db); err != nil {
		return err
	}
	if err := search.Populate(ctx, db); err != nil {
		return err
	}
	sessions, usages, err := loadTuiSessions(ctx, a, db, dir)
	if err != nil {
		return err
	}

	m := newMain(ctx, a, sessions, usages, db, dir)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	// TUI 已退出并恢复终端，此时再恢复会话，避免在 alt screen 内
	// 嵌套启动 agent 导致退出后终端错位/光标丢失
	if m.picked != nil {
		if rerr := resumeSession(ctx, a, m.picked, "", false); rerr != nil {
			fmt.Fprintln(os.Stderr, "talea:", rerr)
			if err == nil {
				err = rerr
			}
		}
	}
	return err
}

// resumeSession 执行会话恢复（复用 cli 包逻辑）。
func resumeSession(ctx context.Context, a *app.App, s *model.Session, cwd string, dryRun bool) error {
	plan, err := resume.Build(*s, cwd, a.Config.PathMapping)
	if err != nil {
		return err
	}
	if !plan.DirExists {
		return fmt.Errorf(i18n.Tr("original working directory missing: %s\ntry: talea go %s --cwd <new dir>", "原工作目录不存在：%s\n可以执行：talea go %s --cwd <新目录>"), plan.TargetDir, s.SessionID)
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

// doResume 记录选中的会话并退出 TUI（Run 返回后恢复，终端已清理）。
func (m *mainModel) doResume(s *model.Session) tea.Cmd {
	m.picked = s
	return tea.Quit
}

// mainModel 是主 TUI 模型。
type mainModel struct {
	ctx      context.Context
	app      *app.App
	db       *index.DB
	dir      string
	sessions []*model.Session
	usages   map[string]timeline.SessionUsageRow
	list     list.Model
	detail   *detailModel
	keys     keyMap
	help     help.Model
	width    int
	height   int
	picked   *model.Session
	indexing bool
	indexErr error
}

type keyMap struct {
	Open   key.Binding
	Quit   key.Binding
	Enter  key.Binding
	Detail key.Binding
	Back   key.Binding
	Turns  key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Enter, k.Detail, k.Open, k.Back, k.Turns, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Enter, k.Detail, k.Open, k.Back, k.Turns, k.Quit}}
}

type item struct {
	title string
	desc  string
	sess  *model.Session
	usage timeline.SessionUsageRow
	hasUs bool
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string {
	return i.sess.FirstQuestion + " " + string(i.sess.AgentID) + " " +
		i.sess.SessionID + " " + i.sess.WorkingDirectory
}

func newMain(ctx context.Context, a *app.App, sessions []*model.Session, usages map[string]timeline.SessionUsageRow, db *index.DB, dir string) *mainModel {
	items := itemsOf(sessions, usages)
	l := list.New(items, newListDelegate(), 0, 0)
	l.Title = "Talea · Agent Sessions"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.Styles = newListStyles()

	km := keyMap{
		Open:   key.NewBinding(key.WithKeys("o"), key.WithHelp("o", i18n.Tr("resume session", "恢复会话"))),
		Quit:   key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", i18n.Tr("quit", "退出"))),
		Enter:  key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", i18n.Tr("open session", "进入会话"))),
		Detail: key.NewBinding(key.WithKeys("d"), key.WithHelp("d", i18n.Tr("details", "详情"))),
		Back:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", i18n.Tr("back", "返回"))),
		Turns:  key.NewBinding(key.WithKeys("t"), key.WithHelp("t", i18n.Tr("user turns", "用户轮次"))),
	}
	return &mainModel{
		ctx:      ctx,
		app:      a,
		db:       db,
		dir:      dir,
		sessions: sessions,
		usages:   usages,
		list:     l,
		keys:     km,
		help:     help.New(),
	}
}

// itemsOf 构造列表项。usages 为可选的会话 usage 映射（key=instance\x00session）。
func itemsOf(sessions []*model.Session, usages map[string]timeline.SessionUsageRow) []list.Item {
	items := make([]list.Item, 0, len(sessions))
	for _, s := range sessions {
		it := item{
			sess: s,
		}
		if usages != nil {
			if u, ok := usages[s.AgentInstanceID+"\x00"+s.SessionID]; ok {
				it.usage = u
				it.hasUs = true
			}
		}
		it.title = itemTitle(it)
		it.desc = itemDesc(it)
		items = append(items, it)
	}
	return items
}

// indexOf 返回匹配会话 ID 的项下标，未找到返回 -1。
func indexOf(items []list.Item, id string) int {
	for i, it := range items {
		if item, ok := it.(item); ok && item.sess.SessionID == id {
			return i
		}
	}
	return -1
}

// itemTitle 渲染列表第一行：agent + 时间 + 使用情况（Token / 缓存命中率）。
func itemTitle(it item) string {
	s := it.sess
	agent := displayAgent(s.AgentID)
	timeStr := ""
	if s.StartedAt != nil {
		timeStr = s.StartedAt.Format("01-02 15:04")
	}
	var sb strings.Builder
	sb.WriteString("[")
	sb.WriteString(padDisplay(agent, 10))
	sb.WriteString("] ")
	sb.WriteString(timeStr)
	if it.hasUs {
		sb.WriteString("  ")
		sb.WriteString(i18n.Tr("Token", "Token"))
		sb.WriteString(" ")
		sb.WriteString(humanNum(valOr(it.usage.TotalTokens)))
		if rate := timeline.CacheHitRate(it.usage); rate >= 0 {
			sb.WriteString("  ")
			sb.WriteString(i18n.Tr("Cache", "缓存"))
			sb.WriteString(" ")
			sb.WriteString(fmt.Sprintf("%.0f%%", rate*100))
		}
	}
	return sb.String()
}

// itemDesc 渲染列表第二、三行：首次提问与最近一次用户消息。
func itemDesc(it item) string {
	s := it.sess
	first := firstLine(s.FirstQuestion)
	if first == "" {
		first = i18n.Tr("No valid user question detected", "未识别到有效用户提问")
	}
	lines := []string{
		i18n.Tr("Q: ", "问：") + truncRunes(first, 100),
	}
	if s.LastUserPrompt != "" {
		lines = append(lines, i18n.Tr("Last: ", "最近：")+truncRunes(firstLine(s.LastUserPrompt), 100))
	}
	return strings.Join(lines, "\n")
}

func valOr(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

// padDisplay 左对齐填充到指定显示宽度。
func padDisplay(s string, w int) string {
	if runewidth.StringWidth(s) >= w {
		return s
	}
	return s + strings.Repeat(" ", w-runewidth.StringWidth(s))
}

func displayAgent(a model.AgentID) string {
	return output.DisplayAgent(a)
}

// Init 初始化模型。
func (m *mainModel) Init() tea.Cmd {
	// 后台增量索引，完成后刷新列表（不阻塞启动）
	m.indexing = true
	return m.runIndex
}

// runIndex 后台执行增量索引并刷新列表。
func (m *mainModel) runIndex() tea.Msg {
	_, err := (&index.Indexer{App: m.app, DB: m.db}).Run(m.ctx)
	if err == nil {
		err = search.Ensure(m.ctx, m.db)
	}
	if err == nil {
		err = search.Populate(m.ctx, m.db)
	}
	m.indexing = false
	m.indexErr = err
	return indexedMsg{}
}

// indexedMsg 通知索引完成。
type indexedMsg struct{}

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
			m.detail.contentValid = false
		}
		return m, nil
	case indexedMsg:
		// 索引完成，刷新会话列表
		if m.indexErr != nil {
			return m, nil
		}
		if m.detail != nil {
			return m, nil
		}
		sessions, usages, err := loadTuiSessions(m.ctx, m.app, m.db, m.dir)
		if err != nil {
			return m, nil
		}
		sel := ""
		if it, ok := m.list.SelectedItem().(item); ok {
			sel = it.sess.SessionID
		}
		m.sessions = sessions
		m.usages = usages
		m.list.SetItems(itemsOf(sessions, usages))
		if sel != "" {
			if idx := indexOf(itemsOf(sessions, usages), sel); idx >= 0 {
				m.list.Select(idx)
			}
		}
		return m, nil
	case tea.KeyMsg:
		if m.detail != nil {
			return m.handleDetailKey(msg)
		}
		// 过滤输入中（Filtering）：按键应输入到过滤器，enter 应用过滤，
		// 不得触发功能键；过滤已应用（FilterApplied）后恢复功能键可进入会话
		if m.list.FilterState() == list.Filtering {
			var fcmd tea.Cmd
			m.list, fcmd = m.list.Update(msg)
			return m, fcmd
		}
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Open), key.Matches(msg, m.keys.Enter):
			if it, ok := m.list.SelectedItem().(item); ok {
				return m, m.doResume(it.sess)
			}
		case key.Matches(msg, m.keys.Detail):
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
			return m, m.doResume(m.detail.sess)
		}
	case key.Matches(msg, m.keys.Turns):
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

// showDetail 进入详情页。
func (m *mainModel) showDetail(s *model.Session) {
	w, h := m.width, m.height
	if w <= 0 {
		w = 100
	}
	if h <= 0 {
		h = 30
	}
	m.detail = &detailModel{ctx: m.ctx, app: m.app, db: m.db, sess: s, width: w, height: h - 2}
}
