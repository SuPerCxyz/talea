package status

import (
	"fmt"
	"strings"
	"time"

	"github.com/talea/talea/internal/i18n"
	"github.com/talea/talea/internal/model"
)

// StateLabel 返回展示状态文字。已完成固定表达“本轮结束”，不声称任务完成。
func StateLabel(ss SessionStatus) string {
	switch ss.State {
	case StateWaiting:
		if ss.Confidence == ConfidenceInferred || ss.TurnState == model.TurnStateWorking {
			return i18n.Tr("Possibly waiting", "可能等待")
		}
		return i18n.Tr("Waiting for you", "等待你确认")
	case StateWorking:
		return i18n.Tr("Working", "进行中")
	case StateCompleted:
		return i18n.Tr("Turn ended", "本轮结束")
	case StateFailed:
		if ss.TurnState == model.TurnStateInterrupted {
			return i18n.Tr("Interrupted", "已中断")
		}
		return i18n.Tr("Failed", "失败")
	case StateIdle:
		return i18n.Tr("Idle", "空闲")
	default:
		return i18n.Tr("Unknown", "未知")
	}
}

// GroupTitle 返回看板分组标题。
func GroupTitle(state DisplayState) string {
	switch state {
	case StateWaiting:
		return i18n.Tr("Needs you", "需要你")
	case StateWorking:
		return i18n.Tr("Working", "进行中")
	case StateCompleted:
		return i18n.Tr("Completed (turn ended)", "已完成（本轮结束）")
	case StateFailed:
		return i18n.Tr("Failed or interrupted", "失败或中断")
	case StateIdle:
		return i18n.Tr("Idle", "空闲")
	default:
		return i18n.Tr("Unknown", "未知")
	}
}

// ConfidenceLabel 返回判断确定程度文字。
func ConfidenceLabel(c Confidence) string {
	switch c {
	case ConfidenceExact:
		return i18n.Tr("exact", "精确")
	case ConfidenceInferred:
		return i18n.Tr("inferred", "推断")
	default:
		return i18n.Tr("unknown", "未知")
	}
}

// Short 返回适合 tmux / 脚本状态栏的单行汇总。
func (s Snapshot) Short() string {
	c := s.Counts
	return i18n.Trf(
		"Talea: needs you %d | working %d | turn ended %d | failed %d | idle %d | unknown %d",
		"Talea：需要你 %d | 进行中 %d | 本轮结束 %d | 失败 %d | 空闲 %d | 未知 %d",
		c.NeedsAttention, c.Working, c.Completed, c.Failed, c.Idle, c.Unknown)
}

// Text 返回面向终端的状态列表。
func (s Snapshot) Text() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s\n", i18n.Trf("%d session(s)", "%d 个会话", s.Counts.Total)))
	sb.WriteString(s.CountsLine() + "\n")
	for _, ss := range s.Sessions {
		line := fmt.Sprintf("[%s] %s  %s  %s  %s",
			ss.Label, displayAgent(ss.AgentID), firstNonEmpty(ss.ProjectName, ss.WorkingDirectory),
			ss.SessionID, relativeTime(ss.LastActivityAt))
		if ss.Reason != "" {
			line += " — " + ss.Reason
		}
		sb.WriteString(line + "\n")
	}
	for _, warning := range s.Warnings {
		sb.WriteString(i18n.Tr("warning: ", "警告：") + warning + "\n")
	}
	return sb.String()
}

// CountsLine 返回状态计数汇总行。
func (s Snapshot) CountsLine() string {
	c := s.Counts
	return fmt.Sprintf("%s %d · %s %d · %s %d · %s %d · %s %d · %s %d",
		GroupTitle(StateWaiting), c.Waiting,
		GroupTitle(StateWorking), c.Working,
		GroupTitle(StateCompleted), c.Completed,
		GroupTitle(StateFailed), c.Failed,
		GroupTitle(StateIdle), c.Idle,
		GroupTitle(StateUnknown), c.Unknown)
}

func displayAgent(id model.AgentID) string {
	if id == "" {
		return i18n.Tr("unknown", "未知")
	}
	return string(id)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func relativeTime(t *time.Time) string {
	if t == nil {
		return i18n.Tr("no activity", "无活动")
	}
	return shortAge(time.Since(*t)) + " " + i18n.Tr("ago", "前")
}
