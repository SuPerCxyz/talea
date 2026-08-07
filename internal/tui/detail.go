package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/chart"
	"github.com/talea/talea/internal/i18n"
	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
	"github.com/talea/talea/internal/timeline"
	"github.com/talea/talea/internal/usage"
)

var (
	labelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#e0e0e0"})
	valueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#3b3b3b", Dark: "#ffffff"})
	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#6a6a6a", Dark: "#9e9e9e"})
)

// detailModel 是会话详情视图。
type detailModel struct {
	ctx    context.Context
	app    *app.App
	db     *index.DB
	sess   *model.Session
	view   viewport.Model
	ready  bool
	width  int
	height int

	// 用户轮次默认折叠，按 t 展开
	turnsVisible bool
	turnsCount   int
	turnsLoaded  bool
}

// render 渲染详情页全部内容（聚合展示，viewport 滚动查看）。
func (d *detailModel) render() string {
	if d.sess == nil {
		return i18n.Tr("No session data", "会话数据为空")
	}
	var sb strings.Builder
	sb.WriteString(d.renderDetail())
	sb.WriteString("\n\n" + titleStyle.Render(i18n.Tr("Context Window Curve", "上下文窗口曲线")) + "\n")
	sb.WriteString(sectionOr(d.renderContext()))
	sb.WriteString("\n\n" + titleStyle.Render(i18n.Tr("Models Summary", "按模型汇总")) + "\n")
	sb.WriteString(sectionOr(d.renderModel()))
	sb.WriteString("\n\n" + titleStyle.Render(i18n.Tr("Token Chart", "Token 图表")) + "\n")
	sb.WriteString(sectionOr(d.renderCharts()))
	sb.WriteString("\n\n" + titleStyle.Render(i18n.Tr("Token Summary", "Token 汇总")) + "\n")
	sb.WriteString(sectionOr(d.renderUsageSummary()))
	sb.WriteString("\n\n" + titleStyle.Render(i18n.Tr("Sub-Agent Sessions", "子 Agent 会话")) + "\n")
	sb.WriteString(sectionOr(d.renderSubagents()))
	sb.WriteString("\n\n" + labelStyle.Render(d.keyHint()))
	return sb.String()
}

// keyHint 详情页底部按键提示。
func (d *detailModel) keyHint() string {
	base := i18n.Tr("keys: o resume  esc/q back  ↑/↓ scroll", "按键：o 恢复  esc/q 返回列表  ↑/↓ 滚动")
	if d.turnsCount == 0 {
		return base
	}
	if d.turnsVisible {
		return i18n.Tr("keys: o resume  t hide turns  esc/q back  ↑/↓ scroll", "按键：o 恢复  t 收起轮次  esc/q 返回列表  ↑/↓ 滚动")
	}
	return i18n.Tr("keys: o resume  t show turns  esc/q back  ↑/↓ scroll", "按键：o 恢复  t 显示轮次  esc/q 返回列表  ↑/↓ 滚动")
}

// sectionOr 返回子渲染内容；剥离其自带标题行，nil 场景返回降级提示。
func sectionOr(s string) string {
	if s == "" {
		return i18n.Tr("(no data)", "（无数据）") + "\n"
	}
	// 错误/空数据消息直接展示
	if strings.HasPrefix(s, "db unavailable") || strings.HasPrefix(s, "加载") ||
		strings.HasPrefix(s, "该会话") || strings.HasPrefix(s, "没有") ||
		strings.HasPrefix(s, "会话格式") || strings.HasPrefix(s, "load") {
		return s + "\n"
	}
	// 剥离首行标题（聚合时已加标题）
	if i := strings.Index(s, "\n"); i >= 0 {
		return s[i+1:] + "\n"
	}
	return s + "\n"
}

