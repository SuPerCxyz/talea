package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/chart"
	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
	"github.com/talea/talea/internal/timeline"
	"github.com/talea/talea/internal/usage"
)

var (
	labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("246"))
	valueStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
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
}

// render 渲染详情页全部内容（聚合展示，viewport 滚动查看）。
func (d *detailModel) render() string {
	if d.sess == nil {
		return "会话数据为空"
	}
	var sb strings.Builder
	sb.WriteString(d.renderDetail())
	sb.WriteString("\n\n" + titleStyle.Render("上下文窗口曲线") + "\n")
	sb.WriteString(sectionOr(d.renderContext()))
	sb.WriteString("\n\n" + titleStyle.Render("按模型汇总") + "\n")
	sb.WriteString(sectionOr(d.renderModel()))
	sb.WriteString("\n\n" + titleStyle.Render("Token 图表") + "\n")
	sb.WriteString(sectionOr(d.renderCharts()))
	sb.WriteString("\n\n" + titleStyle.Render("Token 汇总") + "\n")
	sb.WriteString(sectionOr(d.renderUsageSummary()))
	sb.WriteString("\n\n" + titleStyle.Render("子 Agent 会话") + "\n")
	sb.WriteString(sectionOr(d.renderSubagents()))
	sb.WriteString("\n\n" + labelStyle.Render("按键：o 恢复  esc/q 返回列表  ↑/↓ 滚动"))
	return sb.String()
}

// sectionOr 返回子渲染内容；剥离其自带标题行，nil 场景返回降级提示。
func sectionOr(s string) string {
	if s == "" {
		return "（无数据）\n"
	}
	// 错误/空数据消息直接展示
	if strings.HasPrefix(s, "数据库不可用") || strings.HasPrefix(s, "加载") ||
		strings.HasPrefix(s, "该会话") || strings.HasPrefix(s, "没有") ||
		strings.HasPrefix(s, "会话格式") {
		return s + "\n"
	}
	// 剥离首行标题（聚合时已加标题）
	if i := strings.Index(s, "\n"); i >= 0 {
		return s[i+1:] + "\n"
	}
	return s + "\n"
}

// renderContext 渲染上下文窗口曲线（ASCII 曲线图）。
func (d *detailModel) renderContext() string {
	if d.db == nil {
		return "数据库不可用"
	}
	pts, err := timeline.ContextCurve(d.ctx, d.db, d.sess.AgentInstanceID, d.sess.SessionID, 40)
	if err != nil {
		return "加载上下文曲线失败：" + err.Error()
	}
	if len(pts) == 0 {
		return "没有上下文窗口数据"
	}
	comps, _ := timeline.DetectCompactions(d.ctx, d.db, d.sess.AgentInstanceID, d.sess.SessionID)
	var sb strings.Builder
	sb.WriteString(labelStyle.Render("上下文窗口（Token）：") + "\n")
	vals := make([]float64, len(pts))
	labels := make([]string, len(pts))
	for i, p := range pts {
		vals[i] = float64(p.Context)
		labels[i] = time.Unix(p.Timestamp, 0).Format("15:04")
	}
	sb.WriteString(chart.Line(vals, 50) + "\n")
	// 起点/峰值/终点标注
	sb.WriteString(fmt.Sprintf("起点 %s  峰值 %s  终点 %s\n",
		humanNum(pts[0].Context), humanNum(int64(peakVal(vals))), humanNum(pts[len(pts)-1].Context)))
	if len(comps) > 0 {
		sb.WriteString("\n上下文压缩：\n")
		for _, c := range comps {
			label := "明确压缩"
			if c.IsInferred {
				label = "可能发生上下文压缩"
			}
			sb.WriteString(fmt.Sprintf("  %s → %s（%s）\n", humanNum(c.Before), humanNum(c.After), label))
		}
	}
	// 聚合展示，无需 tab 返回
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
		return "数据库不可用"
	}
	sums, err := timeline.ByModel(d.ctx, d.db, d.sess.AgentInstanceID, d.sess.SessionID)
	if err != nil {
		return "加载模型汇总失败：" + err.Error()
	}
	if len(sums) == 0 {
		return "该会话没有模型数据"
	}
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("按模型汇总") + "\n\n")
	sb.WriteString(fmt.Sprintf("%-24s  %-8s  %-10s  %-10s  %-10s\n", "模型", "请求", "输入", "输出", "总计"))
	for _, m := range sums {
		sb.WriteString(fmt.Sprintf("%-24s  %-8d  %-10s  %-10s  %-10s\n",
			m.Model, m.Requests, humanNum(m.InputTokens), humanNum(m.OutputTokens), humanNum(m.TotalTokens)))
	}
	// 聚合展示，无需 tab 返回
	return sb.String()
}

