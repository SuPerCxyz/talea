// Package search 提供 FTS5 全文搜索。
package search

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
)

// Query 描述搜索条件。
type Query struct {
	Term      string
	Agent     string
	Cwd       string
	Project   string
	Branch    string
	SinceDays int
	Limit     int
}

// dirPrefix 将目录过滤参数规范化为绝对路径。
// 支持相对路径（./、../）与末尾斜杠：先解析为绝对路径，再清理，
// 保证与索引中记录的绝对工作目录精确匹配一致。
func dirPrefix(dir string) string {
	abs, err := filepath.Abs(dir)
	if err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(dir)
}

// Result 是单条搜索结果。
type Result struct {
	Session model.Session
	Score   float64
}

// Ensure 确保 FTS5 表与索引存在。
func Ensure(ctx context.Context, db *index.DB) error {
	stmts := []string{
		`CREATE VIRTUAL TABLE IF NOT EXISTS session_fts USING fts5(
			session_id,
			first_question,
			working_directory,
			project_name,
			git_branch,
			tokenize='trigram'
		)`,
	}
	for _, s := range stmts {
		if _, err := db.SQL().ExecContext(ctx, s); err != nil {
			return fmt.Errorf("创建 FTS 表: %w", err)
		}
	}
	return nil
}

// ftsRow 是待写入 FTS 表的一行。
type ftsRow struct {
	rid                 int
	sid, fq, wd, pn, gb string
}