// renderContext 渲染上下文窗口曲线（面积曲线图，带 y 轴与时间轴）。
func (d *detailModel) renderContext() string {
	if d.db == nil {
		return i18n.Tr("Database unavailable", "数据库不可用")
	}
	pts, err := timeline.ContextCurve(d.ctx, d.db, d.sess.AgentInstanceID, d.sess.SessionID, 0)
	if err != nil {
		return i18n.Trf("Failed to load context curve: %v", "加载上下文曲线失败：%v", err)
	}
	if len(pts) == 0 {
		return i18n.Tr("No context window data", "没有上下文窗口数据")
	}
	comps, _ := timeline.DetectCompactions(d.ctx, d.db, d.sess.AgentInstanceID, d.sess.SessionID)
	var sb strings.Builder
	sb.WriteString(labelStyle.Render(i18n.Tr("Context window (tokens):", "上下文窗口（Token）：")) + "\n")
	vals := make([]float64, len(pts))
	labels := make([]string, len(pts))
	for i, p := range pts {
		vals[i] = float64(p.Context)
		labels[i] = time.Unix(p.Timestamp, 0).Format("15:04")
	}
	plotW := d.width - 8
	if plotW < 30 {
		plotW = 30
	}
	sb.WriteString(chart.Area(vals, labels, 9, 0, plotW) + "\n")
	// 起点/峰值/终点标注
	sb.WriteString(fmt.Sprintf("%s %s  %s %s  %s %s\n",
		i18n.Tr("start", "起点"), humanNum(pts[0].Context),
		i18n.Tr("peak", "峰值"), humanNum(int64(peakVal(vals))),
		i18n.Tr("end", "终点"), humanNum(pts[len(pts)-1].Context)))
	if len(comps) > 0 {
		sb.WriteString("\n" + i18n.Tr("Context compaction:", "上下文压缩：") + "\n")
		for _, c := range comps {
			label := i18n.Tr("explicit compaction", "明确压缩")
			if c.IsInferred {
				label = i18n.Tr("possible compaction", "可能发生上下文压缩")
			}
			sb.WriteString(fmt.Sprintf("  %s → %s（%s）\n", humanNum(c.Before), humanNum(c.After), label))
		}
	}
	return sb.String()
}

// peakVal 返回浮点切片最大值。
func peakVal(v []float64) float64 {
	m := v[0]
	for _, x := range v[1:] {
		if x > m {
			m = x
		}
	}
	return m
}

// renderModel 渲染模型汇总。
func (d *detailModel) renderModel() string {
	if d.db == nil {
		return i18n.Tr("Database unavailable", "数据库不可用")
	}
	sums, err := timeline.ByModel(d.ctx, d.db, d.sess.AgentInstanceID, d.sess.SessionID)
	if err != nil {
		return i18n.Trf("Failed to load model summary: %v", "加载模型汇总失败：%v", err)
	}
	if len(sums) == 0 {
		return i18n.Tr("This session has no model data", "该会话没有模型数据")
	}
	var sb strings.Builder
	sb.WriteString(titleStyle.Render(i18n.Tr("Models Summary", "按模型汇总")) + "\n\n")
	sb.WriteString(fmt.Sprintf("%-24s  %-8s  %-10s  %-10s  %-10s\n",
		i18n.Tr("Model", "模型"), i18n.Tr("Req", "请求"),
		i18n.Tr("Input", "输入"), i18n.Tr("Output", "输出"), i18n.Tr("Total", "总计")))
	for _, m := range sums {
		sb.WriteString(fmt.Sprintf("%-24s  %-8d  %-10s  %-10s  %-10s\n",
			m.Model, m.Requests, humanNum(m.InputTokens), humanNum(m.OutputTokens), humanNum(m.TotalTokens)))
	}
	return sb.String()
}

// renderTurnsTable 渲染用户轮次表格（无标题，供聚合/详情嵌入）。
func (d *detailModel) renderTurnsTable() string {
	if d.db == nil {
		return i18n.Tr("Database unavailable", "数据库不可用") + "\n"
	}
	turns, err := timeline.GroupByTurns(d.ctx, d.db, d.sess.AgentInstanceID, d.sess.SessionID)
	if err != nil {
		return i18n.Trf("Failed to load turns: %v", "加载轮次失败：%v", err) + "\n"
	}
	if len(turns) == 0 {
		return i18n.Tr("This session has no aggregable turns", "该会话没有可聚合的轮次") + "\n"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%-5s  %-40s  %-6s  %-10s\n",
		i18n.Tr("Turn", "轮次"), i18n.Tr("Question", "提问"),
		i18n.Tr("Req", "请求"), i18n.Tr("Token", "Token")))
	for _, t := range turns {
		sb.WriteString(fmt.Sprintf("%-5d  %-40s  %-6d  %-10s\n",
			t.Index, truncRunes(t.Prompt, 38), t.Requests, humanNum(t.Total)))
	}
	return sb.String()
}

