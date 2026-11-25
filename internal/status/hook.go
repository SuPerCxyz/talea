package status

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/talea/talea/internal/model"
)

const maxHookDetailRunes = 200

// hookProperties 是 OpenCode 原始 plugin 事件的 properties 字段。
type hookProperties struct {
	SessionID      string `json:"sessionID"`
	SessionIDCamel string `json:"sessionId"`
	Outcome        string `json:"outcome"`
	Status         string `json:"status"`
	Info           struct {
		ID string `json:"id"`
	} `json:"info"`
}

// hookEnvelope 兼容 Claude Code hooks 与 OpenCode plugin 的常见字段。
type hookEnvelope struct {
	Agent            string         `json:"agent"`
	AgentID          string         `json:"agent_id"`
	AgentInstanceID  string         `json:"agent_instance_id"`
	SessionID        string         `json:"session_id"`
	SessionIDCamel   string         `json:"sessionId"`
	State            string         `json:"state"`
	HookEventName    string         `json:"hook_event_name"`
	Event            string         `json:"event"`
	Type             string         `json:"type"`
	NotificationType string         `json:"notification_type"`
	Outcome          string         `json:"outcome"`
	Properties       hookProperties `json:"properties"`
	Detail           string         `json:"detail"`
	Title            string         `json:"title"`
	Reason           string         `json:"reason"`
	At               string         `json:"at"`
	Timestamp        string         `json:"timestamp"`
}

// EventFromHook 把 hook/plugin 的 JSON 输入映射为统一事件。
// 只提取状态所需字段，不保存消息正文。
func EventFromHook(raw []byte, now time.Time) (Event, error) {
	var in hookEnvelope
	if err := json.Unmarshal(raw, &in); err != nil {
		return Event{}, fmt.Errorf("解析 hook 输入: %w", err)
	}
	sessionID := firstNonEmpty(in.SessionID, in.SessionIDCamel,
		in.Properties.SessionID, in.Properties.SessionIDCamel, in.Properties.Info.ID)
	if sessionID == "" {
		return Event{}, fmt.Errorf("hook 输入缺少 session_id")
	}
	agent, err := resolveHookAgent(in)
	if err != nil {
		return Event{}, err
	}
	state, detail, err := mapHookState(in)
	if err != nil {
		return Event{}, err
	}
	at := parseHookTime(firstNonEmpty(in.At, in.Timestamp), now)
	if detail == "" {
		detail = firstNonEmpty(in.Detail, in.Title, in.Reason)
	}
	return Event{
		AgentID:         string(agent),
		AgentInstanceID: in.AgentInstanceID,
		SessionID:       sessionID,
		State:           state,
		At:              at,
		Detail:          truncateRunes(detail, maxHookDetailRunes),
	}, nil
}

func resolveHookAgent(in hookEnvelope) (model.AgentID, error) {
	raw := firstNonEmpty(in.Agent, in.AgentID)
	if raw != "" {
		return CanonicalAgent(raw), nil
	}
	// Claude hooks 通过 hook_event_name 区分；OpenCode plugin 使用带点号的事件名。
	if in.HookEventName != "" {
		return model.AgentClaudeCode, nil
	}
	event := strings.ToLower(firstNonEmpty(in.Event, in.Type))
	if strings.HasPrefix(event, "session.") || strings.HasPrefix(event, "permission.") ||
		strings.HasPrefix(event, "message.") || strings.HasPrefix(event, "tool.execute.") {
		return model.AgentOpenCode, nil
	}
	return "", fmt.Errorf("无法判断 hook 所属 Agent")
}

// CanonicalAgent 将常见 Agent 别名规范化为注册表中的 AgentID。
func CanonicalAgent(raw string) model.AgentID {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "claude", "claude-code", "claudecode":
		return model.AgentClaudeCode
	case "codex", "codex-cli", "codexcli":
		return model.AgentCodexCLI
	case "opencode", "open-code":
		return model.AgentOpenCode
	default:
		return model.AgentID(strings.TrimSpace(raw))
	}
}

