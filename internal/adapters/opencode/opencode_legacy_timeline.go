package opencode

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/model"
)

// 传统 message/part 时间线路径：仅服务在 message 表有数据的存量会话；
// 上下文快照来自 step-finish part，行为与 session_v2 上线前的实现保持一致。

// iterateLegacyUsageEvents 从 message/part 表提取时间线事件。
// user 消息生成 user_message 事件；step-finish 生成 request 事件（携带 tokens）。
// source_identity 使用 message_id / part_id 保证幂等去重。
func (a *Adapter) iterateLegacyUsageEvents(
	ctx context.Context,
	db *sql.DB,
	s model.Session,
) (adapters.UsageEventIterator, error) {
	var events []*model.UsageTimelineEvent
	seq := int64(0)

	// 1) user 消息事件
	rows, err := db.QueryContext(ctx,
		`SELECT id, time_created, json_extract(data, '$.role')
		 FROM message WHERE session_id=? ORDER BY time_created ASC, id ASC`, s.SessionID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var (
			msgID   string
			created int64
			role    string
		)
		if err := rows.Scan(&msgID, &created, &role); err != nil {
			continue
		}
		if role != "user" {
			continue
		}
		ts := time.UnixMilli(created)
		preview := a.messagePreview(ctx, db, msgID)
		events = append(events, &model.UsageTimelineEvent{
			AgentInstanceID:   s.AgentInstanceID,
			SessionID:         s.SessionID,
			EventID:           "msg-" + msgID,
			EventType:         model.UsageEventUserMessage,
			Timestamp:         &ts,
			Sequence:          seq,
			MessageID:         msgID,
			Source:            model.UsageSourceMessageMetadata,
			Completeness:      model.UsageComplete,
			UserPromptPreview: preview,
			SourceIdentity:    "opencode-msg:" + msgID,
		})
		seq++
	}
	rows.Close()

	// 2) step-finish request 事件
	rows2, err := db.QueryContext(ctx,
		`SELECT p.id, p.message_id, p.time_created, p.data
		 FROM part p
		 WHERE p.session_id = ?
		 ORDER BY p.time_created ASC, p.id ASC`, s.SessionID)
	if err != nil {
		return nil, err
	}
	defer rows2.Close()
	for rows2.Next() {
		var (
			partID, msgID string
			created       int64
			raw           string
		)
		if err := rows2.Scan(&partID, &msgID, &created, &raw); err != nil {
			continue
		}
		var p struct {
			Type   string          `json:"type"`
			Tokens json.RawMessage `json:"tokens"`
			Tool   string          `json:"tool"`
			CallID string          `json:"callID"`
			State  json.RawMessage `json:"state"`
			Auto   *bool           `json:"auto"`
		}
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			continue
		}
		ts := time.UnixMilli(created)

		// 明确压缩事件：compaction part（spec §14.6）
		if p.Type == "compaction" {
			ev := &model.UsageTimelineEvent{
				AgentInstanceID: s.AgentInstanceID,
				SessionID:       s.SessionID,
				EventID:         partID,
				EventType:       model.UsageEventCompactionStart,
				Timestamp:       &ts,
				Sequence:        seq,
				MessageID:       msgID,
				Source:          model.UsageSourceMessageMetadata,
				Completeness:    model.UsageComplete,
				SourceIdentity:  "opencode-compact:" + partID,
				RawFields:       map[string]any{"auto": p.Auto != nil && *p.Auto},
			}
			events = append(events, ev)
			seq++
			continue
		}

		// 工具调用事件：tool part 生成 tool_start/tool_end
		if p.Type == "tool" {
			evType := model.UsageEventToolStart
			filePath := ""
			if len(p.State) > 0 {
				var st struct {
					Status string `json:"status"`
					Input  struct {
						FilePath string `json:"filePath"`
					} `json:"input"`
				}
				if json.Unmarshal(p.State, &st) == nil {
					if st.Status == "completed" {
						evType = model.UsageEventToolEnd
					}
					filePath = st.Input.FilePath
				}
			}
			ev := &model.UsageTimelineEvent{
				AgentInstanceID: s.AgentInstanceID,
				SessionID:       s.SessionID,
				EventID:         partID,
				EventType:       evType,
				Timestamp:       &ts,
				Sequence:        seq,
				MessageID:       msgID,
				ToolCallID:      p.CallID,
				ToolName:        p.Tool,
				FilePath:        filePath,
				Source:          model.UsageSourceMessageMetadata,
				Completeness:    model.UsageComplete,
				SourceIdentity:  "opencode-tool:" + partID,
			}
			events = append(events, ev)
			seq++
			continue
		}

		if p.Type != "step-finish" {
			continue
		}
		var tok struct {
			Total     *int64 `json:"total"`
			Input     *int64 `json:"input"`
			Output    *int64 `json:"output"`
			Reasoning *int64 `json:"reasoning"`
		}
		if err := json.Unmarshal(p.Tokens, &tok); err != nil {
			continue
		}
		ts = time.UnixMilli(created)
		ev := &model.UsageTimelineEvent{
			AgentInstanceID: s.AgentInstanceID,
			SessionID:       s.SessionID,
			EventID:         partID,
			EventType:       model.UsageEventRequest,
			Timestamp:       &ts,
			Sequence:        seq,
			MessageID:       msgID,
			Model:           a.messageModel(ctx, db, msgID),
			InputTokens:     tok.Input,
			OutputTokens:    tok.Output,
			TotalTokens:     tok.Total,
			ReasoningTokens: tok.Reasoning,
			Source:          model.UsageSourceMessageMetadata,
			Completeness:    model.UsageComplete,
			SourceIdentity:  "opencode-part:" + partID,
		}
		// OpenCode step-finish total 为上下文快照（累计），
		// input 为本次请求增量。将 total 映射为 ContextAfter 与 CumulativeTotal。
		if tok.Total != nil {
			ev.ContextAfter = tok.Total
			ev.CumulativeTotal = tok.Total
		}
		events = append(events, ev)
		seq++
	}
	return &eventIterator{events: events}, nil
}

// messageModel 从消息 data 提取模型名。
func (a *Adapter) messageModel(ctx context.Context, db *sql.DB, msgID string) string {
	var raw string
	err := db.QueryRowContext(ctx,
		`SELECT json_extract(data, '$.modelID') FROM message WHERE id=?`, msgID).Scan(&raw)
	if err != nil || raw == "" {
		return ""
	}
	return raw
}

// messagePreview 提取消息文本摘要。
func (a *Adapter) messagePreview(ctx context.Context, db *sql.DB, msgID string) string {
	rows, err := db.QueryContext(ctx,
		`SELECT data FROM part WHERE message_id=? AND json_extract(data,'$.type')='text'
		 ORDER BY time_created ASC`, msgID)
	if err != nil {
		return ""
	}
	defer rows.Close()
	var texts []string
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			continue
		}
		var p struct {
			Text string `json:"text"`
		}
		if json.Unmarshal([]byte(raw), &p) == nil && p.Text != "" {
			texts = append(texts, p.Text)
		}
	}
	joined := strings.Join(texts, "\n")
	runes := []rune(joined)
	if len(runes) > 200 {
		return string(runes[:200])
	}
	return joined
}
