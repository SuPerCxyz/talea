package timeline

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
)

func TestByModel(t *testing.T) {
	ctx := context.Background()
	db := indexOpen(t)
	base := time.Now()
	mk := func(modelName string, seq int64, total int64) *model.UsageTimelineEvent {
		return &model.UsageTimelineEvent{
			AgentInstanceID: "i", SessionID: "s",
			EventType: model.UsageEventRequest, Model: modelName,
			Timestamp: &base, Sequence: seq, TotalTokens: int64p(total),
			SourceIdentity: "m-" + modelName + "-" + itoa(seq),
		}
	}
	db.UpsertTimelineEvent(ctx, mk("model-a", 0, 100))
	db.UpsertTimelineEvent(ctx, mk("model-a", 1, 200))
	db.UpsertTimelineEvent(ctx, mk("model-b", 2, 300))

	sums, err := ByModel(ctx, db, "i", "s")
	if err != nil {
		t.Fatal(err)
	}
	if len(sums) != 2 {
		t.Fatalf("models=%d", len(sums))
	}
	for _, s := range sums {
		if s.Model == "model-a" && s.Requests != 2 {
			t.Fatalf("model-a requests=%d", s.Requests)
		}
		if s.Model == "model-a" && s.TotalTokens != 300 {
			t.Fatalf("model-a total=%d", s.TotalTokens)
		}
	}
}

func TestContextCurveDownsample(t *testing.T) {
	ctx := context.Background()
	db := indexOpen(t)
	base := time.Now()
	for i := 0; i < 50; i++ {
		ts := base.Add(time.Duration(i) * time.Second)
		e := &model.UsageTimelineEvent{
			AgentInstanceID: "i", SessionID: "s",
			EventType: model.UsageEventRequest,
			Timestamp: &ts, Sequence: int64(i),
			ContextAfter:   int64p(int64(i) * 1000),
			SourceIdentity: "c-" + itoa(int64(i)),
		}
		db.UpsertTimelineEvent(ctx, e)
	}
	pts, err := ContextCurve(ctx, db, "i", "s", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pts) > 10 {
		t.Fatalf("downsample failed: %d points", len(pts))
	}
	if len(pts) == 0 {
		t.Fatal("no points")
	}
}

func TestDetectCompactions(t *testing.T) {
	ctx := context.Background()
	db := indexOpen(t)
	base := time.Now()
	// 构造上下文下降序列
	ctxs := []int64{100_000, 110_000, 20_000, 25_000, 130_000, 15_000}
	for i, c := range ctxs {
		ts := base.Add(time.Duration(i) * time.Minute)
		db.UpsertTimelineEvent(ctx, &model.UsageTimelineEvent{
			AgentInstanceID: "i", SessionID: "s",
			EventType: model.UsageEventRequest,
			Timestamp: &ts, Sequence: int64(i),
			ContextAfter:   int64p(c),
			SourceIdentity: "d-" + itoa(int64(i)),
		})
	}
	comps, err := DetectCompactions(ctx, db, "i", "s")
	if err != nil {
		t.Fatal(err)
	}
	// 110k->20k 与 25k->130k 是增长，130k->15k 是压缩
	// 110k->20k: 90k 下降, ratio 81% ✓
	// 130k->15k: 115k 下降, ratio 88% ✓
	if len(comps) != 2 {
		t.Fatalf("compactions=%d", len(comps))
	}
	for _, c := range comps {
		if !c.IsInferred {
			t.Fatal("expected inferred flag")
		}
	}
}

func indexOpen(t *testing.T) *index.DB {
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

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}