// renderTurnsTable 渲染用户轮次表格（无标题，供聚合/详情嵌入）。
func (d *detailModel) renderTurnsTable() string {
	if d.db == nil {
		return "数据库不可用\n"
	}
	turns, err := timeline.GroupByTurns(d.ctx, d.db, d.sess.AgentInstanceID, d.sess.SessionID)
	if err != nil {
		return "加载轮次失败：" + err.Error() + "\n"
	}
	if len(turns) == 0 {
		return "该会话没有可聚合的轮次\n"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%-5s  %-40s  %-6s  %-10s\n", "轮次", "提问", "请求", "Token"))
	for _, t := range turns {
		sb.WriteString(fmt.Sprintf("%-5d  %-40s  %-6d  %-10s\n",
			t.Index, truncRunes(t.Prompt, 38), t.Requests, humanNum(t.Total)))
	}
	return sb.String()
}

// renderCharts 渲染终端图表（速率柱状图 + 累计曲线）。
func (d *detailModel) renderCharts() string {
	if d.db == nil {
		return "数据库不可用"
	}
	buckets, err := timeline.GroupByBucket(d.ctx, d.db, d.sess.AgentInstanceID, d.sess.SessionID, timeline.Bucket5m)
	if err != nil {
		return "加载图表失败：" + err.Error()
	}
	if len(buckets) == 0 {
		return "该会话没有可聚合的数据"
	}
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Token 图表") + "\n\n")

	// 速率柱状图
	vals := make([]float64, len(buckets))
	labels := make([]string, len(buckets))
	for i, b := range buckets {
		vals[i] = float64(b.TotalTokens) / 5.0
		labels[i] = b.Start.Format("15:04")
	}
	sb.WriteString(labelStyle.Render("Token/分钟（每 5 分钟桶）：") + "\n")
	sb.WriteString(chart.Bar(vals, labels, 6) + "\n\n")

	// 累计曲线
	cumVals := make([]float64, len(buckets))
	var cum int64
	for i, b := range buckets {
		cum += b.TotalTokens
		cumVals[i] = float64(cum)
	}
	sb.WriteString(labelStyle.Render("累计 Token 曲线：") + "\n")
	sb.WriteString(chart.Line(cumVals, 50) + "\n")

	// 聚合展示，无需 tab 返回
	return sb.String()
}

// renderUsageSummary 渲染 Token 汇总（U 键）。
func (d *detailModel) renderUsageSummary() string {
	if d.db == nil {
		return "数据库不可用"
	}
	u, err := usage.Load(d.ctx, d.db, d.sess.AgentInstanceID, d.sess.SessionID)
	if err != nil || u == nil {
		return "当前 Agent 会话格式未提供 Token 使用数据"
	}
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Token 汇总") + "\n\n")
	sb.WriteString(fmt.Sprintf("%s %s\n", labelStyle.Render("输入："), valueStyle.Render(tokenValue(u.InputTokens))))
	sb.WriteString(fmt.Sprintf("%s %s\n", labelStyle.Render("输出："), valueStyle.Render(tokenValue(u.OutputTokens))))
	sb.WriteString(fmt.Sprintf("%s %s\n", labelStyle.Render("总计："), valueStyle.Render(tokenValue(u.TotalTokens))))
	sb.WriteString(fmt.Sprintf("%s %s\n", labelStyle.Render("缓存读："), valueStyle.Render(tokenValue(u.CacheReadTokens))))
	sb.WriteString(fmt.Sprintf("%s %s\n", labelStyle.Render("缓存写："), valueStyle.Render(tokenValue(u.CacheWriteTokens))))
	sb.WriteString(fmt.Sprintf("%s %s\n", labelStyle.Render("推理："), valueStyle.Render(tokenValue(u.ReasoningTokens))))
	sb.WriteString(fmt.Sprintf("%s %s\n", labelStyle.Render("请求数："), valueStyle.Render(tokenValue(u.RequestCount))))
	sb.WriteString(fmt.Sprintf("%s %s\n", labelStyle.Render("上下文峰值："), valueStyle.Render(tokenValue(u.PeakContextTokens))))
	sb.WriteString(fmt.Sprintf("%s %s\n", labelStyle.Render("直接子会话："), valueStyle.Render(tokenValue(u.DirectChildTokens))))
	sb.WriteString(fmt.Sprintf("%s %s\n", labelStyle.Render("所有后代："), valueStyle.Render(tokenValue(u.DescendantTokens))))
	sb.WriteString(fmt.Sprintf("%s %s\n", labelStyle.Render("数据来源："), valueStyle.Render(string(u.Source))))
	sb.WriteString(fmt.Sprintf("%s %s\n", labelStyle.Render("完整性："), valueStyle.Render(string(u.Completeness))))
	if u.IsEstimated {
		sb.WriteString("\n注：部分数值为估算值\n")
	}
	// 聚合展示，无需 tab 返回
	return sb.String()
}

// renderSubagents 渲染子 Agent 会话汇总（A 键）。
func (d *detailModel) renderSubagents() string {
	if d.db == nil {
		return "数据库不可用"
	}
	rows, err := d.db.SQL().QueryContext(d.ctx, `
		SELECT s.session_id, s.first_question, s.started_at, s.has_token_usage
		FROM sessions s
		WHERE s.agent_instance_id = ? AND s.parent_session_id = ?
		ORDER BY s.started_at ASC`,
		d.sess.AgentInstanceID, d.sess.SessionID)
	if err != nil {
		return "加载子会话失败：" + err.Error()
	}
	defer rows.Close()
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("子 Agent 会话") + "\n\n")
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
		sb.WriteString("该会话没有子 Agent 会话\n")
	}
	// 聚合展示，无需 tab 返回
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

	sb.WriteString(titleStyle.Render("会话详情") + "\n\n")
	sb.WriteString(fmt.Sprintf("%s %s\n", labelStyle.Render("Agent："), valueStyle.Render(displayAgent(s.AgentID))))
	sb.WriteString(fmt.Sprintf("%s %s\n", labelStyle.Render("Agent 实例："), valueStyle.Render(s.AgentInstanceID)))
	sb.WriteString(fmt.Sprintf("%s %s\n", labelStyle.Render("会话 ID："), valueStyle.Render(s.SessionID)))
	sb.WriteString(fmt.Sprintf("%s %s\n", labelStyle.Render("状态："), valueStyle.Render(activityText(s.Activity))))

	sb.WriteString("\n" + labelStyle.Render("第一次提问（完整内容滚动查看）：") + "\n")
	q := firstQuestionOrFallback(s)
	sb.WriteString(fmt.Sprintf("%s\n", valueStyle.Render(truncateLines(q, 5))))
	if lineCount(q) > 5 {
		sb.WriteString(fmt.Sprintf("%s\n", dimStyle.Render(fmt.Sprintf("（共 %d 行，向下滚动查看完整）", lineCount(q)))))
	}

	// 用户轮次与第一次提问同区展示
	sb.WriteString("\n" + labelStyle.Render("用户轮次：") + "\n")
	sb.WriteString(d.renderTurnsTable())

	if s.StartedAt != nil {
		sb.WriteString(fmt.Sprintf("\n%s %s\n", labelStyle.Render("开始时间："), valueStyle.Render(s.StartedAt.Format("2006-01-02 15:04:05"))))
		sb.WriteString(fmt.Sprintf("%s %s\n", labelStyle.Render("开始依据："), valueStyle.Render(timeSourceText(s.StartTimeSource))))
	}
	if s.EndedAt != nil {
		sb.WriteString(fmt.Sprintf("%s %s\n", labelStyle.Render("结束时间："), valueStyle.Render(s.EndedAt.Format("2006-01-02 15:04:05"))))
		sb.WriteString(fmt.Sprintf("%s %s\n", labelStyle.Render("结束依据："), valueStyle.Render(timeSourceText(s.EndTimeSource))))
	}
	if s.Duration != nil {
		sb.WriteString(fmt.Sprintf("%s %s\n", labelStyle.Render("持续时间："), valueStyle.Render(durationText(*s.Duration))))
	}

	if s.WorkingDirectory != "" {
		sb.WriteString(fmt.Sprintf("\n%s %s\n", labelStyle.Render("原运行目录："), valueStyle.Render(s.WorkingDirectory)))
		sb.WriteString(fmt.Sprintf("%s %s\n", labelStyle.Render("目录状态："), valueStyle.Render(dirStatus(s.WorkingDirExists))))
	}
	if s.GitRoot != "" {
		sb.WriteString(fmt.Sprintf("%s %s\n", labelStyle.Render("Git 根目录："), valueStyle.Render(s.GitRoot)))
	}
	if s.GitBranch != "" {
		sb.WriteString(fmt.Sprintf("%s %s\n", labelStyle.Render("Git 分支："), valueStyle.Render(s.GitBranch)))
	}
	if s.GitRemote != "" {
		sb.WriteString(fmt.Sprintf("%s %s\n", labelStyle.Render("Git Remote："), valueStyle.Render(s.GitRemote)))
	}

	if s.SourcePath != "" {
		sb.WriteString(fmt.Sprintf("%s %s\n", labelStyle.Render("源记录："), valueStyle.Render(s.SourcePath)))
	}

	sb.WriteString(fmt.Sprintf("%s %s\n", labelStyle.Render("消息数量："), valueStyle.Render(fmt.Sprint(s.MessageCount))))
	sb.WriteString(fmt.Sprintf("%s %s\n", labelStyle.Render("用户消息："), valueStyle.Render(fmt.Sprint(s.UserMessageCount))))
	sb.WriteString(fmt.Sprintf("%s %s\n", labelStyle.Render("工具调用："), valueStyle.Render(fmt.Sprint(s.ToolCallCount))))

	if s.TokenUsage != nil {
		sb.WriteString("\n" + labelStyle.Render("Token 汇总：") + "\n")
		u := s.TokenUsage
		sb.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("总计："), tokenValue(u.TotalTokens)))
		sb.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("输入："), tokenValue(u.InputTokens)))
		sb.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("输出："), tokenValue(u.OutputTokens)))
		sb.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("上下文峰值："), tokenValue(u.PeakContextTokens)))
		sb.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("完整性："), valueStyle.Render(string(u.Completeness))))
	} else if s.HasTokenUsage {
		sb.WriteString("\n当前 Agent 会话格式未提供 Token 使用数据\n")
	}

	return sb.String()
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

