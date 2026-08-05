package insights

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
)

func TestGenerateDetectsGrowth(t *testing.T) {
	ctx := context.Background()
	db, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	base := time.Now()
	// 构造快速增长的请求序列
	for i := 0; i < 15; i++ {
		ts := base.Add(time.Duration(i) * time.Second)
		total := int64(100_000 + i*60_000) // 每次增长 60k
		db.UpsertTimelineEvent(ctx, &model.UsageTimelineEvent{
			AgentInstanceID: "i", SessionID: "s",
			EventType: model.UsageEventRequest,
			Timestamp: &ts, Sequence: int64(i),
			TotalTokens:    &total,
			SourceIdentity: "g-" + itoa(int64(i)),
		})
	}
	// 插入一个远超 P95 的尖峰请求
	spike := int64(10_000_000)
	spikeTS := base.Add(20 * time.Second)
	db.UpsertTimelineEvent(ctx, &model.UsageTimelineEvent{
		AgentInstanceID: "i", SessionID: "s",
		EventType: model.UsageEventRequest,
		Timestamp: &spikeTS, Sequence: 100,
		TotalTokens:    &spike,
		SourceIdentity: "spike",
	})
	rep, err := Generate(ctx, db, "i", "s")
	if err != nil {
		t.Fatal(err)
	}
	foundGrowth := false
	foundHigh := false
	for _, ins := range rep.Insights {
		if ins.Type == "growth" {
			foundGrowth = true
		}
		if ins.Type == "high-request" {
			foundHigh = true
		}
	}
	if !foundGrowth {
		t.Fatalf("expected growth insight, got %+v", rep.Insights)
	}
	if !foundHigh {
		t.Fatalf("expected high-request insight, got %+v", rep.Insights)
	}
}

func TestGenerateEmpty(t *testing.T) {
	ctx := context.Background()
	db, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	rep, err := Generate(ctx, db, "i", "s")
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Insights) != 0 {
		t.Fatalf("expected no insights, got %+v", rep.Insights)
	}
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