// renderCharts 渲染终端图表（速率柱状图，带 y 轴）。
func (d *detailModel) renderCharts() string {
	if d.db == nil {
		return i18n.Tr("Database unavailable", "数据库不可用")
	}
	buckets, err := timeline.GroupByBucket(d.ctx, d.db, d.sess.AgentInstanceID, d.sess.SessionID, timeline.Bucket5m)
	if err != nil {
		return i18n.Trf("Failed to load chart: %v", "加载图表失败：%v", err)
	}
	if len(buckets) == 0 {
		return i18n.Tr("This session has no aggregable data", "该会话没有可聚合的数据")
	}
	var sb strings.Builder
	sb.WriteString(titleStyle.Render(i18n.Tr("Token Chart", "Token 图表")) + "\n\n")

	vals := make([]float64, len(buckets))
	labels := make([]string, len(buckets))
	for i, b := range buckets {
		vals[i] = float64(b.TotalTokens) / 5.0
		labels[i] = b.Start.Format("15:04")
	}
	sb.WriteString(labelStyle.Render(i18n.Tr("Tokens/min (5-min buckets):", "Token/分钟（每 5 分钟桶）：")) + "\n")
	maxCols := (d.width - 10) / 2
	if maxCols < 20 {
		maxCols = 20
	}
	sb.WriteString(chart.BarW(vals, labels, 6, maxCols) + "\n")
	return sb.String()
}

// renderUsageSummary 渲染 Token 汇总。
func (d *detailModel) renderUsageSummary() string {
	if d.db == nil {
		return i18n.Tr("Database unavailable", "数据库不可用")
	}
	u, err := usage.Load(d.ctx, d.db, d.sess.AgentInstanceID, d.sess.SessionID)
	if err != nil || u == nil {
		return i18n.Tr("This agent session format has no token usage data", "当前 Agent 会话格式未提供 Token 使用数据")
	}
	pairs := [][2]string{
		{i18n.Tr("Input", "输入"), tokenValue(u.InputTokens)},
		{i18n.Tr("Output", "输出"), tokenValue(u.OutputTokens)},
		{i18n.Tr("Total", "总计"), tokenValue(u.TotalTokens)},
		{i18n.Tr("Cache read", "缓存读"), tokenValue(u.CacheReadTokens)},
		{i18n.Tr("Cache write", "缓存写"), tokenValue(u.CacheWriteTokens)},
		{i18n.Tr("Reasoning", "推理"), tokenValue(u.ReasoningTokens)},
		{i18n.Tr("Requests", "请求数"), tokenValue(u.RequestCount)},
		{i18n.Tr("Peak context", "上下文峰值"), tokenValue(u.PeakContextTokens)},
		{i18n.Tr("Direct sub-sessions", "直接子会话"), tokenValue(u.DirectChildTokens)},
		{i18n.Tr("All descendants", "所有后代"), tokenValue(u.DescendantTokens)},
		{i18n.Tr("Data source", "数据来源"), string(u.Source)},
		{i18n.Tr("Completeness", "完整性"), string(u.Completeness)},
	}
	var sb strings.Builder
	sb.WriteString(titleStyle.Render(i18n.Tr("Token Summary", "Token 汇总")) + "\n\n")
	sb.WriteString(kvBlock(pairs, d.width))
	if u.IsEstimated {
		sb.WriteString("\n" + dimStyle.Render(i18n.Tr("Note: some values are estimates", "注：部分数值为估算值")) + "\n")
	}
	return sb.String()
}

