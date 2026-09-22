package opencode

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/adapters/extract"
	"github.com/talea/talea/internal/model"
)

// v2 时间线事件的幂等来源前缀：与 v1 的 opencode-msg:/opencode-tool:/opencode-compact:
// 区分，避免同一会话在 schema 切换前后重复计数。
const (
	v2MsgPrefix  = "opencode-v2msg:"
	v2ToolPrefix = "opencode-v2tool:"
	v2CompPrefix = "opencode-v2comp:"
	// v2PreviewRunes 与传统 messagePreview 一致的预览截断长度（字符）。
	v2PreviewRunes = 200
)

// session_message 的 type 与内容块 type 取值（实测 OpenCode 2.0.12）。
const (
	typeUser        = "user"
	typeAssistant   = "assistant"
	typeCompaction  = "compaction"
	typeText        = "text"
	typeTool        = "tool"
	statusCompleted = "completed"
)

// useLegacyMessages 判断会话消息是否走传统 message/part 路径：会话在传统 message 表
// 有行则走传统路径，否则走 v2 session_message；message 表缺失或查询失败按无数据处理。
func useLegacyMessages(ctx context.Context, db *sql.DB, sessionID string) bool {
	var one int
	err := db.QueryRowContext(ctx,
		`SELECT 1 FROM message WHERE session_id = ? LIMIT 1`, sessionID).Scan(&one)
	return err == nil
}

// v2UserData 是 session_message 中 user 消息的 data（实测 OpenCode 2.0.12）。
type v2UserData struct {
	Text string `json:"text"`
}

// v2ContentBlock 是 assistant 消息 content[] 的内容块。
type v2ContentBlock struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	ID    string `json:"id"`
	Name  string `json:"name"`
	State struct {
		Status string `json:"status"`
		Input  struct {
			FilePath string `json:"filePath"`
		} `json:"input"`
	} `json:"state"`
	Time struct {
		Created   int64 `json:"created"`
		Completed int64 `json:"completed"`
	} `json:"time"`
}

// v2CacheTokens 是 assistant.data.tokens.cache。
type v2CacheTokens struct {
	Read  *int64 `json:"read"`
	Write *int64 `json:"write"`
}

// v2RequestTokens 是 assistant.data.tokens：单次请求增量，不是会话累计，
// 禁止与 session_v2.tokens_* 的会话累计值相加。
type v2RequestTokens struct {
	Input     *int64         `json:"input"`
	Output    *int64         `json:"output"`
	Reasoning *int64         `json:"reasoning"`
	Cache     *v2CacheTokens `json:"cache"`
}

// v2AssistantData 是 session_message 中 assistant 消息的 data。
type v2AssistantData struct {
	Model struct {
		ID string `json:"id"`
	} `json:"model"`
	Content []v2ContentBlock `json:"content"`
	Tokens  *v2RequestTokens `json:"tokens"`
}

// v2FirstQuestion 从 session_message 中按 seq 升序取第一条 user 消息的 data.text，
// 复用传统路径的注入内容过滤。
func (a *Adapter) v2FirstQuestion(
	ctx context.Context,
	db *sql.DB,
	sessionID string,
) (string, string, float64) {
	var raw string
	err := db.QueryRowContext(ctx,
		`SELECT data FROM session_message
		 WHERE session_id = ? AND type = ?
		 ORDER BY seq ASC LIMIT 1`, sessionID, typeUser).Scan(&raw)
	if err != nil {
		return "", "none", 0
	}
	var data v2UserData
	if json.Unmarshal([]byte(raw), &data) != nil {
		return "", "none", 0
	}
	if first, ok := extract.FirstNonInjected([]string{data.Text}); ok {
		return first, "user_message", 1.0
	}
	return "", "user_message_no_text", 0
}

// v2MessageText 提取单条 session_message 的展示文本：user 取 data.text，
// assistant 拼接 content[] 中 type=text 的文本。
func v2MessageText(msgType string, raw []byte) string {
	if msgType == typeUser {
		var data v2UserData
		if json.Unmarshal(raw, &data) != nil {
			return ""
		}
		return data.Text
	}
	var data v2AssistantData
	if json.Unmarshal(raw, &data) != nil {
		return ""
	}
	var texts []string
	for _, block := range data.Content {
		if block.Type == typeText && block.Text != "" {
			texts = append(texts, block.Text)
		}
	}
	return strings.Join(texts, "\n")
}

// v2PreviewText 提取消息预览并按 v2PreviewRunes 截断（与传统 messagePreview 一致）。
func v2PreviewText(msgType string, raw []byte) string {
	text := v2MessageText(msgType, raw)
	runes := []rune(text)
	if len(runes) > v2PreviewRunes {
		return string(runes[:v2PreviewRunes])
	}
	return text
}

// v2LoadMessages 从 session_message 读取消息预览（仅 user 与 assistant 两类消息）。
func (a *Adapter) v2LoadMessages(
	ctx context.Context,
	s model.Session,
	opts adapters.MessageLoadOptions,
) (adapters.MessageIterator, error) {
	db, err := openRO(s.SourcePath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx,
		`SELECT type, time_created, data FROM session_message
		 WHERE session_id = ? AND type IN (?, ?)
		 ORDER BY time_created ASC, seq ASC`, s.SessionID, typeUser, typeAssistant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []adapters.Message
	for rows.Next() {
		var msgType, raw string
		var created int64
		if err := rows.Scan(&msgType, &created, &raw); err != nil {
			continue
		}
		msgs = append(msgs, adapters.Message{
			Role:      msgType,
			Timestamp: created / 1000,
			Content:   v2MessageText(msgType, []byte(raw)),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if opts.Limit > 0 && len(msgs) > opts.Limit {
		msgs = msgs[len(msgs)-opts.Limit:]
	}
	return &sliceIterator{msgs: msgs}, nil
}
