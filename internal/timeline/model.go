package timeline

import (
	"context"
	"database/sql"
	"sort"

	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
)

// ModelSummary 按模型汇总。
type ModelSummary struct {
	Model        string
	Requests     int64
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
	CacheRead    int64
	Reasoning    int64
}

// ByModel 按模型聚合 request 事件。
func ByModel(ctx context.Context, db *index.DB, instanceID, sessionID string) ([]ModelSummary, error) {
	rows, err := db.SQL().QueryContext(ctx, `
		SELECT COALESCE(NULLIF(model,''),'未知'), COUNT(*),
		       COALESCE(SUM(COALESCE(input_tokens,0)),0),
		       COALESCE(SUM(COALESCE(output_tokens,0)),0),
		       COALESCE(SUM(COALESCE(total_tokens,0)),0),
		       COALESCE(SUM(COALESCE(cache_read_tokens,0)),0),
		       COALESCE(SUM(COALESCE(reasoning_tokens,0)),0)
		FROM usage_timeline_events
		WHERE agent_instance_id=? AND session_id=? AND event_type='request'
		GROUP BY COALESCE(NULLIF(model,''),'未知')
		ORDER BY COUNT(*) DESC`, instanceID, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ModelSummary
	for rows.Next() {
		var m ModelSummary
		if err := rows.Scan(&m.Model, &m.Requests, &m.InputTokens, &m.OutputTokens,
			&m.TotalTokens, &m.CacheRead, &m.Reasoning); err != nil {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

// ContextPoint 是上下文窗口曲线上的一个采样点。
type ContextPoint struct {
	Timestamp   int64
	Context     int64
	ContextLimit int64
	Change      int64 // 与上一个采样点的差值
}

// ContextCurve 生成上下文窗口曲线采样（按 timestamp 排序）。
// context_after 为累计输入（OpenCode step-finish 语义）。
func ContextCurve(ctx context.Context, db *index.DB, instanceID, sessionID string, maxPoints int) ([]ContextPoint, error) {
	if maxPoints <= 0 {
		maxPoints = 100
	}
	rows, err := db.SQL().QueryContext(ctx, `
		SELECT timestamp, context_after, context_limit, total_tokens
		FROM usage_timeline_events
		WHERE agent_instance_id=? AND session_id=? AND event_type='request'
		ORDER BY timestamp ASC`, instanceID, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// 读取全部上下文点，之后按 maxPoints 抽样
	var pts []ContextPoint
	for rows.Next() {
		var (
			ts, ctxAfter, ctxLimit, total sql.NullInt64
		)
		if err := rows.Scan(&ts, &ctxAfter, &ctxLimit, &total); err != nil {
			continue
		}
		context := int64(0)
		if ctxAfter.Valid {
			context = ctxAfter.Int64
		} else if total.Valid {
			context = total.Int64
		}
		if ts.Valid {
			pts = append(pts, ContextPoint{Timestamp: ts.Int64, Context: context})
			if ctxLimit.Valid {
				pts[len(pts)-1].ContextLimit = ctxLimit.Int64
			}
		}
	}
	if len(pts) > maxPoints {
		pts = downsample(pts, maxPoints)
	}
	for i := range pts {
		if i > 0 {
			pts[i].Change = pts[i].Context - pts[i-1].Context
		}
	}
	return pts, nil
}

func downsample(pts []ContextPoint, n int) []ContextPoint {
	out := make([]ContextPoint, 0, n)
	step := float64(len(pts)) / float64(n)
	for i := 0; i < n; i++ {
		idx := int(float64(i) * step)
		if idx >= len(pts) {
			idx = len(pts) - 1
		}
		out = append(out, pts[idx])
	}
	return out
}

// CompactionEvent 描述一次上下文压缩事件。
type CompactionEvent struct {
	Timestamp  int64
	Before     int64
	After      int64
	Reduced    int64
	Ratio      float64
	IsInferred bool
}

// DetectCompactions 检测上下文压缩。
// 明确压缩：上下文显著下降（超过前值 40% 且下降量 > 10k）。
// 标记 IsInferred，因为原始数据未直接提供压缩记录。
func DetectCompactions(ctx context.Context, db *index.DB, instanceID, sessionID string) ([]CompactionEvent, error) {
	pts, err := ContextCurve(ctx, db, instanceID, sessionID, 0)
	if err != nil {
		return nil, err
	}
	var out []CompactionEvent
	for i := 1; i < len(pts); i++ {
		before := pts[i-1].Context
		after := pts[i].Context
		if before <= 0 || after >= before {
			continue
		}
		reduced := before - after
		if reduced < 10_000 {
			continue
		}
		ratio := float64(reduced) / float64(before)
		if ratio < 0.4 {
			continue
		}
		out = append(out, CompactionEvent{
			Timestamp:  pts[i].Timestamp,
			Before:     before,
			After:      after,
			Reduced:    reduced,
			Ratio:      ratio,
			IsInferred: true,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp < out[j].Timestamp })
	return out, nil
}

var _ = model.UsageEventRequest
