package timeline

import (
	"context"
	"time"

	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
)

// Bucket 是时间桶聚合结果。
type Bucket struct {
	Start        time.Time
	End          time.Time
	Requests     int64
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
	CacheRead    int64
	Reasoning    int64
}

// BucketSize 支持的时间桶。
type BucketSize string

const (
	BucketRequest BucketSize = "request"
	Bucket1m      BucketSize = "1m"
	Bucket5m      BucketSize = "5m"
	Bucket15m     BucketSize = "15m"
	Bucket1h      BucketSize = "1h"
)

// BucketDuration 返回桶的时长。request 返回 0（按请求聚合）。
func BucketDuration(size BucketSize) time.Duration {
	switch size {
	case Bucket1m:
		return time.Minute
	case Bucket5m:
		return 5 * time.Minute
	case Bucket15m:
		return 15 * time.Minute
	case Bucket1h:
		return time.Hour
	default:
		return 0
	}
}

// GroupByBucket 将 request 事件按时间桶聚合。
// size 为 request 时返回逐请求明细。
func GroupByBucket(ctx context.Context, db *index.DB, instanceID, sessionID string, size BucketSize) ([]Bucket, error) {
	events, err := List(ctx, db, Query{AgentInstanceID: instanceID, SessionID: sessionID, Limit: 100000})
	if err != nil {
		return nil, err
	}

	if size == BucketRequest {
		var out []Bucket
		for _, e := range events {
			if e.EventType != model.UsageEventRequest {
				continue
			}
			b := Bucket{Requests: 1}
			if e.Timestamp != nil {
				b.Start = *e.Timestamp
				b.End = *e.Timestamp
			}
			b.InputTokens = value(e.InputTokens)
			b.OutputTokens = value(e.OutputTokens)
			b.TotalTokens = value(e.TotalTokens)
			b.CacheRead = value(e.CacheReadTokens)
			b.Reasoning = value(e.ReasoningTokens)
			out = append(out, b)
		}
		return out, nil
	}

	dur := BucketDuration(size)
	if dur == 0 {
		dur = time.Minute
	}
	buckets := make([]Bucket, 0, 16)
	var (
		cur    *Bucket
		curKey int64
	)
	for _, e := range events {
		if e.EventType != model.UsageEventRequest || e.Timestamp == nil {
			continue
		}
		key := e.Timestamp.Unix() / int64(dur.Seconds())
		if cur == nil || key != curKey {
			start := time.Unix(key*int64(dur.Seconds()), 0)
			b := Bucket{Start: start, End: start.Add(dur)}
			buckets = append(buckets, b)
			cur = &buckets[len(buckets)-1]
			curKey = key
		}
		cur.Requests++
		cur.InputTokens += value(e.InputTokens)
		cur.OutputTokens += value(e.OutputTokens)
		cur.TotalTokens += value(e.TotalTokens)
		cur.CacheRead += value(e.CacheReadTokens)
		cur.Reasoning += value(e.ReasoningTokens)
	}
	return buckets, nil
}

func value(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
