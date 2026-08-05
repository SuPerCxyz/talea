package timeline

import (
	"context"
	"time"

	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
)

// TurnUsage 是一次用户轮次的聚合结果。
type TurnUsage struct {
	Index     int
	Prompt    string
	StartedAt time.Time
	EndedAt   time.Time
	Requests  int
	Total     int64
	Input     int64
	Output    int64
	Reasoning int64
}

// GroupByTurns 将时间线事件聚合为用户轮次。
// 一条 user_message 事件开启新轮次；其后的 request 事件计入当前轮次。
func GroupByTurns(ctx context.Context, db *index.DB, instanceID, sessionID string) ([]TurnUsage, error) {
	events, err := List(ctx, db, Query{AgentInstanceID: instanceID, SessionID: sessionID, Limit: 100000})
	if err != nil {
		return nil, err
	}
	var turns []TurnUsage
	cur := &TurnUsage{Index: 0}
	for _, e := range events {
		if e.EventType == model.UsageEventUserMessage {
			if !cur.StartedAt.IsZero() {
				cur.EndedAt = nowTime(e.Timestamp)
				turns = append(turns, *cur)
			}
			cur = &TurnUsage{Index: len(turns) + 1}
			if e.Timestamp != nil {
				cur.StartedAt = *e.Timestamp
			}
			cur.Prompt = firstLine(e.UserPromptPreview)
			continue
		}
		if e.EventType == model.UsageEventRequest {
			cur.Requests++
			if e.TotalTokens != nil {
				cur.Total += *e.TotalTokens
			}
			if e.InputTokens != nil {
				cur.Input += *e.InputTokens
			}
			if e.OutputTokens != nil {
				cur.Output += *e.OutputTokens
			}
			if e.ReasoningTokens != nil {
				cur.Reasoning += *e.ReasoningTokens
			}
			if e.Timestamp != nil {
				cur.EndedAt = *e.Timestamp
			}
		}
	}
	if !cur.StartedAt.IsZero() {
		turns = append(turns, *cur)
	}
	return turns, nil
}

func firstLine(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return s[:i]
		}
	}
	return s
}

func nowTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}
