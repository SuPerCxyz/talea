package timeline

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
)

func newDB(t *testing.T) *index.DB {
	t.Helper()
	db, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return db
}

func int64p(v int64) *int64 { return &v }

func ev(ts time.Time, seq int64, total *int64) *model.UsageTimelineEvent {
	return &model.UsageTimelineEvent{
		AgentInstanceID: "i",
		SessionID:       "s",
		EventType:       model.UsageEventRequest,
		Timestamp:       &ts,
		Sequence:        seq,
		TotalTokens:     total,
		SourceIdentity:  "ev-" + ts.String(),
	}
}

func TestAggregateCumulative(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	base := time.Now()
	in := int64p(100)
	out := int64p(50)
	// 两个请求，最后一个是累计值
	db.UpsertTimelineEvent(ctx, ev(base, 0, int64p(1000)))
	db.UpsertTimelineEvent(ctx, ev(base.Add(time.Second), 1, int64p(1200)))
	_ = in
	_ = out

	s, err := Aggregate(ctx, db, "i", "s")
	if err != nil {
		t.Fatal(err)
	}
	if s.RequestCount != 2 {
		t.Fatalf("requests=%d", s.RequestCount)
	}
	if s.CumulativeTotal != 1200 {
		t.Fatalf("cumulative=%d", s.CumulativeTotal)
	}
}

func TestDedupSameSourceIdentity(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	base := time.Now()
	e := ev(base, 0, int64p(500))
	e.SourceIdentity = "same-id"
	ok, _ := db.UpsertTimelineEvent(ctx, e)
	if !ok {
		t.Fatal("first insert expected ok")
	}
	ok, _ = db.UpsertTimelineEvent(ctx, e)
	if ok {
		t.Fatal("dup insert expected ignored")
	}
	events, err := List(ctx, db, Query{AgentInstanceID: "i", SessionID: "s"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events=%d", len(events))
	}
}

func TestGroupByTurns(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	base := time.Now()

	user := func(ts time.Time, seq int64, preview string) *model.UsageTimelineEvent {
		return &model.UsageTimelineEvent{
			AgentInstanceID:   "i",
			SessionID:         "s",
			EventType:         model.UsageEventUserMessage,
			Timestamp:         &ts,
			Sequence:          seq,
			UserPromptPreview: preview,
			SourceIdentity:    "u-" + ts.String(),
		}
	}
	db.UpsertTimelineEvent(ctx, user(base, 0, "第一个问题"))
	db.UpsertTimelineEvent(ctx, ev(base.Add(time.Minute), 1, int64p(800)))
	db.UpsertTimelineEvent(ctx, user(base.Add(2*time.Minute), 2, "第二个问题"))
	db.UpsertTimelineEvent(ctx, ev(base.Add(3*time.Minute), 3, int64p(1600)))

	turns, err := GroupByTurns(ctx, db, "i", "s")
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 {
		t.Fatalf("turns=%d", len(turns))
	}
	if turns[0].Prompt != "第一个问题" {
		t.Fatalf("turn0 prompt=%q", turns[0].Prompt)
	}
	if turns[0].Total != 800 {
		t.Fatalf("turn0 total=%d", turns[0].Total)
	}
	if turns[1].Total != 1600 {
		t.Fatalf("turn1 total=%d", turns[1].Total)
	}
}
