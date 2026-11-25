package status

import (
	"testing"
	"time"

	"github.com/talea/talea/internal/model"
)

func TestEventFromClaudePermissionNotification(t *testing.T) {
	raw := []byte(`{"session_id":"s1","hook_event_name":"Notification","notification_type":"permission_prompt"}`)
	event, err := EventFromHook(raw, testTime())
	if err != nil {
		t.Fatal(err)
	}
	if event.AgentID != string(model.AgentClaudeCode) || event.SessionID != "s1" {
		t.Fatalf("event=%+v", event)
	}
	if event.State != model.TurnStateWaitingInput || event.At != testTime() {
		t.Fatalf("state=%q at=%v", event.State, event.At)
	}
}

func TestEventFromClaudePermissionRequest(t *testing.T) {
	raw := []byte(`{"session_id":"s1","hook_event_name":"PermissionRequest","tool_name":"Bash"}`)
	event, err := EventFromHook(raw, testTime())
	if err != nil {
		t.Fatal(err)
	}
	if event.State != model.TurnStateWaitingInput || event.AgentID != string(model.AgentClaudeCode) {
		t.Fatalf("event=%+v", event)
	}
	if event.Detail != "permission request" {
		t.Fatalf("detail=%q", event.Detail)
	}
}

func TestEventFromClaudeStopFailure(t *testing.T) {
	raw := []byte(`{"session_id":"s1","hook_event_name":"StopFailure","reason":"api error"}`)
	event, err := EventFromHook(raw, testTime())
	if err != nil {
		t.Fatal(err)
	}
	if event.State != model.TurnStateFailed {
		t.Fatalf("event=%+v", event)
	}
}

func TestEventFromClaudeNotificationWithoutType(t *testing.T) {
	raw := []byte(`{"session_id":"s1","hook_event_name":"Notification"}`)
	event, err := EventFromHook(raw, testTime())
	if err != nil {
		t.Fatal(err)
	}
	if event.State != model.TurnStateWaitingInput || event.Detail != "notification" {
		t.Fatalf("event=%+v", event)
	}
}

func TestEventFromClaudeNotificationAuthSuccessIgnored(t *testing.T) {
	raw := []byte(`{"session_id":"s1","hook_event_name":"Notification","notification_type":"auth_success"}`)
	if event, err := EventFromHook(raw, testTime()); err == nil {
		t.Fatalf("auth_success 不应映射为等待: %+v", event)
	}
}

func TestEventFromClaudeStop(t *testing.T) {
	raw := []byte(`{"session_id":"s1","hook_event_name":"Stop","sessionId":"ignored"}`)
	event, err := EventFromHook(raw, testTime())
	if err != nil {
		t.Fatal(err)
	}
	if event.State != model.TurnStateCompleted || event.AgentID != string(model.AgentClaudeCode) {
		t.Fatalf("event=%+v", event)
	}
}

func TestEventFromOpenCodeIdleOutcome(t *testing.T) {
	raw := []byte(`{"agent":"opencode","session_id":"ses_1","event":"session.idle","outcome":"failed"}`)
	event, err := EventFromHook(raw, testTime())
	if err != nil {
		t.Fatal(err)
	}
	if event.AgentID != string(model.AgentOpenCode) || event.State != model.TurnStateFailed {
		t.Fatalf("event=%+v", event)
	}
}

func TestEventFromRawOpenCodeSessionIdle(t *testing.T) {
	raw := []byte(`{"type":"session.idle","properties":{"sessionID":"ses_1"}}`)
	event, err := EventFromHook(raw, testTime())
	if err != nil {
		t.Fatal(err)
	}
	if event.AgentID != string(model.AgentOpenCode) || event.SessionID != "ses_1" {
		t.Fatalf("event=%+v", event)
	}
	if event.State != model.TurnStateCompleted {
		t.Fatalf("无 outcome 的 session.idle 应为本轮结束: %q", event.State)
	}
}

func TestEventFromRawOpenCodeIdleOutcome(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want model.TurnState
	}{
		{
			name: "properties outcome failed",
			raw:  `{"type":"session.idle","properties":{"sessionID":"ses_1","outcome":"failed"}}`,
			want: model.TurnStateFailed,
		},
		{
			name: "properties status interrupted",
			raw:  `{"type":"session.idle","properties":{"sessionId":"ses_1","status":"interrupted"}}`,
			want: model.TurnStateInterrupted,
		},
		{
			name: "unknown outcome falls back idle",
			raw:  `{"type":"session.idle","properties":{"sessionID":"ses_1","outcome":"mystery"}}`,
			want: model.TurnStateIdle,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			event, err := EventFromHook([]byte(tc.raw), testTime())
			if err != nil {
				t.Fatal(err)
			}
			if event.State != tc.want {
				t.Fatalf("state=%q want=%q", event.State, tc.want)
			}
		})
	}
}

func TestEventFromRawOpenCodePermissionRequest(t *testing.T) {
	raw := []byte(`{"type":"permission.asked","properties":{"info":{"id":"ses_info"}}}`)
	event, err := EventFromHook(raw, testTime())
	if err != nil {
		t.Fatal(err)
	}
	if event.AgentID != string(model.AgentOpenCode) || event.SessionID != "ses_info" ||
		event.State != model.TurnStateWaitingInput {
		t.Fatalf("event=%+v", event)
	}
}

func TestEventFromExplicitState(t *testing.T) {
	raw := []byte(`{"agent":"codex","session_id":"s2","state":"interrupted","detail":"ctrl-c","at":"2026-09-01T11:00:00Z"}`)
	event, err := EventFromHook(raw, testTime())
	if err != nil {
		t.Fatal(err)
	}
	if event.AgentID != string(model.AgentCodexCLI) || event.State != model.TurnStateInterrupted {
		t.Fatalf("event=%+v", event)
	}
	if event.At.Format(time.RFC3339) != "2026-09-01T11:00:00Z" {
		t.Fatalf("at=%v", event.At)
	}
}

func TestEventFromHookRejectsMalformedOrUnknown(t *testing.T) {
	cases := [][]byte{
		[]byte(`not-json`),
		[]byte(`{"agent":"claude-code","state":"working"}`),
		[]byte(`{"agent":"claude-code","session_id":"s","state":"unknown"}`),
		[]byte(`{"agent":"claude-code","session_id":"s","hook_event_name":"Mystery"}`),
		[]byte(`{"agent":"opencode","session_id":"s","event":"mystery"}`),
		[]byte(`{"agent":"claude-code","session_id":"s","hook_event_name":"Notification","notification_type":"auth_success"}`),
	}
	for _, raw := range cases {
		if _, err := EventFromHook(raw, testTime()); err == nil {
			t.Fatalf("应拒绝输入: %s", raw)
		}
	}
}

func TestEventFromHookDetailDoesNotStoreMessage(t *testing.T) {
	raw := []byte(`{"session_id":"s1","hook_event_name":"Notification","notification_type":"permission_prompt","message":"rm -rf /secret","title":"需要确认"}`)
	event, err := EventFromHook(raw, testTime())
	if err != nil {
		t.Fatal(err)
	}
	if event.Detail != "notification: permission_prompt" {
		t.Fatalf("detail=%q", event.Detail)
	}
}
