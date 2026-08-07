// Package index 管理 Talea 本地 SQLite 索引：schema、迁移、增量写入。
package index

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/talea/talea/internal/model"
)

// SchemaVersion 是当前 schema 版本。
const SchemaVersion = 1

// DB 封装 SQLite 索引。
type DB struct {
	sql *sql.DB
	dir string
}

// Open 打开（或创建）索引数据库，权限 0600。
func Open(path string) (*DB, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("创建数据目录 %s: %w", dir, err)
	}
	// 预创建文件以确保权限，再以 WAL 打开
	if _, err := os.Stat(path); os.IsNotExist(err) {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, fmt.Errorf("创建索引 %s: %w", path, err)
		}
		f.Close()
	}
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)", path)
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("打开索引 %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil && runtime.GOOS != "windows" {
		sqlDB.Close()
		return nil, fmt.Errorf("设置索引权限 0600: %w", err)
	}
	return &DB{sql: sqlDB, dir: dir}, nil
}

// SQL 返回底层数据库（仅供 search/usage 只读使用）。
func (db *DB) SQL() *sql.DB { return db.sql }

// Close 关闭数据库。
func (db *DB) Close() error { return db.sql.Close() }

// Migrate 执行幂等迁移。迁移前若检测到 schema 版本变化，先备份旧数据库。
func (db *DB) Migrate(ctx context.Context) error {
	if err := db.backupIfVersionChange(ctx); err != nil {
		return err
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS schema_meta (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			agent_id TEXT NOT NULL,
			agent_instance_id TEXT NOT NULL,
			session_id TEXT NOT NULL,
			format_name TEXT,
			format_version TEXT,
			first_question TEXT,
			first_question_source TEXT,
			first_question_confidence REAL,
			started_at INTEGER,
			ended_at INTEGER,
			last_activity_at INTEGER,
			duration_seconds INTEGER,
			start_time_source TEXT,
			end_time_source TEXT,
			working_directory TEXT,
			working_dir_source TEXT,
			working_dir_exists INTEGER NOT NULL DEFAULT 0,
			project_name TEXT,
			git_root TEXT,
			git_branch TEXT,
			git_remote TEXT,
			message_count INTEGER NOT NULL DEFAULT 0,
			user_message_count INTEGER NOT NULL DEFAULT 0,
			tool_call_count INTEGER NOT NULL DEFAULT 0,
			parent_session_id TEXT,
			is_subagent INTEGER NOT NULL DEFAULT 0,
			activity_state TEXT NOT NULL DEFAULT 'unknown',
			source_path TEXT,
			source_id TEXT,
			source_mtime INTEGER,
			source_size INTEGER,
			source_offset INTEGER NOT NULL DEFAULT 0,
			has_token_usage INTEGER NOT NULL DEFAULT 0,
			indexed_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			PRIMARY KEY (agent_instance_id, session_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_activity
			ON sessions (last_activity_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_working_directory
			ON sessions (working_directory)`,
		`CREATE TABLE IF NOT EXISTS session_usage (
			agent_instance_id TEXT NOT NULL,
			session_id TEXT NOT NULL,
			input_tokens INTEGER,
			output_tokens INTEGER,
			total_tokens INTEGER,
			cache_read_tokens INTEGER,
			cache_write_tokens INTEGER,
			reasoning_tokens INTEGER,
			tool_tokens INTEGER,
			request_count INTEGER,
			peak_context_tokens INTEGER,
			max_input_tokens INTEGER,
			max_output_tokens INTEGER,
			max_total_tokens INTEGER,
			self_tokens INTEGER,
			direct_child_tokens INTEGER,
			descendant_tokens INTEGER,
			estimated_cost_micros INTEGER,
			currency TEXT,
			pricing_model TEXT,
			pricing_snapshot_at INTEGER,
			usage_source TEXT NOT NULL DEFAULT 'unknown',
			completeness TEXT NOT NULL DEFAULT 'unknown',
			is_estimated INTEGER NOT NULL DEFAULT 0,
			raw_fields_json TEXT,
			updated_at INTEGER NOT NULL,
			PRIMARY KEY (agent_instance_id, session_id)
		)`,
		`CREATE TABLE IF NOT EXISTS usage_timeline_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			agent_instance_id TEXT NOT NULL,
			session_id TEXT NOT NULL,
			event_id TEXT,
			event_type TEXT NOT NULL,
			timestamp INTEGER,
			sequence INTEGER NOT NULL,
			duration_ms INTEGER,
			request_id TEXT,
			response_id TEXT,
			message_id TEXT,
			parent_message_id TEXT,
			tool_call_id TEXT,
			subagent_id TEXT,
			model TEXT,
			provider TEXT,
			input_tokens INTEGER,
			output_tokens INTEGER,
			total_tokens INTEGER,
			cache_read_tokens INTEGER,
			cache_write_tokens INTEGER,
			reasoning_tokens INTEGER,
			tool_tokens INTEGER,
			context_before INTEGER,
			context_after INTEGER,
			context_limit INTEGER,
			cumulative_input INTEGER,
			cumulative_output INTEGER,
			cumulative_total INTEGER,
			user_prompt_preview TEXT,
			tool_name TEXT,
			file_path TEXT,
			command_preview TEXT,
			value_mode TEXT,
			usage_source TEXT NOT NULL,
			completeness TEXT NOT NULL,
			is_estimated INTEGER NOT NULL DEFAULT 0,
			source_identity TEXT NOT NULL,
			raw_fields_json TEXT,
			UNIQUE (agent_instance_id, session_id, source_identity)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_timeline_session_time
			ON usage_timeline_events (agent_instance_id, session_id, timestamp, sequence)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_agent
			ON sessions (agent_instance_id, last_activity_at DESC)`,
		`CREATE TABLE IF NOT EXISTS session_meta (
			agent_instance_id TEXT NOT NULL,
			session_id TEXT NOT NULL,
			favorite INTEGER NOT NULL DEFAULT 0,
			note TEXT,
			PRIMARY KEY (agent_instance_id, session_id)
		)`,
		`CREATE TABLE IF NOT EXISTS session_tags (
			agent_instance_id TEXT NOT NULL,
			session_id TEXT NOT NULL,
			tag TEXT NOT NULL,
			PRIMARY KEY (agent_instance_id, session_id, tag)
		)`,
	}
	for _, s := range stmts {
		if _, err := db.sql.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("迁移执行失败: %w", err)
		}
	}
	_, err := db.sql.ExecContext(ctx,
		`INSERT INTO schema_meta (key, value) VALUES ('schema_version', ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, fmt.Sprint(SchemaVersion))
	if err != nil {
		return err
	}
	return nil
}

// backupIfVersionChange 检测到 schema 版本差异时备份旧数据库。
// 版本相同或数据库不存在时跳过。使用 VACUUM INTO 生成一致快照，权限 0600。
func (db *DB) backupIfVersionChange(ctx context.Context) error {
	// schema_meta 可能还不存在（首次迁移），此时无需备份
	if _, err := db.sql.ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS schema_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		return err
	}
	var curVersion string
	err := db.sql.QueryRowContext(ctx,
		`SELECT value FROM schema_meta WHERE key='schema_version'`).Scan(&curVersion)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil // 新库，无备份必要
		}
		return err
	}
	if curVersion == fmt.Sprint(SchemaVersion) {
		return nil // 版本一致
	}
	// 版本变化：用 VACUUM INTO 生成一致快照备份
	bakPath := filepath.Join(db.dir,
		fmt.Sprintf("index.db.v%s.bak-%d", curVersion, time.Now().Unix()))
	// VACUUM INTO 不支持绑定参数，使用单引号引用路径并转义内部单引号
	quotedPath := strings.ReplaceAll(bakPath, "'", "''")
	if _, err := db.sql.ExecContext(ctx, fmt.Sprintf(`VACUUM INTO '%s'`, quotedPath)); err != nil {
		return fmt.Errorf("备份旧数据库失败: %w", err)
	}
	if err := os.Chmod(bakPath, 0o600); err != nil && runtime.GOOS != "windows" {
		return fmt.Errorf("设置备份权限 0600: %w", err)
	}
	return nil
}

// UpsertSession 写入或更新会话。调用方负责在事务内使用（或直接调用）。
func (db *DB) UpsertSession(ctx context.Context, s *model.Session) error {
	_, err := db.sql.ExecContext(ctx, `
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
		boolInt(s.HasTokenUsage), s.IndexedAt.Unix(), s.UpdatedAt.Unix())
	if err != nil {
		return fmt.Errorf("写入会话 %s/%s: %w", s.AgentInstanceID, s.SessionID, err)
	}
	if s.TokenUsage != nil {
		return db.UpsertUsage(ctx, s)
	}
	return nil
}

// UpsertUsage 写入 Token 汇总。
func (db *DB) UpsertUsage(ctx context.Context, s *model.Session) error {
	u := s.TokenUsage
	raw, _ := json.Marshal(u.RawFields)
	_, err := db.sql.ExecContext(ctx, `
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
	return err
}

// UpsertTimelineEvent 写入时间线事件，source_identity 冲突时跳过。
func (db *DB) UpsertTimelineEvent(ctx context.Context, e *model.UsageTimelineEvent) (bool, error) {
	raw, _ := json.Marshal(e.RawFields)
	res, err := db.sql.ExecContext(ctx, `
		INSERT OR IGNORE INTO usage_timeline_events (
			agent_instance_id, session_id, event_id, event_type, timestamp, sequence,
			duration_ms, request_id, response_id, message_id, parent_message_id,
			tool_call_id, subagent_id, model, provider, input_tokens, output_tokens,
			total_tokens, cache_read_tokens, cache_write_tokens, reasoning_tokens,
			tool_tokens, context_before, context_after, context_limit,
			cumulative_input, cumulative_output, cumulative_total,
			user_prompt_preview, tool_name, file_path, command_preview,
			value_mode, usage_source, completeness, is_estimated,
			source_identity, raw_fields_json
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		e.AgentInstanceID, e.SessionID, e.EventID, string(e.EventType), toEpoch(e.Timestamp), e.Sequence,
		durMillis(e.Duration), e.RequestID, e.ResponseID, e.MessageID, e.ParentMessageID,
		e.ToolCallID, e.SubagentID, e.Model, e.Provider,
		ptrOrNil(e.InputTokens), ptrOrNil(e.OutputTokens), ptrOrNil(e.TotalTokens),
		ptrOrNil(e.CacheReadTokens), ptrOrNil(e.CacheWriteTokens), ptrOrNil(e.ReasoningTokens),
		ptrOrNil(e.ToolTokens), ptrOrNil(e.ContextBefore), ptrOrNil(e.ContextAfter), ptrOrNil(e.ContextLimit),
		ptrOrNil(e.CumulativeInput), ptrOrNil(e.CumulativeOutput), ptrOrNil(e.CumulativeTotal),
		e.UserPromptPreview, e.ToolName, e.FilePath, e.CommandPreview,
		string(e.ValueMode), string(e.Source), string(e.Completeness), boolInt(e.IsEstimated),
		e.SourceIdentity, raw)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// UpsertTimelineEvents 在单事务内批量写入时间线事件，source_identity 冲突时跳过。
// 返回成功插入数。单个事件失败立即中止。
func (db *DB) UpsertTimelineEvents(ctx context.Context, events []*model.UsageTimelineEvent) (int, error) {
	if len(events) == 0 {
		return 0, nil
	}
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR IGNORE INTO usage_timeline_events (
			agent_instance_id, session_id, event_id, event_type, timestamp, sequence,
			duration_ms, request_id, response_id, message_id, parent_message_id,
			tool_call_id, subagent_id, model, provider, input_tokens, output_tokens,
			total_tokens, cache_read_tokens, cache_write_tokens, reasoning_tokens,
			tool_tokens, context_before, context_after, context_limit,
			cumulative_input, cumulative_output, cumulative_total,
			user_prompt_preview, tool_name, file_path, command_preview,
			value_mode, usage_source, completeness, is_estimated,
			source_identity, raw_fields_json
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return 0, err
	}
	defer func() { _ = stmt.Close() }()
	var n int
	for _, e := range events {
		raw, _ := json.Marshal(e.RawFields)
		res, err := stmt.ExecContext(ctx,
			e.AgentInstanceID, e.SessionID, e.EventID, string(e.EventType), toEpoch(e.Timestamp), e.Sequence,
			durMillis(e.Duration), e.RequestID, e.ResponseID, e.MessageID, e.ParentMessageID,
			e.ToolCallID, e.SubagentID, e.Model, e.Provider,
			ptrOrNil(e.InputTokens), ptrOrNil(e.OutputTokens), ptrOrNil(e.TotalTokens),
			ptrOrNil(e.CacheReadTokens), ptrOrNil(e.CacheWriteTokens), ptrOrNil(e.ReasoningTokens),
			ptrOrNil(e.ToolTokens), ptrOrNil(e.ContextBefore), ptrOrNil(e.ContextAfter), ptrOrNil(e.ContextLimit),
			ptrOrNil(e.CumulativeInput), ptrOrNil(e.CumulativeOutput), ptrOrNil(e.CumulativeTotal),
			e.UserPromptPreview, e.ToolName, e.FilePath, e.CommandPreview,
			string(e.ValueMode), string(e.Source), string(e.Completeness), boolInt(e.IsEstimated),
			e.SourceIdentity, raw)
		if err != nil {
			return n, err
		}
		if cnt, _ := res.RowsAffected(); cnt > 0 {
			n++
		}
	}
	return n, tx.Commit()
}

func ptrOrNil(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}

// AggregateChildTokens 将子会话总 Token 聚合到父会话的
// direct_child_tokens / descendant_tokens。返回是否更新。
// 使用直接赋值（非累加），保证幂等：重复调用不会重复增加。
func (db *DB) AggregateChildTokens(ctx context.Context, rel model.SessionRelation) error {
	var childTotal sql.NullInt64
	err := db.sql.QueryRowContext(ctx,
		`SELECT total_tokens FROM session_usage WHERE agent_instance_id=? AND session_id=?`,
		rel.ChildAgentInstanceID, rel.ChildSessionID).Scan(&childTotal)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil // 子会话无 usage，跳过
		}
		return err
	}
	if !childTotal.Valid {
		return nil
	}
	_, err = db.sql.ExecContext(ctx, `
		INSERT INTO session_usage (
			agent_instance_id, session_id, direct_child_tokens, descendant_tokens, updated_at
		) VALUES (?,?,?,?,?)
		ON CONFLICT(agent_instance_id, session_id) DO UPDATE SET
			direct_child_tokens = excluded.direct_child_tokens,
			descendant_tokens = excluded.descendant_tokens,
			updated_at = excluded.updated_at`,
		rel.ParentAgentInstanceID, rel.ParentSessionID,
		childTotal.Int64, childTotal.Int64, time.Now().Unix())
	return err
}

// ClearTimelineEvents 清除会话的时间线事件（用于全量重建）。
func (db *DB) ClearTimelineEvents(ctx context.Context, instanceID, sessionID string) error {
	_, err := db.sql.ExecContext(ctx,
		`DELETE FROM usage_timeline_events WHERE agent_instance_id=? AND session_id=?`,
		instanceID, sessionID)
	return err
}

// SetActivity 更新会话活动状态。
func (db *DB) SetActivity(ctx context.Context, instanceID, sessionID string, state model.ActivityState) (bool, error) {
	res, err := db.sql.ExecContext(ctx,
		`UPDATE sessions SET activity_state = ? WHERE agent_instance_id = ? AND session_id = ?`,
		string(state), instanceID, sessionID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// SetAllActivityByAgent 批量更新某 Agent 全部会话的活动状态（单条 SQL）。
func (db *DB) SetAllActivityByAgent(ctx context.Context, agentID model.AgentID, state model.ActivityState) (int64, error) {
	res, err := db.sql.ExecContext(ctx,
		`UPDATE sessions SET activity_state = ? WHERE agent_id = ?`,
		string(state), string(agentID))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// SetAllActivity 批量更新全部会话的活动状态（单条 SQL）。
func (db *DB) SetAllActivity(ctx context.Context, state model.ActivityState) (int64, error) {
	res, err := db.sql.ExecContext(ctx,
		`UPDATE sessions SET activity_state = ?`, string(state))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// SetRecentActive 将最近 n 秒内更新的会话标记为可能进行中（单条 SQL）。
func (db *DB) SetRecentActive(ctx context.Context, agentID model.AgentID, nSec int64) (int64, error) {
	res, err := db.sql.ExecContext(ctx,
		`UPDATE sessions SET activity_state = 'possibly_active'
		 WHERE agent_id = ? AND source_mtime >= ?`,
		string(agentID), time.Now().Add(-time.Duration(nSec)*time.Second).Unix())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func toEpoch(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.Unix()
}

func durSeconds(d *time.Duration) any {
	if d == nil {
		return nil
	}
	return int64(d.Seconds())
}

func durMillis(d *time.Duration) any {
	if d == nil {
		return nil
	}
	return d.Milliseconds()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
