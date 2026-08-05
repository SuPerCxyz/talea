package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/model"
)

var (
	labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("246"))
	valueStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
)

// buildDetail 渲染会话详情文本。
func (d *detailModel) render() string {
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

	sb.WriteString("\n\n" + labelStyle.Render("按键：o 恢复  esc 返回  q 退出"))
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

// doResume 由主模型调用。
func (d *detailModel) doResume() tea.Cmd { return nil }

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