// renderSubagents 渲染子 Agent 会话汇总。
func (d *detailModel) renderSubagents() string {
	if d.db == nil {
		return i18n.Tr("Database unavailable", "数据库不可用")
	}
	rows, err := d.db.SQL().QueryContext(d.ctx, `
		SELECT s.session_id, s.first_question, s.started_at, s.has_token_usage
		FROM sessions s
		WHERE s.agent_instance_id = ? AND s.parent_session_id = ?
		ORDER BY s.started_at ASC`,
		d.sess.AgentInstanceID, d.sess.SessionID)
	if err != nil {
		return i18n.Trf("Failed to load sub-sessions: %v", "加载子会话失败：%v", err)
	}
	defer rows.Close()
	var sb strings.Builder
	sb.WriteString(titleStyle.Render(i18n.Tr("Sub-Agent Sessions", "子 Agent 会话")) + "\n\n")
	found := false
	for rows.Next() {
		found = true
		var (
			sid, fq  string
			started  *int64
			hasUsage bool
		)
		if err := rows.Scan(&sid, &fq, &started, &hasUsage); err != nil {
			continue
		}
		ts := ""
		if started != nil {
			ts = time.Unix(*started, 0).Format("01-02 15:04")
		}
		sb.WriteString(fmt.Sprintf("%s  %s  %s\n", ts, truncRunes(sid, 20), truncRunes(firstLine(fq), 40)))
		_ = hasUsage
	}
	if !found {
		sb.WriteString(i18n.Tr("This session has no sub-agent sessions", "该会话没有子 Agent 会话") + "\n")
	}
	return sb.String()
}

func truncRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// renderDetail 渲染会话详情文本。
func (d *detailModel) renderDetail() string {
	s := d.sess
	var sb strings.Builder

	sb.WriteString(titleStyle.Render(i18n.Tr("Session Details", "会话详情")) + "\n\n")

	// 基本信息 + 时间 + 目录 + 计数等窄键值对，双列展示
	var pairs [][2]string
	pairs = append(pairs,
		[2]string{i18n.Tr("Agent", "Agent"), displayAgent(s.AgentID)},
		[2]string{i18n.Tr("Agent Instance", "Agent 实例"), s.AgentInstanceID},
		[2]string{i18n.Tr("Session ID", "会话 ID"), s.SessionID},
		[2]string{i18n.Tr("Status", "状态"), activityText(s.Activity)},
	)
	if s.StartedAt != nil {
		pairs = append(pairs,
			[2]string{i18n.Tr("Started", "开始时间"), s.StartedAt.Format("2006-01-02 15:04:05")},
			[2]string{i18n.Tr("Start basis", "开始依据"), timeSourceText(s.StartTimeSource)},
		)
	}
	if s.EndedAt != nil {
		pairs = append(pairs,
			[2]string{i18n.Tr("Ended", "结束时间"), s.EndedAt.Format("2006-01-02 15:04:05")},
			[2]string{i18n.Tr("End basis", "结束依据"), timeSourceText(s.EndTimeSource)},
		)
	}
	if s.Duration != nil {
		pairs = append(pairs, [2]string{i18n.Tr("Duration", "持续时间"), durationText(*s.Duration)})
	}
	if s.WorkingDirectory != "" {
		pairs = append(pairs,
			[2]string{i18n.Tr("Working dir", "原运行目录"), s.WorkingDirectory},
			[2]string{i18n.Tr("Dir status", "目录状态"), dirStatus(s.WorkingDirExists)},
		)
	}
	if s.GitRoot != "" {
		pairs = append(pairs, [2]string{i18n.Tr("Git root", "Git 根目录"), s.GitRoot})
	}
	if s.GitBranch != "" {
		pairs = append(pairs, [2]string{i18n.Tr("Git branch", "Git 分支"), s.GitBranch})
	}
	if s.GitRemote != "" {
		pairs = append(pairs, [2]string{i18n.Tr("Git remote", "Git Remote"), s.GitRemote})
	}
	if s.SourcePath != "" {
		pairs = append(pairs, [2]string{i18n.Tr("Source", "源记录"), s.SourcePath})
	}
	pairs = append(pairs,
		[2]string{i18n.Tr("Messages", "消息数量"), fmt.Sprint(s.MessageCount)},
		[2]string{i18n.Tr("User messages", "用户消息"), fmt.Sprint(s.UserMessageCount)},
		[2]string{i18n.Tr("Tool calls", "工具调用"), fmt.Sprint(s.ToolCallCount)},
	)
	sb.WriteString(kvBlock(pairs, d.width))

	// 第一次提问
	sb.WriteString("\n" + labelStyle.Render(i18n.Tr("First question (full content scrollable):", "第一次提问（完整内容滚动查看）：")) + "\n")
	q := firstQuestionOrFallback(s)
	sb.WriteString(fmt.Sprintf("%s\n", valueStyle.Render(truncateLines(q, 5))))
	if lineCount(q) > 5 {
		sb.WriteString(dimStyle.Render(i18n.Trf("(total %d lines, scroll down for full)", "（共 %d 行，向下滚动查看完整）", lineCount(q))) + "\n")
	}

	// 用户轮次：默认折叠，按 t 展开
	sb.WriteString("\n" + labelStyle.Render(i18n.Tr("User turns:", "用户轮次：")) + "\n")
	if !d.turnsLoaded {
		d.loadTurnsCount()
	}
	if d.turnsVisible {
		sb.WriteString(d.renderTurnsTable())
	} else if d.turnsCount > 0 {
		sb.WriteString(dimStyle.Render(i18n.Trf("press t to show %d user turns", "按 t 显示 %d 个用户轮次", d.turnsCount)) + "\n")
	} else {
		sb.WriteString(dimStyle.Render(i18n.Tr("this session has no user turns", "该会话没有用户轮次")) + "\n")
	}

	// Token 汇总（会话级）
	if s.TokenUsage != nil {
		u := s.TokenUsage
		pairs := [][2]string{
			{i18n.Tr("Total", "总计"), tokenValue(u.TotalTokens)},
			{i18n.Tr("Input", "输入"), tokenValue(u.InputTokens)},
			{i18n.Tr("Output", "输出"), tokenValue(u.OutputTokens)},
			{i18n.Tr("Peak context", "上下文峰值"), tokenValue(u.PeakContextTokens)},
			{i18n.Tr("Completeness", "完整性"), string(u.Completeness)},
		}
		sb.WriteString("\n" + labelStyle.Render(i18n.Tr("Token summary:", "Token 汇总：")) + "\n")
		sb.WriteString(kvBlock(pairs, d.width))
	} else if s.HasTokenUsage {
		sb.WriteString("\n" + i18n.Tr("This agent session format has no token usage data", "当前 Agent 会话格式未提供 Token 使用数据") + "\n")
	}

	return sb.String()
}

// loadTurnsCount 惰性加载用户轮次数（渲染阶段最多查一次）。
func (d *detailModel) loadTurnsCount() {
	d.turnsLoaded = true
	if d.db == nil {
		return
	}
	turns, err := timeline.GroupByTurns(d.ctx, d.db, d.sess.AgentInstanceID, d.sess.SessionID)
	if err != nil {
		return
	}
	d.turnsCount = len(turns)
}

// View 渲染详情视图。
func (d *detailModel) View() string {
	if !d.ready {
		w, h := d.width, d.height
		if w <= 0 {
			w = 100
		}
		if h <= 0 {
			h = 30
		}
		d.view = viewport.New(w, h)
		d.ready = true
	}
	d.view.SetContent(d.render())
	return d.view.View()
}

// kvCell 是双列布局中的一个键值单元（含样式与显示宽度）。
type kvCell struct {
	styled string
	plainW int
}

