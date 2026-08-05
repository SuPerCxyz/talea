// Package search 提供 FTS5 全文搜索。
package search

import (
	"context"
	"database/sql"
	"fmt"
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

// Populate 全量重建 FTS 表内容。
func Populate(ctx context.Context, db *index.DB) error {
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
	for rows.Next() {
		var (
			rid        int
			sid, fq    string
			wd, pn, gb string
		)
		if err := rows.Scan(&rid, &sid, &fq, &wd, &pn, &gb); err != nil {
			continue
		}
		if _, err := db.SQL().ExecContext(ctx,
			`INSERT INTO session_fts(rowid, session_id, first_question, working_directory, project_name, git_branch)
			 VALUES (?,?,?,?,?,?)`, rid, sid, fq, wd, pn, gb); err != nil {
			return err
		}
	}
	return nil
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
		where = append(where, `s.working_directory LIKE ?`)
		args = append(args, q.Cwd+"%")
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
		       %s AS score
		FROM sessions s
		WHERE %s
		ORDER BY %s
		LIMIT ?`,
		scoreExpr, strings.Join(where, " AND "), orderBy)
	if hasTerm {
		args = append(args, ftsTerm(q.Term))
	}
	args = append(args, limit)

	rows, err := db.SQL().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Result
	for rows.Next() {
		var (
			s                    model.Session
			started, ended, last *int64
			duration             *int64
			wdExists             int
			activity             string
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
			&activity, &score); err != nil {
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
		// 加载 Token 汇总
		if s.HasTokenUsage {
			var input, output, total, peak sql.NullInt64
			err := db.SQL().QueryRowContext(ctx,
				`SELECT input_tokens, output_tokens, total_tokens, peak_context_tokens
				 FROM session_usage WHERE agent_instance_id=? AND session_id=?`,
				s.AgentInstanceID, s.SessionID).Scan(&input, &output, &total, &peak)
			if err == nil {
				u := &model.TokenUsage{Source: model.UsageSourceAgentDatabase}
				u.InputTokens = nn(input)
				u.OutputTokens = nn(output)
				u.TotalTokens = nn(total)
				u.PeakContextTokens = nn(peak)
				s.TokenUsage = u
			}
		}
		out = append(out, Result{Session: s, Score: -score}) // bm25 为负分，越小越相关
	}
	return out, nil
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
