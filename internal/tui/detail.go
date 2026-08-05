package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
	"github.com/talea/talea/internal/preview"
	"github.com/talea/talea/internal/timeline"
)

var (
	labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("246"))
	valueStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
)

// detailModel 是会话详情视图。
type detailModel struct {
	ctx   context.Context
	app   *app.App
	db    *index.DB
	sess  *model.Session
	view  viewport.Model
	ready bool
	tab   string // "" 详情, "timeline", "context", "model", "preview"
}

// render 渲染当前 tab 内容。
func (d *detailModel) render() string {
	switch d.tab {
	case "timeline":
		return d.renderTimeline()
	case "context":
		return d.renderContext()
	case "model":
		return d.renderModel()
	case "preview":
		return d.renderPreview()
	default:
		return d.renderDetail()
	}
}

// renderTimeline 渲染请求级时间线。
func (d *detailModel) renderTimeline() string {
	if d.db == nil {
		return "数据库不可用"
	}
	events, err := timeline.List(d.ctx, d.db, timeline.Query{
		AgentInstanceID: d.sess.AgentInstanceID,
		SessionID:       d.sess.SessionID,
		Limit:           200,
	})
	if err != nil {
		return "加载时间线失败：" + err.Error()
	}
	if len(events) == 0 {
		return "该会话没有 Token 时间线数据"
	}
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Token 时间线") + "\n\n")
	for _, e := range events {
		ts := ""
		if e.Timestamp != nil {
			ts = e.Timestamp.Format("15:04:05")
		}
		sb.WriteString(fmt.Sprintf("%s  %-18s  %s\n", ts, string(e.EventType), timelineDesc(e)))
	}
	sb.WriteString("\n按 h 返回详情")
	return sb.String()
}

// renderContext 渲染上下文窗口曲线。
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
	sb.WriteString(titleStyle.Render("上下文窗口曲线") + "\n\n")
	sb.WriteString(fmt.Sprintf("%-10s  %-10s  %-12s\n", "时间", "上下文", "变化"))
	for _, p := range pts {
		sb.WriteString(fmt.Sprintf("%-10s  %-10s  %+s\n",
			time.Unix(p.Timestamp, 0).Format("15:04:05"),
			humanNum(p.Context), signedHuman(p.Change)))
	}
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
	sb.WriteString("\n按 h 返回详情")
	return sb.String()
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
	sb.WriteString("\n按 h 返回详情")
	return sb.String()
}

// renderPreview 渲染对话预览（按需加载，ANSI 清理 + 脱敏）。
func (d *detailModel) renderPreview() string {
	ad, ok := d.app.Registry.Get(d.sess.AgentID)
	if !ok {
		return "会话格式不支持"
	}
	msgs, err := preview.Load(d.ctx, ad, *d.sess, preview.Options{
		Limit:  60,
		Redact: d.app.Config.Privacy.RedactSecretsInPreview,
	})
	if err != nil {
		return "加载预览失败：" + err.Error()
	}
	if len(msgs) == 0 {
		return "该会话没有可预览的消息"
	}
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("对话预览") + "\n\n")
	for _, m := range msgs {
		roleStyle := valueStyle
		if m.Role == "用户" {
			roleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("120"))
		}
		sb.WriteString(fmt.Sprintf("[%s] %s\n", roleStyle.Render(m.Role), labelStyle.Render(m.Timestamp)))
		content := strings.TrimSpace(m.Content)
		if content != "" {
			sb.WriteString(content + "\n")
		}
		sb.WriteString("\n")
	}
	sb.WriteString("按 h 返回详情")
	return sb.String()
}

func timelineDesc(e timeline.Event) string {
	var parts []string
	if e.TotalTokens != nil {
		parts = append(parts, fmt.Sprintf("总计 %s", humanNum(*e.TotalTokens)))
	}
	if e.InputTokens != nil {
		parts = append(parts, fmt.Sprintf("输入 %s", humanNum(*e.InputTokens)))
	}
	if e.OutputTokens != nil {
		parts = append(parts, fmt.Sprintf("输出 %s", humanNum(*e.OutputTokens)))
	}
	return strings.Join(parts, "  ")
}

func signedHuman(n int64) string {
	if n >= 0 {
		return "+" + humanNum(n)
	}
	return "-" + humanNum(-n)
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

	sb.WriteString("\n" + labelStyle.Render("第一次提问：") + "\n")
	sb.WriteString(fmt.Sprintf("%s\n", valueStyle.Render(firstQuestionOrFallback(s))))

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

	sb.WriteString("\n\n" + labelStyle.Render("按键：t 时间线  c 上下文  m 模型  o 恢复  esc 返回  q 退出"))
	return sb.String()
}

// View 渲染详情视图。
func (d *detailModel) View() string {
	if !d.ready {
		d.view = viewport.New(0, 0)
		d.ready = true
	}
	d.view.SetContent(d.render())
	return d.view.View()
}

var _ = context.Background
var _ = adapters.Command{}
var _ = app.App{}

func firstQuestionOrFallback(s *model.Session) string {
	if s.FirstQuestion != "" {
		return s.FirstQuestion
	}
	return "未识别到有效用户提问"
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
