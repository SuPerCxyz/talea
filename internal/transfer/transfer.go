// Package transfer 支持多设备离线导入导出。
//
// 导出为 JSON 文件（含会话、Token 汇总、标签、备注），可拷贝到另一台机器
// 导入。不包含 Agent 原始数据，仅 Talea 索引内容。
package transfer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
	"github.com/talea/talea/internal/tags"
	"github.com/talea/talea/internal/usage"
)

// ExportFile 是导出的文件结构。
type ExportFile struct {
	Version  int             `json:"version"`
	Exported string          `json:"exported"`
	Sessions []ExportSession `json:"sessions"`
}

// ExportSession 是单个会话的导出结构。
type ExportSession struct {
	Session  model.Session     `json:"session"`
	Usage    *model.TokenUsage `json:"usage,omitempty"`
	Tags     []string          `json:"tags,omitempty"`
	Note     string            `json:"note,omitempty"`
	Favorite bool              `json:"favorite"`
}

// Export 导出全部会话到文件。
func Export(ctx context.Context, db *index.DB, outPath string) error {
	rows, err := db.SQL().QueryContext(ctx, `
		SELECT agent_id, agent_instance_id, session_id,
		       first_question, first_question_source, first_question_confidence,
		       started_at, ended_at, last_activity_at, duration_seconds,
		       start_time_source, end_time_source,
		       working_directory, working_dir_source, working_dir_exists,
		       project_name, git_root, git_branch, git_remote,
		       message_count, user_message_count, tool_call_count,
		       parent_session_id, is_subagent, activity_state,
		       source_path, source_id, source_mtime, source_size, source_offset,
		       has_token_usage, format_name, format_version
		FROM sessions`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var out ExportFile
	out.Version = 1
	out.Exported = nowString()
	for rows.Next() {
		var s model.Session
		var started, ended, last, dur *int64
		var wdExists, subagent int
		var fqSource, eTS, sTS string
		var fqConf sql.NullFloat64
		var wdSource string
		if err := rows.Scan(
			&s.AgentID, &s.AgentInstanceID, &s.SessionID,
			&s.FirstQuestion, &fqSource, &fqConf,
			&started, &ended, &last, &dur,
			&sTS, &eTS,
			&s.WorkingDirectory, &wdSource, &wdExists,
			&s.ProjectName, &s.GitRoot, &s.GitBranch, &s.GitRemote,
			&s.MessageCount, &s.UserMessageCount, &s.ToolCallCount,
			&s.ParentSessionID, &subagent, &s.Activity,
			&s.SourcePath, &s.SourceID, &s.SourceMtime, &s.SourceSize, &s.SourceOffset,
			&s.HasTokenUsage, &s.FormatName, &s.FormatVersion,
		); err != nil {
			continue
		}
		s.IsSubagent = subagent != 0
		s.WorkingDirExists = wdExists != 0
		s.StartedAt = epochToTime(started)
		s.EndedAt = epochToTime(ended)
		s.LastActivityAt = epochToTime(last)
		if dur != nil {
			d := time.Duration(*dur) * time.Second
			s.Duration = &d
		}
		s.StartTimeSource = model.TimeSource(sTS)
		s.EndTimeSource = model.TimeSource(eTS)
		s.FirstQuestionSource = fqSource
		if fqConf.Valid {
			s.FirstQuestionConfidence = fqConf.Float64
		}
		s.WorkingDirSource = wdSource

		es := ExportSession{Session: s}
		if u, err := usage.Load(ctx, db, s.AgentInstanceID, s.SessionID); err == nil {
			es.Usage = u
		}
		if m, err := tags.Get(ctx, db, s.AgentInstanceID, s.SessionID); err == nil {
			es.Tags = m.Tags
			es.Note = m.Note
			es.Favorite = m.Favorite
		}
		out.Sessions = append(out.Sessions, es)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(outPath, data, 0o600)
}

// Import 从 JSON 文件导入会话。
// 返回导入条数。已存在的会话跳过。整个导入在一个事务中执行。
func Import(ctx context.Context, db *index.DB, inPath string) (int, error) {
	data, err := os.ReadFile(inPath)
	if err != nil {
		return 0, err
	}
	var in ExportFile
	if err := json.Unmarshal(data, &in); err != nil {
		return 0, fmt.Errorf("解析导出文件: %w", err)
	}
	if in.Version != 1 {
		return 0, fmt.Errorf("不支持的导出版本: %d", in.Version)
	}
	count := 0
	for _, es := range in.Sessions {
		s := es.Session
		s.IndexedAt = timeNow()
		s.UpdatedAt = timeNow()
		// 使用 INSERT OR IGNORE 语义：已存在的会话跳过
		inserted, err := db.InsertIfNew(ctx, &s)
		if err != nil || !inserted {
			continue
		}
		if es.Usage != nil {
			s.TokenUsage = es.Usage
			s.HasTokenUsage = true
			_ = db.UpsertUsage(ctx, &s)
		}
		if len(es.Tags) > 0 {
			_ = tags.SetTags(ctx, db, s.AgentInstanceID, s.SessionID, joinTags(es.Tags))
		}
		if es.Note != "" {
			_ = tags.SetNote(ctx, db, s.AgentInstanceID, s.SessionID, es.Note)
		}
		if es.Favorite {
			_ = tags.SetFavorite(ctx, db, s.AgentInstanceID, s.SessionID, true)
		}
		count++
	}
	return count, nil
}

func epochToTime(p *int64) *time.Time {
	if p == nil {
		return nil
	}
	t := time.Unix(*p, 0)
	return &t
}

func joinTags(tags []string) string {
	return strings.Join(tags, ",")
}

func nowString() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func timeNow() time.Time {
	return time.Now().UTC()
}
