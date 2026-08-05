package index

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/talea/talea/internal/model"
)

// IncrementStats 汇总一次增量索引结果。
type IncrementStats struct {
	Added   int
	Updated int
	Skipped int
	Errors  int
	Errs    []error
}

// TrackedSource 描述已知会话来源状态。
type TrackedSource struct {
	AgentInstanceID string
	SessionID       string
	SourceMtime     int64
	SourceSize      int64
	SourceOffset    int64
}

// LoadTracked 读取数据库中已跟踪的会话来源。
func (db *DB) LoadTracked(ctx context.Context) (map[string]TrackedSource, error) {
	rows, err := db.sql.QueryContext(ctx,
		`SELECT agent_instance_id, session_id, source_mtime, source_size, source_offset
		 FROM sessions`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]TrackedSource)
	for rows.Next() {
		var t TrackedSource
		if err := rows.Scan(&t.AgentInstanceID, &t.SessionID, &t.SourceMtime, &t.SourceSize, &t.SourceOffset); err != nil {
			continue
		}
		key := t.AgentInstanceID + "\x00" + t.SessionID
		out[key] = t
	}
	return out, nil
}

// UpsertMany 在事务中批量写入会话。
func (db *DB) UpsertMany(ctx context.Context, sessions []*model.Session) (IncrementStats, error) {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return IncrementStats{}, err
	}
	defer tx.Rollback()

	var st IncrementStats
	// 复用事务句柄的直通方法
	txDB := &txIndex{tx: tx}
	for _, s := range sessions {
		updated, err := txDB.upsertSession(ctx, s)
		if err != nil {
			st.Errors++
			st.Errs = append(st.Errs, err)
			continue
		}
		if updated {
			st.Updated++
		} else {
			st.Added++
		}
	}
	if err := tx.Commit(); err != nil {
		return st, fmt.Errorf("索引事务提交失败: %w", err)
	}
	return st, nil
}

// txIndex 是事务内索引写入的薄封装。
type txIndex struct {
	tx *sql.Tx
}