func firstQuestionOrFallback(s *model.Session) string {
	if s.FirstQuestion != "" {
		return s.FirstQuestion
	}
	return "未识别到有效用户提问"
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
		return "会话元数据"
	case model.TimeSourceFirstUserMsg:
		return "第一条用户消息"
	case model.TimeSourceFirstEvent:
		return "第一条有效事件"
	case model.TimeSourceLastActivity:
		return "最后一次会话活动"
	case model.TimeSourceFileMtime:
		return "文件修改时间"
	case model.TimeSourceProcessStart:
		return "进程启动"
	case model.TimeSourceProcessExit:
		return "进程退出"
	default:
		return "未知"
	}
}

func activityText(a model.ActivityState) string {
	switch a {
	case model.ActivityActive:
		return "进行中"
	case model.ActivityPossiblyActive:
		return "可能进行中"
	case model.ActivityInactive:
		return "已结束"
	default:
		return "未知"
	}
}

func dirStatus(exists bool) string {
	if exists {
		return "存在"
	}
	return "不存在"
}

func durationText(d time.Duration) string {
	total := int64(d / time.Second)
	if total < 60 {
		return fmt.Sprintf("%d秒", total)
	}
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return fmt.Sprintf("%d小时%d分%d秒", h, m, s)
	}
	return fmt.Sprintf("%d分%d秒", m, s)
}

func tokenValue(v *int64) string {
	if v == nil {
		return "未知"
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
		return fmt.Sprintf("%ds", total)
	}
	h := total / 3600
	m := (total % 3600) / 60
	if h > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

func shortHome(p string) string {
	if strings.HasPrefix(p, "/home/") {
		parts := strings.SplitN(strings.TrimPrefix(p, "/home/"), "/", 2)
		if len(parts) == 2 {
			return "~/" + parts[1]
		}
	}
	return p
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
