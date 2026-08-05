package timeline

import (
	"context"

	"github.com/talea/talea/internal/index"
)

// BucketMetrics 描述时间桶聚合指标（spec §14.8）。
type BucketMetrics struct {
	// 每桶 Token 速率（Token/分钟）
	TokenPerMinute float64
	// 累计 Token
	CumulativeTotal int64
	// 输入/输出占比
	InputShare  float64
	OutputShare float64
	// 缓存利用率（缓存读 / 输入）
	CacheUtilization float64
	// 请求数
	Requests int64
}

// Metrics 是会话级 Token 指标汇总。
type Metrics struct {
	DurationSeconds  int64
	TokenPerMinute   float64
	CumulativeTotal  int64
	InputTokens      int64
	OutputTokens     int64
	CacheRead        int64
	CacheUtilization float64
	InputShare       float64
	OutputShare      float64
	Requests         int64

	// 模型占比（模型名 -> 总 Token）
	ModelShare map[string]int64
	// 子 Agent 占比
	SubagentShare float64
}

// ComputeMetrics 计算会话 Token 指标。
func ComputeMetrics(ctx context.Context, db *index.DB, instanceID, sessionID string) (Metrics, error) {
	var m Metrics
	m.ModelShare = map[string]int64{}

	// 时间跨度
	var minTS, maxTS *int64
	row := db.SQL().QueryRowContext(ctx, `
		SELECT MIN(timestamp), MAX(timestamp), COUNT(*),
		       COALESCE(SUM(COALESCE(input_tokens,0)),0),
		       COALESCE(SUM(COALESCE(output_tokens,0)),0),
		       COALESCE(SUM(COALESCE(cache_read_tokens,0)),0)
		FROM usage_timeline_events
		WHERE agent_instance_id=? AND session_id=? AND event_type='request'`,
		instanceID, sessionID)
	if err := row.Scan(&minTS, &maxTS, &m.Requests, &m.InputTokens, &m.OutputTokens, &m.CacheRead); err != nil {
		return m, err
	}
	if minTS != nil && maxTS != nil && *maxTS > *minTS {
		m.DurationSeconds = *maxTS - *minTS
		if m.DurationSeconds > 0 {
			m.TokenPerMinute = float64(m.InputTokens+m.OutputTokens) / (float64(m.DurationSeconds) / 60)
		}
	}
	m.CumulativeTotal = m.InputTokens + m.OutputTokens
	if m.InputTokens+m.OutputTokens > 0 {
		m.InputShare = float64(m.InputTokens) / float64(m.InputTokens+m.OutputTokens)
		m.OutputShare = float64(m.OutputTokens) / float64(m.InputTokens+m.OutputTokens)
	}
	if m.InputTokens > 0 {
		m.CacheUtilization = float64(m.CacheRead) / float64(m.InputTokens)
	}

	// 模型占比
	rows, err := db.SQL().QueryContext(ctx, `
		SELECT COALESCE(NULLIF(model,''),'未知'), COALESCE(SUM(COALESCE(total_tokens,0)),0)
		FROM usage_timeline_events
		WHERE agent_instance_id=? AND session_id=? AND event_type='request'
		GROUP BY COALESCE(NULLIF(model,''),'未知')`, instanceID, sessionID)
	if err != nil {
		return m, err
	}
	defer rows.Close()
	for rows.Next() {
		var modelName string
		var total int64
		if err := rows.Scan(&modelName, &total); err != nil {
			continue
		}
		m.ModelShare[modelName] = total
	}
	return m, nil
}
