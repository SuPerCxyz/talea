package timeline

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
)

func newTestDB(t *testing.T) *index.DB {
	t.Helper()
	db, err := index.Open(t.TempDir() + "/index.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return db
}

func insertUserEvent(t *testing.T, db *index.DB, iid, sid, preview string, ts int64, seq int64) {
	t.Helper()
	ctx := context.Background()
	ev := &model.UsageTimelineEvent{
		AgentInstanceID:  iid,
		SessionID:        sid,
		EventType:        model.UsageEventUserMessage,
		Timestamp:        timep(time.Unix(ts, 0)),
		Sequence:         seq,
		UserPromptPreview: preview,
		SourceIdentity:   fmt.Sprintf("u-%s-%d", iid, ts), // 每条消息唯一，避免 UNIQUE 冲突
	}
	if _, err := db.UpsertTimelineEvents(ctx, []*model.UsageTimelineEvent{ev}); err != nil {
		t.Fatal(err)
	}
}

func TestLastUserPromptBySession(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	// 会话 A：两条用户消息，应取最后一条
	insertUserEvent(t, db, "inst-a", "s1", "第一条", 100, 1)
	insertUserEvent(t, db, "inst-a", "s1", "第二条（最后）", 200, 2)

	// 会话 B：一条用户消息
	insertUserEvent(t, db, "inst-b", "s2", "唯一", 300, 1)

	// 会话 C：无用户消息（只有 request 事件）
	reqEv := &model.UsageTimelineEvent{
		AgentInstanceID: "inst-c", SessionID: "s3",
		EventType:      model.UsageEventRequest,
		Timestamp:      timep(time.Unix(400, 0)),
		Sequence:       1,
		SourceIdentity: "req-c",
	}
	if _, err := db.UpsertTimelineEvents(ctx, []*model.UsageTimelineEvent{reqEv}); err != nil {
		t.Fatal(err)
	}

	// 跨实例相同 session_id
	insertUserEvent(t, db, "inst-a", "dup", "A的", 100, 1)
	insertUserEvent(t, db, "inst-b", "dup", "B的", 500, 1)

	keys := [][2]string{
		{"inst-a", "s1"},
		{"inst-b", "s2"},
		{"inst-c", "s3"},
		{"inst-a", "dup"},
		{"inst-b", "dup"},
	}
	prompts, err := LastUserPromptBySession(ctx, db, keys)
	if err != nil {
		t.Fatal(err)
	}

	cases := map[string]string{
		"inst-a\x00s1":  "第二条（最后）",
		"inst-b\x00s2":  "唯一",
		"inst-a\x00dup": "A的",
		"inst-b\x00dup": "B的",
	}
	for k, want := range cases {
		if got := prompts[k]; got != want {
			t.Errorf("%s: got %q want %q", k, got, want)
		}
	}
	// 无用户消息的会话不应出现在结果中
	if _, ok := prompts["inst-c\x00s3"]; ok {
		t.Errorf("会话 c 不应有 last prompt")
	}
	// 空 keys
	empty, err := LastUserPromptBySession(ctx, db, nil)
	if err != nil || len(empty) != 0 {
		t.Errorf("空 keys 应返回空 map: %v %v", empty, err)
	}
}