func (t *txIndex) upsertSession(ctx context.Context, s *model.Session) (bool, error) {
	// 查询是否已存在以区分新增/更新
	var exists bool
	err := t.tx.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM sessions WHERE agent_instance_id=? AND session_id=?)`,
		s.AgentInstanceID, s.SessionID).Scan(&exists)
	if err != nil {
		return false, err
	}

	res, err := t.tx.ExecContext(ctx, `
		INSERT INTO sessions (
			agent_id, agent_instance_id, session_id, format_name, format_version,
			first_question, first_question_source, first_question_confidence,
			started_at, ended_at, last_activity_at, duration_seconds,
			start_time_source, end_time_source,
			working_directory, working_dir_source, working_dir_exists,
			project_name, git_root, git_branch, git_remote,
			message_count, user_message_count, tool_call_count,
			parent_session_id, is_subagent, activity_state,
			source_path, source_id, source_mtime, source_size, source_offset,
			has_token_usage, indexed_at, updated_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(agent_instance_id, session_id) DO UPDATE SET
			agent_id=excluded.agent_id,
			format_name=excluded.format_name,
			format_version=excluded.format_version,
			first_question=excluded.first_question,
			first_question_source=excluded.first_question_source,
			first_question_confidence=excluded.first_question_confidence,
			started_at=excluded.started_at,
			ended_at=excluded.ended_at,
			last_activity_at=excluded.last_activity_at,
			duration_seconds=excluded.duration_seconds,
			start_time_source=excluded.start_time_source,
			end_time_source=excluded.end_time_source,
			working_directory=excluded.working_directory,
			working_dir_source=excluded.working_dir_source,
			working_dir_exists=excluded.working_dir_exists,
			project_name=excluded.project_name,
			git_root=excluded.git_root,
			git_branch=excluded.git_branch,
			git_remote=excluded.git_remote,
			message_count=excluded.message_count,
			user_message_count=excluded.user_message_count,
			tool_call_count=excluded.tool_call_count,
			parent_session_id=excluded.parent_session_id,
			is_subagent=excluded.is_subagent,
			activity_state=excluded.activity_state,
			source_path=excluded.source_path,
			source_id=excluded.source_id,
			source_mtime=excluded.source_mtime,
			source_size=excluded.source_size,
			source_offset=excluded.source_offset,
			has_token_usage=excluded.has_token_usage,
			updated_at=excluded.updated_at`,
		s.AgentID, s.AgentInstanceID, s.SessionID, s.FormatName, s.FormatVersion,
		s.FirstQuestion, s.FirstQuestionSource, s.FirstQuestionConfidence,
		toEpoch(s.StartedAt), toEpoch(s.EndedAt), toEpoch(s.LastActivityAt), durSeconds(s.Duration),
		s.StartTimeSource, s.EndTimeSource,
		s.WorkingDirectory, s.WorkingDirSource, boolInt(s.WorkingDirExists),
		s.ProjectName, s.GitRoot, s.GitBranch, s.GitRemote,
		s.MessageCount, s.UserMessageCount, s.ToolCallCount,
		s.ParentSessionID, boolInt(s.IsSubagent), string(s.Activity),
		s.SourcePath, s.SourceID, s.SourceMtime, s.SourceSize, s.SourceOffset,
		boolInt(s.HasTokenUsage), time.Now().Unix(), time.Now().Unix())
	if err != nil {
		return false, err
	}
	_, err = res.RowsAffected()
	if s.TokenUsage != nil {
		if uerr := t.upsertUsage(ctx, s); uerr != nil {
			return false, uerr
		}
	}
	return exists, nil
}

func (t *txIndex) upsertUsage(ctx context.Context, s *model.Session) error {
	u := s.TokenUsage
	raw, _ := json.Marshal(u.RawFields)
	_, err := t.tx.ExecContext(ctx, `
		INSERT INTO session_usage (
			agent_instance_id, session_id, input_tokens, output_tokens, total_tokens,
			cache_read_tokens, cache_write_tokens, reasoning_tokens, tool_tokens,
			request_count, peak_context_tokens, max_input_tokens, max_output_tokens,
			max_total_tokens, self_tokens, direct_child_tokens, descendant_tokens,
			estimated_cost_micros, currency, pricing_model, pricing_snapshot_at,
			usage_source, completeness, is_estimated, raw_fields_json, updated_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(agent_instance_id, session_id) DO UPDATE SET
			input_tokens=excluded.input_tokens,
			output_tokens=excluded.output_tokens,
			total_tokens=excluded.total_tokens,
			cache_read_tokens=excluded.cache_read_tokens,
			cache_write_tokens=excluded.cache_write_tokens,
			reasoning_tokens=excluded.reasoning_tokens,
			tool_tokens=excluded.tool_tokens,
			request_count=excluded.request_count,
			peak_context_tokens=excluded.peak_context_tokens,
			max_input_tokens=excluded.max_input_tokens,
			max_output_tokens=excluded.max_output_tokens,
			max_total_tokens=excluded.max_total_tokens,
			self_tokens=excluded.self_tokens,
			direct_child_tokens=excluded.direct_child_tokens,
			descendant_tokens=excluded.descendant_tokens,
			estimated_cost_micros=excluded.estimated_cost_micros,
			currency=excluded.currency,
			pricing_model=excluded.pricing_model,
			pricing_snapshot_at=excluded.pricing_snapshot_at,
			usage_source=excluded.usage_source,
			completeness=excluded.completeness,
			is_estimated=excluded.is_estimated,
			raw_fields_json=excluded.raw_fields_json,
			updated_at=excluded.updated_at`,
		s.AgentInstanceID, s.SessionID,
		ptrOrNil(u.InputTokens), ptrOrNil(u.OutputTokens), ptrOrNil(u.TotalTokens),
		ptrOrNil(u.CacheReadTokens), ptrOrNil(u.CacheWriteTokens), ptrOrNil(u.ReasoningTokens), ptrOrNil(u.ToolTokens),
		ptrOrNil(u.RequestCount), ptrOrNil(u.PeakContextTokens), ptrOrNil(u.MaxInputTokens),
		ptrOrNil(u.MaxOutputTokens), ptrOrNil(u.MaxTotalTokens), ptrOrNil(u.SelfTokens),
		ptrOrNil(u.DirectChildTokens), ptrOrNil(u.DescendantTokens),
		ptrOrNil(u.EstimatedCostMicros), u.Currency, u.PricingModel, toEpoch(u.PricingSnapshotAt),
		string(u.Source), string(u.Completeness), boolInt(u.IsEstimated), raw, time.Now().Unix())
	if err != nil {
		return err
	}
	return nil
}

// Count 返回会话总数。
func (db *DB) Count(ctx context.Context) (int, error) {
	var n int
	err := db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions`).Scan(&n)
	return n, err
}
