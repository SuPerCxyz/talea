package status

import (
	"testing"
	"time"

	"github.com/talea/talea/internal/i18n"
	"github.com/talea/talea/internal/model"
)

func testTime() time.Time { return time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC) }

func testSession(last time.Time, activity model.ActivityState) model.Session {
	return model.Session{
		AgentID:         model.AgentClaudeCode,
		AgentInstanceID: "inst",
		SessionID:       "sess",
		ProjectName:     "proj",
		LastActivityAt:  &last,
		Activity:        activity,
	}
}

func testRecord(state model.TurnState, source model.TurnStateSource, at time.Time) model.TurnStateRecord {
	return model.TurnStateRecord{
		AgentInstanceID: "inst",
		SessionID:       "sess",
		State:           state,
		Source:          source,
		StateAt:         &at,
	}
}

func TestEvaluateCompletedWindow(t *testing.T) {
	i18n.Set(i18n.LangEn)
	t.Cleanup(func() { i18n.Set(i18n.LangEn) })
	now := testTime()
	ss := Evaluate(testSession(now.Add(-time.Hour), model.ActivityInactive),
		testRecord(model.TurnStateCompleted, model.TurnStateSourceNative, now.Add(-time.Hour)), nil, now)
	if ss.State != StateCompleted || ss.NeedsAttention {
		t.Fatalf("state=%q attention=%v", ss.State, ss.NeedsAttention)
	}
	if ss.Label != "Turn ended" {
		t.Fatalf("label=%q", ss.Label)
	}
	if ss.Confidence != ConfidenceExact {
		t.Fatalf("confidence=%q", ss.Confidence)
	}

	old := Evaluate(testSession(now.Add(-25*time.Hour), model.ActivityInactive),
		testRecord(model.TurnStateCompleted, model.TurnStateSourceNative, now.Add(-25*time.Hour)), nil, now)
	if old.State != StateIdle || old.Source != model.TurnStateSourceInferred ||
		old.Confidence != ConfidenceInferred {
		t.Fatalf("过期完成: state=%q source=%q confidence=%q", old.State, old.Source, old.Confidence)
	}
}

func TestEvaluateWorkingThresholds(t *testing.T) {
	now := testTime()
	cases := []struct {
		name  string
		age   time.Duration
		state DisplayState
		conf  Confidence
	}{
		{"quiet threshold exact", QuietThreshold, StateWorking, ConfidenceExact},
		{"over quiet", QuietThreshold + time.Second, StateWaiting, ConfidenceInferred},
		{"active window exact", ActiveWindow, StateWaiting, ConfidenceInferred},
		{"over active window", ActiveWindow + time.Second, StateIdle, ConfidenceInferred},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			at := now.Add(-tc.age)
			ss := Evaluate(testSession(at, model.ActivityInactive),
				testRecord(model.TurnStateWorking, model.TurnStateSourceNative, at), nil, now)
			if ss.State != tc.state || ss.Confidence != tc.conf {
				t.Fatalf("state=%q confidence=%q reason=%q", ss.State, ss.Confidence, ss.Reason)
			}
			wantAttention := tc.state == StateWaiting
			if ss.NeedsAttention != wantAttention {
				t.Fatalf("attention=%v want=%v", ss.NeedsAttention, wantAttention)
			}
		})
	}
}

func TestEvaluateWaitingAndFailure(t *testing.T) {
	now := testTime()
	at := now.Add(-time.Minute)
	waiting := Evaluate(testSession(at, model.ActivityInactive),
		testRecord(model.TurnStateWaitingInput, model.TurnStateSourceNative, at), nil, now)
	if waiting.State != StateWaiting || !waiting.NeedsAttention || waiting.Confidence != ConfidenceExact {
		t.Fatalf("waiting=%+v", waiting)
	}
	failed := Evaluate(testSession(at, model.ActivityInactive),
		testRecord(model.TurnStateFailed, model.TurnStateSourceNative, at), nil, now)
	if failed.State != StateFailed || !failed.NeedsAttention {
		t.Fatalf("failed=%+v", failed)
	}
	interrupted := Evaluate(testSession(at, model.ActivityInactive),
		testRecord(model.TurnStateInterrupted, model.TurnStateSourceNative, at), nil, now)
	if interrupted.State != StateFailed || !interrupted.NeedsAttention {
		t.Fatalf("interrupted=%+v", interrupted)
	}
	if interrupted.Label != "Interrupted" {
		t.Fatalf("label=%q", interrupted.Label)
	}
}

