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

// 传统 message/part 读取路径：仅服务在 message 表有数据的存量会话，
// 行为与 session_v2 上线前的实现保持一致。

// legacyFirstQuestion 读取传统 message/part 中最早 user 消息正文。
func (a *Adapter) legacyFirstQuestion(ctx context.Context, db *sql.DB, sessionID string) (string, string, float64) {
	row := db.QueryRowContext(ctx,
		`SELECT m.id FROM message m
		 WHERE m.session_id = ? AND json_extract(m.data, '$.role') = 'user'
		 ORDER BY m.time_created ASC LIMIT 1`, sessionID)
	var msgID string
	if err := row.Scan(&msgID); err != nil {
		return "", "none", 0
	}
	rows, err := db.QueryContext(ctx,
		`SELECT data FROM part WHERE message_id = ? ORDER BY time_created ASC`, msgID)
	if err != nil {
		return "", "none", 0
	}
	defer rows.Close()
	var texts []string
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			continue
		}
		var p struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			continue
		}
		if p.Type == "text" && p.Text != "" {
			texts = append(texts, p.Text)
		}
	}
	first, ok := extract.FirstNonInjected(texts)
	if !ok {
		return "", "user_message_no_text", 0
	}
	return first, "user_message", 1.0
}

// loadLegacyMessages 从传统 message/part 读取消息预览。
func (a *Adapter) loadLegacyMessages(
	ctx context.Context,
	s model.Session,
	opts adapters.MessageLoadOptions,
) (adapters.MessageIterator, error) {
	db, err := openRO(s.SourcePath)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx,
		`SELECT id, time_created FROM message WHERE session_id = ? ORDER BY time_created ASC`, s.SessionID)
	if err != nil {
		db.Close()
		return nil, err
	}
	type msgInfo struct {
		ID   string
		Time int64
	}
	var msgs []msgInfo
	for rows.Next() {
		var m msgInfo
		if err := rows.Scan(&m.ID, &m.Time); err != nil {
			continue
		}
		msgs = append(msgs, m)
	}
	rows.Close()
	db.Close()

	if opts.Limit > 0 && len(msgs) > opts.Limit {
		msgs = msgs[len(msgs)-opts.Limit:]
	}

	// 重新打开数据库按需加载正文
	db2, err := openRO(s.SourcePath)
	if err != nil {
		return nil, err
	}
	var out []adapters.Message
	for _, m := range msgs {
		var role string
		db2.QueryRowContext(ctx, `SELECT json_extract(data,'$.role') FROM message WHERE id=?`, m.ID).Scan(&role)
		pRows, err := db2.QueryContext(ctx, `SELECT data FROM part WHERE message_id=? ORDER BY time_created ASC`, m.ID)
		if err != nil {
			continue
		}
		var parts []string
		for pRows.Next() {
			var raw string
			pRows.Scan(&raw)
			var p struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			if json.Unmarshal([]byte(raw), &p) == nil && p.Type == "text" {
				parts = append(parts, p.Text)
			}
		}
		pRows.Close()
		out = append(out, adapters.Message{
			Role:      role,
			Timestamp: m.Time / 1000,
			Content:   strings.Join(parts, "\n"),
		})
	}
	db2.Close()
	return &sliceIterator{msgs: out}, nil
}
