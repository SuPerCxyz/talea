package opencode

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/model"
)

// v2MessageRow 是一条 session_message 记录。
type v2MessageRow struct {
	id      string
	msgType string
	created int64
	raw     []byte
}

// iterateV2UsageEvents 从 session_message 生成时间线事件：user → 用户消息事件，
// compaction → 压缩事件，assistant → 工具事件与单次增量请求事件。
// v2 不存在 step-finish 上下文快照，TotalTokens/ContextAfter/CumulativeTotal 保持未知（空）。
func (a *Adapter) iterateV2UsageEvents(
	ctx context.Context,
	db *sql.DB,
	s model.Session,
) (adapters.UsageEventIterator, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, type, time_created, data FROM session_message
		 WHERE session_id = ? AND type IN (?, ?, ?)
		 ORDER BY time_created ASC, seq ASC`,
		s.SessionID, typeUser, typeAssistant, typeCompaction)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*model.UsageTimelineEvent
	var seq int64
	for rows.Next() {
		msg, ok := scanV2MessageRow(rows)
		if !ok {
			continue // 单行损坏不影响同一会话与其它会话
		}
		evs := v2MessageEvents(msg, s, seq)
		events = append(events, evs...)
		seq += int64(len(evs))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &eventIterator{events: events}, nil
}

// scanV2MessageRow 读取一行 session_message；扫描失败时返回 false 跳过该行。
func scanV2MessageRow(rows *sql.Rows) (v2MessageRow, bool) {
	var msg v2MessageRow
	var raw string
	if err := rows.Scan(&msg.id, &msg.msgType, &msg.created, &raw); err != nil {
		return v2MessageRow{}, false
	}
	msg.raw = []byte(raw)
	return msg, true
}

// v2MessageEvents 把单条 session_message 展开为时间线事件。
func v2MessageEvents(msg v2MessageRow, s model.Session, seq int64) []*model.UsageTimelineEvent {
	switch msg.msgType {
	case typeUser:
		return []*model.UsageTimelineEvent{v2UserEvent(msg, s, seq)}
	case typeCompaction:
		return []*model.UsageTimelineEvent{v2CompactionEvent(msg, s, seq)}
	case typeAssistant:
		return v2AssistantEvents(msg, s, seq)
	}
	return nil
}

// v2UserEvent 生成用户消息事件。
func v2UserEvent(msg v2MessageRow, s model.Session, seq int64) *model.UsageTimelineEvent {
	ts := time.UnixMilli(msg.created)
	return &model.UsageTimelineEvent{
		AgentInstanceID:   s.AgentInstanceID,
		SessionID:         s.SessionID,
		EventID:           "v2msg-" + msg.id,
		EventType:         model.UsageEventUserMessage,
		Timestamp:         &ts,
		Sequence:          seq,
		MessageID:         msg.id,
		UserPromptPreview: v2PreviewText(msg.msgType, msg.raw),
		Source:            model.UsageSourceMessageMetadata,
		Completeness:      model.UsageComplete,
		SourceIdentity:    v2MsgPrefix + msg.id,
	}
}

// v2CompactionEvent 生成上下文压缩事件。
func v2CompactionEvent(msg v2MessageRow, s model.Session, seq int64) *model.UsageTimelineEvent {
	ts := time.UnixMilli(msg.created)
	return &model.UsageTimelineEvent{
		AgentInstanceID: s.AgentInstanceID,
		SessionID:       s.SessionID,
		EventID:         "v2comp-" + msg.id,
		EventType:       model.UsageEventCompactionStart,
		Timestamp:       &ts,
		Sequence:        seq,
		MessageID:       msg.id,
		Source:          model.UsageSourceMessageMetadata,
		Completeness:    model.UsageComplete,
		SourceIdentity:  v2CompPrefix + msg.id,
	}
}

// v2AssistantEvents 生成 assistant 消息的工具事件与请求事件；
// 工具在前、请求在后，与传统 step-finish 收尾的事件顺序一致。
func v2AssistantEvents(msg v2MessageRow, s model.Session, seq int64) []*model.UsageTimelineEvent {
	var data v2AssistantData
	if err := json.Unmarshal(msg.raw, &data); err != nil {
		return nil // 单条 data 非法 JSON 时跳过该消息
	}
	var events []*model.UsageTimelineEvent
	cur := seq
	for i, block := range data.Content {
		if ev := v2ToolEvent(msg, s, cur, i, block); ev != nil {
			events = append(events, ev)
			cur++
		}
	}
	if ev := v2RequestEvent(msg, s, cur, data); ev != nil {
		events = append(events, ev)
	}
	return events
}

// v2ToolEvent 把 assistant content 中的 tool 块转为工具开始/结束事件；非 tool 块返回 nil。
func v2ToolEvent(
	msg v2MessageRow,
	s model.Session,
	seq int64,
	idx int,
	block v2ContentBlock,
) *model.UsageTimelineEvent {
	if block.Type != typeTool {
		return nil
	}
	evType := model.UsageEventToolStart
	if block.State.Status == statusCompleted {
		evType = model.UsageEventToolEnd
	}
	ts := time.UnixMilli(msg.created)
	if block.Time.Created > 0 {
		ts = time.UnixMilli(block.Time.Created)
	}
	return &model.UsageTimelineEvent{
		AgentInstanceID: s.AgentInstanceID,
		SessionID:       s.SessionID,
		EventID:         fmt.Sprintf("v2tool-%s-%d", msg.id, idx),
		EventType:       evType,
		Timestamp:       &ts,
		Sequence:        seq,
		MessageID:       msg.id,
		ToolCallID:      block.ID,
		ToolName:        block.Name,
		FilePath:        block.State.Input.FilePath,
		Source:          model.UsageSourceMessageMetadata,
		Completeness:    model.UsageComplete,
		SourceIdentity:  fmt.Sprintf("%s%s:%d", v2ToolPrefix, msg.id, idx),
	}
}

// v2RequestEvent 把 assistant.data.tokens 作为单次增量生成请求事件。
// v2 无上下文快照，TotalTokens/ContextAfter/CumulativeTotal 保持空（未知）而不是 0；
// tokens 缺失或全零视为未知，不生成事件。
func v2RequestEvent(
	msg v2MessageRow,
	s model.Session,
	seq int64,
	data v2AssistantData,
) *model.UsageTimelineEvent {
	if !hasV2RequestTokens(data.Tokens) {
		return nil
	}
	ts := time.UnixMilli(msg.created)
	var cacheRead, cacheWrite *int64
	if data.Tokens.Cache != nil {
		cacheRead = data.Tokens.Cache.Read
		cacheWrite = data.Tokens.Cache.Write
	}
	return &model.UsageTimelineEvent{
		AgentInstanceID:  s.AgentInstanceID,
		SessionID:        s.SessionID,
		EventID:          "v2req-" + msg.id,
		EventType:        model.UsageEventRequest,
		Timestamp:        &ts,
		Sequence:         seq,
		MessageID:        msg.id,
		Model:            data.Model.ID,
		InputTokens:      data.Tokens.Input,
		OutputTokens:     data.Tokens.Output,
		ReasoningTokens:  data.Tokens.Reasoning,
		CacheReadTokens:  cacheRead,
		CacheWriteTokens: cacheWrite,
		Source:           model.UsageSourceMessageMetadata,
		Completeness:     model.UsageComplete,
		SourceIdentity:   v2MsgPrefix + msg.id,
	}
}

// hasV2RequestTokens 判断 tokens 是否含正数增量；缺失或全零视为未知。
func hasV2RequestTokens(t *v2RequestTokens) bool {
	if t == nil {
		return false
	}
	if positiveInt64(t.Input) || positiveInt64(t.Output) || positiveInt64(t.Reasoning) {
		return true
	}
	return t.Cache != nil && (positiveInt64(t.Cache.Read) || positiveInt64(t.Cache.Write))
}

// positiveInt64 判断指针计数是否存在且为正。
func positiveInt64(v *int64) bool { return v != nil && *v > 0 }
