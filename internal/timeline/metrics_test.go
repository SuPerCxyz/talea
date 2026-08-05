package timeline

import (
	"context"
	"testing"
	"time"

	"github.com/talea/talea/internal/model"
)

func TestComputeMetrics(t *testing.T) {
	ctx := context.Background()
	db := indexOpen(t)
	base := time.Now()

	mk := func(seq int64, modelName string, input, output, cache int64) *model.UsageTimelineEvent {
		ts := base.Add(time.Duration(seq) * time.Minute)
		return &model.UsageTimelineEvent{
			AgentInstanceID: "i", SessionID: "s",
			EventType: model.UsageEventRequest,
			Timestamp: &ts, Sequence: seq,
			Model:           modelName,
			InputTokens:     int64p(input),
			OutputTokens:    int64p(output),
			CacheReadTokens: int64p(cache),
			TotalTokens:     int64p(input + output),
			SourceIdentity:  "m-" + itoa(seq),
		}
	}
	db.UpsertTimelineEvent(ctx, mk(0, "model-a", 1000, 100, 500))
	db.UpsertTimelineEvent(ctx, mk(1, "model-a", 2000, 200, 1000))
	db.UpsertTimelineEvent(ctx, mk(2, "model-b", 3000, 300, 1500))

	m, err := ComputeMetrics(ctx, db, "i", "s")
	if err != nil {
		t.Fatal(err)
	}
	if m.Requests != 3 {
		t.Fatalf("requests=%d", m.Requests)
	}
	if m.InputTokens != 6000 {
		t.Fatalf("input=%d", m.InputTokens)
	}
	if m.OutputTokens != 600 {
		t.Fatalf("output=%d", m.OutputTokens)
	}
	if m.CumulativeTotal != 6600 {
		t.Fatalf("cumulative=%d", m.CumulativeTotal)
	}
	// 输入占比 = 6000/6600 ≈ 90.9%
	if m.InputShare < 0.9 || m.InputShare > 0.92 {
		t.Fatalf("input share=%f", m.InputShare)
	}
	// 缓存利用率 = 3000/6000 = 50%
	if m.CacheUtilization < 0.49 || m.CacheUtilization > 0.51 {
		t.Fatalf("cache util=%f", m.CacheUtilization)
	}
	// 模型占比：model-a = 3300, model-b = 3300
	if m.ModelShare["model-a"] != 3300 || m.ModelShare["model-b"] != 3300 {
		t.Fatalf("model share=%v", m.ModelShare)
	}
}

func TestComputeMetricsEmpty(t *testing.T) {
	ctx := context.Background()
	db := indexOpen(t)
	m, err := ComputeMetrics(ctx, db, "i", "s")
	if err != nil {
		t.Fatal(err)
	}
	// 空库：零值 Metrics（MIN/MAX 为 NULL，聚合为 0）
	if m.Requests != 0 || m.CumulativeTotal != 0 {
		t.Fatalf("empty metrics: %+v", m)
	}
}