func mapHookState(in hookEnvelope) (model.TurnState, string, error) {
	if explicit := strings.TrimSpace(in.State); explicit != "" {
		state := model.TurnState(strings.ToLower(explicit))
		if !state.Valid() || state == model.TurnStateUnknown {
			return "", "", fmt.Errorf("无效的 hook 状态: %s", explicit)
		}
		return state, "", nil
	}
	if name := strings.TrimSpace(in.HookEventName); name != "" {
		return mapClaudeHook(name, in)
	}
	if name := strings.TrimSpace(in.Event); name != "" {
		return mapEventHook(name, hookOutcome(in))
	}
	if name := strings.TrimSpace(in.Type); name != "" {
		return mapEventHook(name, hookOutcome(in))
	}
	return "", "", fmt.Errorf("hook 输入缺少事件或状态")
}

func hookOutcome(in hookEnvelope) string {
	return firstNonEmpty(in.Outcome, in.Properties.Outcome, in.Properties.Status)
}

func mapClaudeHook(name string, in hookEnvelope) (model.TurnState, string, error) {
	switch strings.ToLower(name) {
	case "userpromptsubmit", "pretooluse", "posttooluse":
		return model.TurnStateWorking, strings.ToLower(name), nil
	case "permissionrequest":
		return model.TurnStateWaitingInput, "permission request", nil
	case "posttoolusefailure", "stopfailure":
		return model.TurnStateFailed, strings.ToLower(name), nil
	case "stop":
		return model.TurnStateCompleted, strings.ToLower(name), nil
	case "sessionend":
		detail := "sessionend"
		if in.Reason != "" {
			detail += ": " + in.Reason
		}
		return model.TurnStateIdle, detail, nil
	case "notification":
		// 缺失 type 时 Claude Notification 本身仍需用户注意；已知等待类型也映射等待。
		// auth_success 等通知不代表等待用户，必须忽略，避免误报。
		nt := strings.ToLower(strings.TrimSpace(in.NotificationType))
		switch nt {
		case "", "permission_prompt", "idle_prompt", "elicitation_dialog":
			detail := "notification"
			if nt != "" {
				detail += ": " + nt
			}
			return model.TurnStateWaitingInput, detail, nil
		default:
			return "", "", fmt.Errorf("忽略非等待类 notification: %s", nt)
		}
	default:
		return "", "", fmt.Errorf("未知的 Claude hook: %s", name)
	}
}

func mapEventHook(name, outcome string) (model.TurnState, string, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "permission.asked", "permission_request", "permission_requested", "permission.updated":
		return model.TurnStateWaitingInput, "permission asked", nil
	case "permission.replied", "message.updated", "message.part.updated",
		"session.started", "tool.execute.before", "tool.execute.after":
		return model.TurnStateWorking, strings.ToLower(name), nil
	case "session.idle":
		// 原始 OpenCode 事件通常只有 type，没有 outcome；空闲表示本轮已结束。
		if strings.TrimSpace(outcome) == "" {
			return model.TurnStateCompleted, "session.idle", nil
		}
		return idleStateOf(outcome), "session.idle", nil
	case "session.completed":
		return model.TurnStateCompleted, "session.completed", nil
	case "session.failed", "session.error":
		return model.TurnStateFailed, strings.ToLower(name), nil
	case "session.interrupted", "session.aborted":
		return model.TurnStateInterrupted, strings.ToLower(name), nil
	default:
		return "", "", fmt.Errorf("未知的 plugin 事件: %s", name)
	}
}

// idleStateOf 把 idle outcome 映射为通用状态；未知 outcome 视为普通空闲。
func idleStateOf(outcome string) model.TurnState {
	switch strings.ToLower(strings.TrimSpace(outcome)) {
	case "succeeded", "success", "completed", "complete", "ok", "done":
		return model.TurnStateCompleted
	case "failed", "failure", "error", "errored":
		return model.TurnStateFailed
	case "interrupted", "cancelled", "canceled", "aborted", "stopped":
		return model.TurnStateInterrupted
	default:
		return model.TurnStateIdle
	}
}

func parseHookTime(raw string, now time.Time) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return now
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t
		}
	}
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		if n > 1e12 {
			return time.UnixMilli(n)
		}
		return time.Unix(n, 0)
	}
	return now
}

func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}
