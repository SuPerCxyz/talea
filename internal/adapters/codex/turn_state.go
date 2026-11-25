package codex

import (
	"strings"
	"time"

	"github.com/talea/talea/internal/model"
)

// turnSnapshot 记录解析过程中最近一次有效回合状态。
type turnSnapshot struct {
	state  model.TurnState
	at     *time.Time
	source model.TurnStateSource
	detail string
}

// apply 按事件时间更新回合状态；较旧的事件不覆盖较新的状态。
func (t *turnSnapshot) apply(state model.TurnState, source model.TurnStateSource, detail string, at time.Time, hasAt bool) {
	if hasAt {
		if t.at != nil && at.Before(*t.at) {
			return
		}
		ts := at
		t.at = &ts
	} else if t.at != nil {
		return
	}
	t.state = state
	t.source = source
	t.detail = detail
}

// applyCodexEvent 把 Codex event_msg 映射为通用回合状态。
// 审批事件名在不同版本间存在差异，未知事件直接忽略。
func applyCodexEvent(turn *turnSnapshot, eventType, reason string, ts time.Time, hasTS bool) {
	name := strings.ToLower(strings.TrimSpace(eventType))
	switch name {
	case "task_started", "turn_started":
		turn.apply(model.TurnStateWorking, model.TurnStateSourceNative, name, ts, hasTS)
	case "task_complete", "turn_complete":
		turn.apply(model.TurnStateCompleted, model.TurnStateSourceNative, name, ts, hasTS)
	case "turn_aborted":
		state := model.TurnStateInterrupted
		lower := strings.ToLower(reason)
		if strings.Contains(lower, "error") || strings.Contains(lower, "fail") {
			state = model.TurnStateFailed
		}
		detail := name
		if reason != "" {
			detail += ": " + reason
		}
		turn.apply(state, model.TurnStateSourceNative, detail, ts, hasTS)
	case "exec_approval_request", "apply_patch_approval_request", "request_user_input",
		"user_input_request", "elicitation_request", "permission_request":
		turn.apply(model.TurnStateWaitingInput, model.TurnStateSourceNative, name, ts, hasTS)
	case "error", "stream_error":
		turn.apply(model.TurnStateFailed, model.TurnStateSourceNative, name, ts, hasTS)
	}
}