// ftsInsert 在事务内批量写入 FTS 行。
func ftsInsert(ctx context.Context, tx *sql.Tx, batch []ftsRow) error {
	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO session_fts(rowid, session_id, first_question, working_directory, project_name, git_branch)
		 VALUES (?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()
	for _, r := range batch {
		if _, err := stmt.ExecContext(ctx, r.rid, r.sid, r.fq, r.wd, r.pn, r.gb); err != nil {
			return err
		}
	}
	return nil
}

// Populate 增量同步 FTS 表：只插入缺失行，不重建。
// 适用于 list/search 每次调用（O(新行) 而非 O(全表)）。
// 单事务批量写入，避免逐条 autocommit 的 fsync 开销。
func Populate(ctx context.Context, db *index.DB) error {
	rows, err := db.SQL().QueryContext(ctx,
		`SELECT s.rowid, s.session_id, s.first_question, s.working_directory, s.project_name, s.git_branch
		 FROM sessions s
		 WHERE NOT EXISTS(SELECT 1 FROM session_fts f WHERE f.rowid = s.rowid)`)
	if err != nil {
		return err
	}
	var batch []ftsRow
	for rows.Next() {
		var r ftsRow
		if err := rows.Scan(&r.rid, &r.sid, &r.fq, &r.wd, &r.pn, &r.gb); err != nil {
			continue
		}
		batch = append(batch, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()
	if len(batch) == 0 {
		return nil
	}
	tx, err := db.SQL().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := ftsInsert(ctx, tx, batch); err != nil {
		return err
	}
	return tx.Commit()
}

// Rebuild 全量重建 FTS 表（用于 talea index --rebuild）。
// 单事务批量写入。
func Rebuild(ctx context.Context, db *index.DB) error {
	if _, err := db.SQL().ExecContext(ctx, `DELETE FROM session_fts`); err != nil {
		return err
	}
	rows, err := db.SQL().QueryContext(ctx,
		`SELECT rowid, session_id, first_question, working_directory, project_name, git_branch
		 FROM sessions`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var batch []ftsRow
	for rows.Next() {
		var r ftsRow
		if err := rows.Scan(&r.rid, &r.sid, &r.fq, &r.wd, &r.pn, &r.gb); err != nil {
			continue
		}
		batch = append(batch, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(batch) == 0 {
		return nil
	}
	tx, err := db.SQL().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := ftsInsert(ctx, tx, batch); err != nil {
		return err
	}
	return tx.Commit()
}

// Search 执行全文搜索。
func Search(ctx context.Context, db *index.DB, q Query) ([]Result, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 200
	}

	var (
		where   []string
		args    []any
		hasTerm bool
	)
	if q.Term != "" {
		if utf8.RuneCountInString(q.Term) >= 3 {
			// 3 字以上走 FTS5，带列权重排序（bm25，权重按列序：
			// session_id=10, first_question=5, working_directory=3,
			// project_name=2, git_branch=1）
			where = append(where, `EXISTS(SELECT 1 FROM session_fts f
				WHERE f.rowid = s.rowid AND session_fts MATCH ?)`)
			args = append(args, ftsTerm(q.Term))
			hasTerm = true
		} else {
			where = append(where, `(s.session_id = ? OR s.first_question LIKE ? OR s.working_directory LIKE ?)`)
			args = append(args, q.Term, "%"+q.Term+"%", "%"+q.Term+"%")
		}
	}
	if q.Agent != "" {
		where = append(where, `s.agent_id = ?`)
		args = append(args, q.Agent)
	}
	if q.Cwd != "" {
		where = append(where, `s.working_directory = ?`)
		args = append(args, dirPrefix(q.Cwd))
	}
	if q.Project != "" {
		where = append(where, `s.project_name LIKE ?`)
		args = append(args, "%"+q.Project+"%")
	}
	if q.Branch != "" {
		where = append(where, `s.git_branch = ?`)
		args = append(args, q.Branch)
	}
	if q.SinceDays > 0 {
		where = append(where, `s.last_activity_at >= ?`)
		args = append(args, sinceEpoch(q.SinceDays))
	}
	if len(where) == 0 {
		where = append(where, `1=1`)
	}

	// 关键词搜索时按 FTS 相关度排序（列权重），否则按最近活动
	orderBy := "s.last_activity_at DESC"
	scoreExpr := "0.0"
	if hasTerm {
		orderBy = "score"
		scoreExpr = `(SELECT bm25(session_fts, 10.0, 5.0, 3.0, 2.0, 1.0)
		          FROM session_fts f WHERE f.rowid = s.rowid AND session_fts MATCH ?)`
	}

	query := fmt.Sprintf(`
		SELECT s.agent_id, s.agent_instance_id, s.session_id,
		       s.first_question, s.started_at, s.ended_at,
		       s.last_activity_at, s.duration_seconds,
		       s.working_directory, s.git_branch,
		       s.is_subagent, s.has_token_usage,
		       s.source_path, s.source_id, s.source_mtime,
		       s.source_size, s.source_offset,
		       s.format_name, s.format_version,
		       s.working_dir_exists, s.project_name, s.git_root, s.git_remote,
		       s.activity_state,
		       u.input_tokens, u.output_tokens, u.total_tokens, u.peak_context_tokens,
		       %s AS score
		FROM sessions s
		LEFT JOIN session_usage u
		       ON u.agent_instance_id = s.agent_instance_id AND u.session_id = s.session_id
		WHERE %s
		ORDER BY %s
		LIMIT ?`,
		scoreExpr, strings.Join(where, " AND "), orderBy)
	// scoreExpr 的 MATCH ? 在 SELECT 中最先出现，必须置于参数最前；
	// 否则后续 where 参数（如 agent 过滤）会与 EXISTS 的 MATCH ? 错位。
	queryArgs := make([]any, 0, len(args)+2)
	if hasTerm {
		queryArgs = append(queryArgs, ftsTerm(q.Term))
	}
	queryArgs = append(queryArgs, args...)
	queryArgs = append(queryArgs, limit)

	rows, err := db.SQL().QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanSessions(rows)
}

// ByIDPrefix 按 session_id 前缀精确查找会话（不经 FTS，前缀短于 3 字符也能匹配）。
// agent 非空时过滤 agent_id；limit 为返回上限。
func ByIDPrefix(ctx context.Context, db *index.DB, prefix, agent string, limit int) ([]Result, error) {
	if prefix == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	// 转义 LIKE 通配符，确保前缀匹配精确
	escapedPrefix := strings.NewReplacer("%", "\\%", "_", "\\_").Replace(prefix)
	where := `s.session_id LIKE ? ESCAPE '\'`
	args := []any{escapedPrefix + "%"}
	if agent != "" {
		where += ` AND s.agent_id = ?`
		args = append(args, agent)
	}
	query := fmt.Sprintf(`
		SELECT s.agent_id, s.agent_instance_id, s.session_id,
		       s.first_question, s.started_at, s.ended_at,
		       s.last_activity_at, s.duration_seconds,
		       s.working_directory, s.git_branch,
		       s.is_subagent, s.has_token_usage,
		       s.source_path, s.source_id, s.source_mtime,
		       s.source_size, s.source_offset,
		       s.format_name, s.format_version,
		       s.working_dir_exists, s.project_name, s.git_root, s.git_remote,
		       s.activity_state,
		       u.input_tokens, u.output_tokens, u.total_tokens, u.peak_context_tokens,
		       0.0 AS score
		FROM sessions s
		LEFT JOIN session_usage u
		       ON u.agent_instance_id = s.agent_instance_id AND u.session_id = s.session_id
		WHERE %s
		ORDER BY s.last_activity_at DESC
		LIMIT ?`, where)
	args = append(args, limit)
	rows, err := db.SQL().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSessions(rows)
}

// scanSessions 将查询行扫描为 Session 结果（含 Token 汇总）。
func scanSessions(rows *sql.Rows) ([]Result, error) {
	var out []Result
	for rows.Next() {
		var (
			s                    model.Session
			started, ended, last *int64
			duration             *int64
			wdExists             int
			activity             string
			inTok, outTok        sql.NullInt64
			totTok, peakTok      sql.NullInt64
			score                float64
		)
		if err := rows.Scan(&s.AgentID, &s.AgentInstanceID, &s.SessionID,
			&s.FirstQuestion, &started, &ended, &last, &duration,
			&s.WorkingDirectory, &s.GitBranch,
			&s.IsSubagent, &s.HasTokenUsage,
			&s.SourcePath, &s.SourceID, &s.SourceMtime,
			&s.SourceSize, &s.SourceOffset,
			&s.FormatName, &s.FormatVersion,
			&wdExists, &s.ProjectName, &s.GitRoot, &s.GitRemote,
			&activity,
			&inTok, &outTok, &totTok, &peakTok,
			&score); err != nil {
			continue
		}
		s.WorkingDirExists = wdExists != 0
		s.Activity = model.ActivityState(activity)
		if started != nil {
			t := fromEpoch(*started)
			s.StartedAt = &t
		}
		if ended != nil {
			t := fromEpoch(*ended)
			s.EndedAt = &t
		}
		if last != nil {
			t := fromEpoch(*last)
			s.LastActivityAt = &t
		}
		if duration != nil {
			d := time.Duration(*duration) * time.Second
			s.Duration = &d
		}
		if s.HasTokenUsage {
			u := &model.TokenUsage{Source: model.UsageSourceAgentDatabase}
			u.InputTokens = nn(inTok)
			u.OutputTokens = nn(outTok)
			u.TotalTokens = nn(totTok)
			u.PeakContextTokens = nn(peakTok)
			s.TokenUsage = u
		}
		out = append(out, Result{Session: s, Score: -score}) // bm25 为负分，越小越相关
	}
	return out, rows.Err()
}

// List 列出会话（与 Search 共用，无关键词时）。
func List(ctx context.Context, db *index.DB, q Query) ([]Result, error) {
	q.Term = ""
	return Search(ctx, db, q)
}

func ftsTerm(term string) string {
	// trigram 需要加引号避免语法错误
	return fmt.Sprintf(`"%s"`, strings.ReplaceAll(term, `"`, `""`))
}

func sinceEpoch(days int) int64 {
	return time.Now().AddDate(0, 0, -days).Unix()
}

func fromEpoch(sec int64) time.Time {
	return time.Unix(sec, 0)
}

func nn(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	x := v.Int64
	return &x
}

var _ = sql.ErrNoRows