func TestEvaluateUnknownDoesNotBecomeCompleted(t *testing.T) {
	now := testTime()
	ss := Evaluate(testSession(now.Add(-time.Hour), model.ActivityInactive),
		model.TurnStateRecord{}, nil, now)
	if ss.State != StateUnknown || ss.Confidence != ConfidenceUnknown {
		t.Fatalf("state=%q confidence=%q", ss.State, ss.Confidence)
	}

	active := Evaluate(testSession(now.Add(-time.Minute), model.ActivityActive),
		model.TurnStateRecord{}, nil, now)
	if active.State != StateUnknown || active.Source != model.TurnStateSourceUnknown {
		t.Fatalf("无回合事件必须保持未知: state=%q source=%q", active.State, active.Source)
	}
	if !containsAny(active.Reason, "agent process active") {
		t.Fatalf("未知原因未包含活动解释: %q", active.Reason)
	}
}

func TestEvaluateUsesSessionTurnFieldsWhenRecordMissing(t *testing.T) {
	now := testTime()
	at := now.Add(-time.Minute)
	sess := testSession(at, model.ActivityInactive)
	sess.TurnState = model.TurnStateCompleted
	sess.TurnStateAt = &at
	sess.TurnStateSource = model.TurnStateSourceNative
	ss := Evaluate(sess, model.TurnStateRecord{}, nil, now)
	if ss.State != StateCompleted || ss.Source != model.TurnStateSourceNative {
		t.Fatalf("state=%q source=%q", ss.State, ss.Source)
	}
}

func TestEvaluateHookOverridesOnlyWhenNotOlder(t *testing.T) {
	now := testTime()
	stateAt := now.Add(-5 * time.Minute)
	rec := testRecord(model.TurnStateCompleted, model.TurnStateSourceNative, stateAt)

	newer := Event{State: model.TurnStateWaitingInput, At: now.Add(-time.Minute), Detail: "permission_prompt"}
	ss := Evaluate(testSession(stateAt, model.ActivityInactive), rec, &newer, now)
	if ss.TurnState != model.TurnStateWaitingInput || ss.Source != model.TurnStateSourceHook {
		t.Fatalf("新事件未覆盖: state=%q source=%q", ss.TurnState, ss.Source)
	}
	if ss.Confidence != ConfidenceExact || !ss.NeedsAttention {
		t.Fatalf("hook confidence=%q attention=%v", ss.Confidence, ss.NeedsAttention)
	}

	older := Event{State: model.TurnStateWaitingInput, At: now.Add(-10 * time.Minute)}
	ss = Evaluate(testSession(stateAt, model.ActivityInactive), rec, &older, now)
	if ss.TurnState != model.TurnStateCompleted || ss.Source != model.TurnStateSourceNative {
		t.Fatalf("旧事件覆盖了新状态: state=%q source=%q", ss.TurnState, ss.Source)
	}
}

func TestEvaluateNoStateTimeUsesLastActivity(t *testing.T) {
	now := testTime()
	last := now.Add(-5 * time.Minute)
	rec := model.TurnStateRecord{State: model.TurnStateWorking, Source: model.TurnStateSourceNative}
	ss := Evaluate(testSession(last, model.ActivityInactive), rec, nil, now)
	if ss.State != StateWaiting || ss.Confidence != ConfidenceInferred {
		t.Fatalf("state=%q confidence=%q", ss.State, ss.Confidence)
	}
}

func TestCountOf(t *testing.T) {
	sessions := []SessionStatus{
		{State: StateWaiting}, {State: StateWaiting}, {State: StateWorking},
		{State: StateCompleted}, {State: StateFailed}, {State: StateIdle}, {State: StateUnknown},
	}
	c := countOf(sessions)
	if c.Total != 7 || c.NeedsAttention != 3 || c.Waiting != 2 || c.Working != 1 ||
		c.Completed != 1 || c.Failed != 1 || c.Idle != 1 || c.Unknown != 1 {
		t.Fatalf("counts=%+v", c)
	}
}

func TestShortIsSingleLine(t *testing.T) {
	s := Snapshot{Counts: Counts{NeedsAttention: 2, Working: 1, Completed: 3, Failed: 0, Idle: 4, Unknown: 0, Total: 10}}
	short := s.Short()
	if len(short) == 0 || containsAny(short, "\n", "\r") {
		t.Fatalf("short 不是单行: %q", short)
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(sub) > 0 && len(s) >= len(sub) {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}
