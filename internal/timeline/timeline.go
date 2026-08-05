// Package timeline 提供 Token 时间线查询与聚合。
package timeline

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
)

// Event 是查询返回的时间线事件（含可选 usage）。
type Event struct {
	model.UsageTimelineEvent
	DurationMillis *int64
}

// Query 描述时间线查询条件。
type Query struct {
	AgentInstanceID string
	SessionID       string
	Limit           int
	Offset          int
}

// List 查询时间线事件（分页）。
func List(ctx context.Context, db *index.DB, q Query) ([]Event, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 1000
	}
	rows, err := db.SQL().QueryContext(ctx, `
		SELECT event_id, event_type, timestamp, sequence, duration_ms,
		       request_id, response_id, message_id, parent_message_id,
		       tool_call_id, subagent_id, model, provider,
		       input_tokens, output_tokens, total_tokens,
		       cache_read_tokens, cache_write_tokens, reasoning_tokens, tool_tokens,
		       context_before, context_after, context_limit,
		       cumulative_input, cumulative_output, cumulative_total,
		       user_prompt_preview, tool_name, file_path, command_preview,
		       value_mode, usage_source, completeness, is_estimated, source_identity, raw_fields_json
		FROM usage_timeline_events
		WHERE agent_instance_id=? AND session_id=?
		ORDER BY timestamp, sequence
		LIMIT ? OFFSET ?`, q.AgentInstanceID, q.SessionID, limit, q.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Event
	for rows.Next() {
		var e Event
		var (
			ts, dur, in, ot, tot         sql.NullInt64
			cacheR, cacheW, reason, tool sql.NullInt64
			ctxB, ctxA, ctxL             sql.NullInt64
			cumIn, cumOut, cumTot        sql.NullInt64
			estimated                    int
			raw, sourceIdentity          string
		)
		if err := rows.Scan(&e.EventID, &e.EventType, &ts, &e.Sequence, &dur,
			&e.RequestID, &e.ResponseID, &e.MessageID, &e.ParentMessageID,
			&e.ToolCallID, &e.SubagentID, &e.Model, &e.Provider,
			&in, &ot, &tot, &cacheR, &cacheW, &reason, &tool,
			&ctxB, &ctxA, &ctxL, &cumIn, &cumOut, &cumTot,
			&e.UserPromptPreview, &e.ToolName, &e.FilePath, &e.CommandPreview,
			&e.ValueMode, &e.Source, &e.Completeness, &estimated, &sourceIdentity, &raw); err != nil {
			continue
		}
		if ts.Valid {
			t := time.Unix(ts.Int64, 0)
			e.Timestamp = &t
		}
		if dur.Valid {
			d := time.Duration(dur.Int64) * time.Millisecond
			e.Duration = &d
			e.DurationMillis = &dur.Int64
		}
		e.InputTokens = nn(in)
		e.OutputTokens = nn(ot)
		e.TotalTokens = nn(tot)
		e.CacheReadTokens = nn(cacheR)
		e.CacheWriteTokens = nn(cacheW)
		e.ReasoningTokens = nn(reason)
		e.ToolTokens = nn(tool)
		e.ContextBefore = nn(ctxB)
		e.ContextAfter = nn(ctxA)
		e.ContextLimit = nn(ctxL)
		e.CumulativeInput = nn(cumIn)
		e.CumulativeOutput = nn(cumOut)
		e.CumulativeTotal = nn(cumTot)
		e.IsEstimated = estimated != 0
		e.SourceIdentity = sourceIdentity
		if raw != "" {
			_ = json.Unmarshal([]byte(raw), &e.RawFields)
		}
		out = append(out, e)
	}
	return out, nil
}

func nn(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	x := v.Int64
	return &x
}

// Summary 是时间线聚合汇总。
type Summary struct {
	RequestCount int64
	TotalTokens  int64
	InputTokens  int64
	OutputTokens int64
	PeakContext  int64
	// 会话级真实累计（取最后一个事件的累计值，若存在）
	CumulativeTotal int64
}

// Aggregate 聚合指定会话的请求级 usage。
func Aggregate(ctx context.Context, db *index.DB, instanceID, sessionID string) (Summary, error) {
	var s Summary
	row := db.SQL().QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(COALESCE(total_tokens,0)),0),
		       COALESCE(SUM(COALESCE(input_tokens,0)),0),
		       COALESCE(SUM(COALESCE(output_tokens,0)),0),
		       COALESCE(MAX(COALESCE(context_after,0)),0)
		FROM usage_timeline_events
		WHERE agent_instance_id=? AND session_id=?`, instanceID, sessionID)
	err := row.Scan(&s.RequestCount, &s.TotalTokens, &s.InputTokens, &s.OutputTokens, &s.PeakContext)
	if err != nil {
		return s, err
	}
	// 会话真实累计：取最后一条 request 事件的累计上下文
	// （total_tokens 为上下文快照，或 context_after）
	var lastTotal sql.NullInt64
	row = db.SQL().QueryRowContext(ctx, `
		SELECT COALESCE(total_tokens, context_after) FROM usage_timeline_events
		WHERE agent_instance_id=? AND session_id=? AND event_type='request'
		ORDER BY timestamp DESC, sequence DESC LIMIT 1`, instanceID, sessionID)
	if err := row.Scan(&lastTotal); err == nil && lastTotal.Valid {
		s.CumulativeTotal = lastTotal.Int64
	}
	return s, nil
}
