package opencode

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/talea/talea/internal/model"
)

// latestTurnMessage 是会话中最后一条与回合状态相关的消息。
type latestTurnMessage struct {
	Type    string
	AtMS    int64
	Outcome string
}

// turnState 根据最新 session_message / 传统 message 与 idle_outcome 映射通用回合状态。
func (a *Adapter) turnState(
	ctx context.Context,
	db *sql.DB,
	row sessionRow,
	legacy bool,
) (model.TurnState, *time.Time, model.TurnStateSource, string) {
	latest := latestTurnMessageOf(ctx, db, row.ID, legacy)

	// 最新消息本身是 idle 状态时，按 outcome 映射。
	if latest != nil && latest.Type == typeIdle {
		outcome := latest.Outcome
		if outcome == "" {
			outcome = row.IdleOutcome.String
		}
		return mapIdleOutcome(outcome, latest.AtMS)
	}
	// session_v2 的 time_idle 不早于最后一条消息时，使用原生 idle outcome。
	if row.TimeIdle.Valid && row.TimeIdle.Int64 > 0 &&
		(latest == nil || row.TimeIdle.Int64 >= latest.AtMS) {
		return mapIdleOutcome(row.IdleOutcome.String, row.TimeIdle.Int64)
	}
	if latest == nil {
		return model.TurnStateUnknown, nil, model.TurnStateSourceUnknown, ""
	}
	if latest.Type == typeUser || latest.Type == typeAssistant {
		return model.TurnStateWorking, unixMilliPtr(latest.AtMS),
			model.TurnStateSourceInferred, latest.Type + " message"
	}
	return model.TurnStateUnknown, nil, model.TurnStateSourceUnknown, ""
}

// latestTurnMessageOf 读取最后一条 user/assistant/idle 消息。
// 传统消息路径没有 idle 行，按最后一条 message 的角色兜底。
func latestTurnMessageOf(
	ctx context.Context,
	db *sql.DB,
	sessionID string,
	legacy bool,
) *latestTurnMessage {
	if legacy {
		return latestLegacyMessage(ctx, db, sessionID)
	}
	return latestV2Message(ctx, db, sessionID)
}

// latestLegacyMessage 读取传统 message 表最后一条消息的角色。
func latestLegacyMessage(ctx context.Context, db *sql.DB, sessionID string) *latestTurnMessage {
	var data string
	var created sql.NullInt64
	err := db.QueryRowContext(ctx,
		`SELECT data, time_created FROM message
		 WHERE session_id = ? ORDER BY time_created DESC LIMIT 1`, sessionID).
		Scan(&data, &created)
	if err != nil {
		return nil
	}
	var m struct {
		Role string `json:"role"`
		Time struct {
			Created int64 `json:"created"`
		} `json:"time"`
	}
	_ = json.Unmarshal([]byte(data), &m)
	at := m.Time.Created
	if at == 0 && created.Valid {
		at = created.Int64
	}
	return &latestTurnMessage{Type: m.Role, AtMS: at}
}

// latestV2Message 读取 session_message 最后一条相关消息（含 idle 状态行）。
func latestV2Message(ctx context.Context, db *sql.DB, sessionID string) *latestTurnMessage {
	var msgType, data string
	var created int64
	err := db.QueryRowContext(ctx,
		`SELECT type, time_created, data FROM session_message
		 WHERE session_id = ? AND type IN ('user','assistant','idle')
		 ORDER BY seq DESC, time_created DESC LIMIT 1`, sessionID).
		Scan(&msgType, &created, &data)
	if err != nil {
		return nil
	}
	return &latestTurnMessage{Type: msgType, AtMS: created, Outcome: outcomeOf(data)}
}

// outcomeOf 从消息 data 中提取 idle outcome（兼容常见字段名）。
func outcomeOf(data string) string {
	var d struct {
		Outcome string `json:"outcome"`
		Status  string `json:"status"`
		State   string `json:"state"`
	}
	if err := json.Unmarshal([]byte(data), &d); err != nil {
		return ""
	}
	for _, v := range []string{d.Outcome, d.Status, d.State} {
		if v != "" {
			return v
		}
	}
	return ""
}

// mapIdleOutcome 把 idle outcome 映射为通用状态；未知 outcome 仍视为空闲。
func mapIdleOutcome(outcome string, atMS int64) (model.TurnState, *time.Time, model.TurnStateSource, string) {
	state := model.TurnStateIdle
	switch strings.ToLower(strings.TrimSpace(outcome)) {
	case "succeeded", "success", "completed", "complete", "ok", "done":
		state = model.TurnStateCompleted
	case "failed", "failure", "error", "errored":
		state = model.TurnStateFailed
	case "interrupted", "cancelled", "canceled", "aborted", "stopped":
		state = model.TurnStateInterrupted
	}
	detail := "idle outcome"
	if outcome != "" {
		detail += ": " + outcome
	}
	return state, unixMilliPtr(atMS), model.TurnStateSourceNative, detail
}

func unixMilliPtr(ms int64) *time.Time {
	if ms <= 0 {
		return nil
	}
	t := time.UnixMilli(ms)
	return &t
}
