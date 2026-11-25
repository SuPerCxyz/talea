package index

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/talea/talea/internal/model"
)

// execContexter 是 *sql.DB 与 *sql.Tx 的共同执行接口。
type execContexter interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// writeTurnState 将会话的通用回合状态写入独立表。
// 缺少状态时写入 unknown，保证旧会话和无法判断的会话也有稳定默认值。
func writeTurnState(ctx context.Context, ex execContexter, s *model.Session, updatedAt int64) error {
	state := s.TurnState
	if !state.Valid() {
		state = model.TurnStateUnknown
	}
	source := s.TurnStateSource
	if !source.Valid() {
		source = model.TurnStateSourceUnknown
	}
	_, err := ex.ExecContext(ctx, `
		INSERT INTO session_turn_state (
			agent_instance_id, session_id, turn_state, state_at,
			state_source, state_detail, updated_at
		) VALUES (?,?,?,?,?,?,?)
		ON CONFLICT(agent_instance_id, session_id) DO UPDATE SET
			turn_state=excluded.turn_state,
			state_at=excluded.state_at,
			state_source=excluded.state_source,
			state_detail=excluded.state_detail,
			updated_at=excluded.updated_at`,
		s.AgentInstanceID, s.SessionID, string(state), toEpoch(s.TurnStateAt),
		string(source), s.TurnStateDetail, updatedAt)
	if err != nil {
		return fmt.Errorf("写入回合状态 %s/%s: %w", s.AgentInstanceID, s.SessionID, err)
	}
	return nil
}

// LoadTurnStates 批量加载全部回合状态，键为 model.TurnStateKey。
// 未写入状态的旧会话不会出现在结果中，调用方按 unknown 处理。
func (db *DB) LoadTurnStates(ctx context.Context) (map[string]model.TurnStateRecord, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT agent_instance_id, session_id, turn_state, state_at,
		       state_source, state_detail
		FROM session_turn_state`)
	if err != nil {
		return nil, fmt.Errorf("读取回合状态失败: %w", err)
	}
	defer rows.Close()

	out := make(map[string]model.TurnStateRecord)
	for rows.Next() {
		var (
			rec     model.TurnStateRecord
			state   string
			source  string
			stateAt sql.NullInt64
			detail  sql.NullString
		)
		if err := rows.Scan(&rec.AgentInstanceID, &rec.SessionID, &state,
			&stateAt, &source, &detail); err != nil {
			continue
		}
		rec.State = model.TurnState(state)
		validState := rec.State.Valid()
		if !validState {
			rec.State = model.TurnStateUnknown
		}
		rec.Source = model.TurnStateSource(source)
		if !validState || !rec.Source.Valid() {
			rec.Source = model.TurnStateSourceUnknown
		}
		if stateAt.Valid {
			t := time.Unix(stateAt.Int64, 0)
			rec.StateAt = &t
		}
		rec.Detail = detail.String
		out[model.TurnStateKey(rec.AgentInstanceID, rec.SessionID)] = rec
	}
	return out, rows.Err()
}