// kvBlock 将键值对排布为最多两列（宽度不足自动回退单列）。
func kvBlock(pairs [][2]string, width int) string {
	if len(pairs) == 0 {
		return ""
	}
	cells := make([]kvCell, len(pairs))
	for i, p := range pairs {
		label := p[0] + ": "
		cells[i] = kvCell{
			styled: labelStyle.Render(label) + valueStyle.Render(p[1]),
			plainW: runewidth.StringWidth(label + p[1]),
		}
	}
	// 尝试双列
	if width > 0 {
		colW := (width - 2) / 2
		if colW >= 10 {
			fit := true
			for _, c := range cells {
				if c.plainW > colW {
					fit = false
					break
				}
			}
			if fit {
				var sb strings.Builder
				for i := 0; i < len(cells); i += 2 {
					sb.WriteString(padCell(cells[i], colW))
					if i+1 < len(cells) {
						sb.WriteString("  " + cells[i+1].styled)
					}
					sb.WriteString("\n")
				}
				return sb.String()
			}
		}
	}
	var sb strings.Builder
	for _, c := range cells {
		sb.WriteString(c.styled + "\n")
	}
	return sb.String()
}

// padCell 按显示宽度补空格（忽略 ANSI 样式）。
func padCell(c kvCell, w int) string {
	if c.plainW >= w {
		return c.styled
	}
	return c.styled + strings.Repeat(" ", w-c.plainW)
}

func firstQuestionOrFallback(s *model.Session) string {
	if s.FirstQuestion != "" {
		return s.FirstQuestion
	}
	return i18n.Tr("No valid user question detected", "未识别到有效用户提问")
}

// lineCount 统计字符串行数。
func lineCount(s string) int {
	if s == "" {
		return 0
	}
	return 1 + strings.Count(s, "\n")
}

// truncateLines 截断为前 max 行，超出加省略行。
func truncateLines(s string, max int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= max {
		return s
	}
	return strings.Join(lines[:max], "\n") + "\n…"
}

func timeSourceText(src model.TimeSource) string {
	switch src {
	case model.TimeSourceSessionMeta:
		return i18n.Tr("session metadata", "会话元数据")
	case model.TimeSourceFirstUserMsg:
		return i18n.Tr("first user message", "第一条用户消息")
	case model.TimeSourceFirstEvent:
		return i18n.Tr("first valid event", "第一条有效事件")
	case model.TimeSourceLastActivity:
		return i18n.Tr("last session activity", "最后一次会话活动")
	case model.TimeSourceFileMtime:
		return i18n.Tr("file modification time", "文件修改时间")
	case model.TimeSourceProcessStart:
		return i18n.Tr("process start", "进程启动")
	case model.TimeSourceProcessExit:
		return i18n.Tr("process exit", "进程退出")
	default:
		return i18n.Tr("unknown", "未知")
	}
}

func activityText(a model.ActivityState) string {
	switch a {
	case model.ActivityActive:
		return i18n.Tr("Active", "进行中")
	case model.ActivityPossiblyActive:
		return i18n.Tr("Possibly active", "可能进行中")
	case model.ActivityInactive:
		return i18n.Tr("Ended", "已结束")
	default:
		return i18n.Tr("Unknown", "未知")
	}
}

func dirStatus(exists bool) string {
	if exists {
		return i18n.Tr("exists", "存在")
	}
	return i18n.Tr("missing", "不存在")
}

func durationText(d time.Duration) string {
	total := int64(d / time.Second)
	if total < 60 {
		return i18n.Trf("%ds", "%d秒", total)
	}
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return i18n.Trf("%dh %dm %ds", "%d小时%d分%d秒", h, m, s)
	}
	return i18n.Trf("%dm %ds", "%d分%d秒", m, s)
}

func tokenValue(v *int64) string {
	if v == nil {
		return i18n.Tr("unknown", "未知")
	}
	return humanNum(*v)
}

func humanNum(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.2fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func humanDur(d time.Duration) string {
	total := int64(d / time.Second)
	if total < 60 {
		return i18n.Trf("%ds", "%ds", total)
	}
	h := total / 3600
	m := (total % 3600) / 60
	if h > 0 {
		return i18n.Trf("%dh%dm", "%d小时%d分", h, m)
	}
	return i18n.Trf("%dm", "%d分", m)
}

func shortHome(p string) string {
	if p == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	rel, err := filepath.Rel(home, p)
	if err != nil || strings.HasPrefix(rel, "..") {
		return p
	}
	return filepath.Join("~", rel)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
