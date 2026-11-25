package status

import (
	"fmt"
	"time"

	"github.com/talea/talea/internal/model"
)

const (
	// QuietThreshold 后仍无新回合事件时，显示为“可能等待”（推断）。
	QuietThreshold = 2 * time.Minute
	// ActiveWindow 后仍无新回合事件时，不再视为进行中。
	ActiveWindow = 30 * time.Minute
	// CompletedWindow 内显示“本轮结束”，更早归入空闲。
	CompletedWindow = 24 * time.Hour
)

// Evaluate 合并回合状态、精确事件、活动状态与时间阈值，生成展示态。
func Evaluate(sess model.Session, rec model.TurnStateRecord, event *Event, now time.Time) SessionStatus {
	base := rec
	if base.State == "" {
		// 索引缺少记录时允许直接使用适配器已填入的会话字段。
		base.State = sess.TurnState
		base.Source = sess.TurnStateSource
		base.StateAt = sess.TurnStateAt
		base.Detail = sess.TurnStateDetail
	}
	if base.State == "" || !base.State.Valid() {
		base.State = model.TurnStateUnknown
		base.Source = model.TurnStateSourceUnknown
		base.StateAt = nil
		base.Detail = ""
	}
	if base.Source == "" || !base.Source.Valid() {
		base.Source = model.TurnStateSourceUnknown
	}
	if event != nil && eventApplies(*event, base.StateAt) {
		base.State = event.State
		base.StateAt = timePtr(event.At)
		base.Source = model.TurnStateSourceHook
		base.Detail = event.Detail
	}

	ss := SessionStatus{
		AgentID:          sess.AgentID,
		Agent:            string(sess.AgentID),
		AgentInstanceID:  sess.AgentInstanceID,
		SessionID:        sess.SessionID,
		ProjectName:      sess.ProjectName,
		WorkingDirectory: sess.WorkingDirectory,
		TurnState:        base.State,
		Source:           base.Source,
		StateAt:          base.StateAt,
		LastActivityAt:   sess.LastActivityAt,
		TotalTokens:      sessionTokens(sess),
		Session:          &sess,
	}
	stateAt := base.StateAt
	if stateAt == nil {
		stateAt = sess.LastActivityAt
	}
	applyState(&ss, base, stateAt, sess.Activity, now)
	ss.Label = StateLabel(ss)
	return ss
}

// eventApplies 判断精确事件是否不早于会话记录中的回合状态时间。
func eventApplies(event Event, stateAt *time.Time) bool {
	if event.At.IsZero() {
		return false
	}
	if stateAt == nil {
		return true
	}
	return !event.At.Before(*stateAt)
}

func applyState(ss *SessionStatus, base model.TurnStateRecord, at *time.Time, activity model.ActivityState, now time.Time) {
	ss.Confidence = confidenceOf(base.Source)
	switch base.State {
	case model.TurnStateWaitingInput:
		setDisplay(ss, StateWaiting, "waiting for user input")
		if base.Detail != "" {
			ss.Reason = base.Detail
		}
	case model.TurnStateWorking:
		age, ok := ageOf(at, now)
		switch {
		case ok && age > ActiveWindow:
			setDisplay(ss, StateIdle, fmt.Sprintf("no turn activity for %s", shortAge(age)))
			ss.Source = model.TurnStateSourceInferred
			ss.Confidence = ConfidenceInferred
		case ok && age > QuietThreshold:
			setDisplay(ss, StateWaiting, fmt.Sprintf("possibly waiting: no new turn events for %s", shortAge(age)))
			ss.Source = model.TurnStateSourceInferred
			ss.Confidence = ConfidenceInferred
		default:
			setDisplay(ss, StateWorking, workingReason(base.Detail, age, ok))
		}
	case model.TurnStateCompleted:
		if at != nil && now.Sub(*at) > CompletedWindow {
			setDisplay(ss, StateIdle, fmt.Sprintf("turn ended %s ago; no recent activity", shortAge(now.Sub(*at))))
			ss.Source = model.TurnStateSourceInferred
			ss.Confidence = ConfidenceInferred
		} else {
			setDisplay(ss, StateCompleted, completedReason(at, now))
		}
	case model.TurnStateFailed, model.TurnStateInterrupted:
		setDisplay(ss, StateFailed, base.Detail)
		if base.State == model.TurnStateInterrupted && ss.Reason == "" {
			ss.Reason = "turn interrupted"
		}
		if ss.Reason == "" {
			ss.Reason = "turn failed"
		}
	case model.TurnStateIdle:
		setDisplay(ss, StateIdle, "session idle")
	case model.TurnStateUnknown:
		// 活动状态只用于解释原因；没有可识别回合事件时不得猜测为完成或进行中。
		reason := "no recognizable turn event"
		if activity == model.ActivityActive {
			reason += " (agent process active)"
		} else if activity == model.ActivityPossiblyActive {
			reason += " (recent session activity)"
		}
		setDisplay(ss, StateUnknown, reason)
		ss.Source = model.TurnStateSourceUnknown
		ss.Confidence = ConfidenceUnknown
	default:
		setDisplay(ss, StateUnknown, "no reliable turn state")
		ss.Confidence = ConfidenceUnknown
	}
}

func setDisplay(ss *SessionStatus, state DisplayState, reason string) {
	ss.State = state
	ss.Reason = reason
	ss.NeedsAttention = state == StateWaiting || state == StateFailed
}

func confidenceOf(source model.TurnStateSource) Confidence {
	switch source {
	case model.TurnStateSourceNative, model.TurnStateSourceHook:
		return ConfidenceExact
	case model.TurnStateSourceInferred:
		return ConfidenceInferred
	default:
		return ConfidenceUnknown
	}
}

func workingReason(detail string, age time.Duration, hasAge bool) string {
	reason := "turn in progress"
	if detail != "" {
		reason += ": " + detail
	}
	if hasAge {
		reason += " (" + shortAge(age) + " ago)"
	}
	return reason
}

func completedReason(at *time.Time, now time.Time) string {
	if at == nil {
		return "turn ended; waiting for next step"
	}
	d := now.Sub(*at)
	if d < 0 {
		d = 0
	}
	return fmt.Sprintf("turn ended %s ago; waiting for next step", shortAge(d))
}

func ageOf(at *time.Time, now time.Time) (time.Duration, bool) {
	if at == nil || at.IsZero() {
		return 0, false
	}
	age := now.Sub(*at)
	if age < 0 {
		return 0, true
	}
	return age, true
}

func shortAge(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func sessionTokens(s model.Session) *int64 {
	if !s.HasTokenUsage || s.TokenUsage == nil {
		return nil
	}
	u := s.TokenUsage
	switch {
	case u.TotalTokens != nil:
		v := *u.TotalTokens
		return &v
	case u.InputTokens != nil && u.OutputTokens != nil:
		v := *u.InputTokens + *u.OutputTokens
		return &v
	case u.InputTokens != nil:
		v := *u.InputTokens
		return &v
	case u.OutputTokens != nil:
		v := *u.OutputTokens
		return &v
	default:
		return nil
	}
}

func timePtr(t time.Time) *time.Time {
	ts := t
	return &ts
}
