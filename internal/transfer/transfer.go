// Package transfer 支持多设备离线导入导出。
//
// 导出为 JSON 文件（含会话、Token 汇总、标签、备注），可拷贝到另一台机器
// 导入。不包含 Agent 原始数据，仅 Talea 索引内容。
package transfer

import (
	"context"
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
		SELECT agent_id, agent_instance_id, session_id, first_question,
		       started_at, ended_at, last_activity_at, working_directory,
		       git_branch, is_subagent
		FROM sessions`)
	if err != nil {
		return err
	}
	var out ExportFile
	out.Version = 1
	out.Exported = nowString()
	for rows.Next() {
		var s model.Session
		var started, ended, last *int64
		var subagent int
		if err := rows.Scan(&s.AgentID, &s.AgentInstanceID, &s.SessionID,
			&s.FirstQuestion, &started, &ended, &last,
			&s.WorkingDirectory, &s.GitBranch, &subagent); err != nil {
			continue
		}
		s.IsSubagent = subagent != 0
		s.StartedAt = epochToTime(started)
		s.EndedAt = epochToTime(ended)
		s.LastActivityAt = epochToTime(last)

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
	rows.Close()

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(outPath, data, 0o600)
}

// Import 从 JSON 文件导入会话。
// 返回导入条数。已存在的会话跳过。
func Import(ctx context.Context, db *index.DB, inPath string) (int, error) {
	data, err := os.ReadFile(inPath)
	if err != nil {
		return 0, err
	}
	var in ExportFile
	if err := json.Unmarshal(data, &in); err != nil {
		return 0, fmt.Errorf("解析导出文件: %w", err)
	}
	count := 0
	for _, es := range in.Sessions {
		s := es.Session
		s.IndexedAt = timeNow()
		s.UpdatedAt = timeNow()
		// 存在则跳过
		var exists int
		err := db.SQL().QueryRowContext(ctx,
			`SELECT COUNT(*) FROM sessions WHERE agent_instance_id=? AND session_id=?`,
			s.AgentInstanceID, s.SessionID).Scan(&exists)
		if err != nil || exists > 0 {
			continue
		}
		if err := db.UpsertSession(ctx, &s); err != nil {
			continue
		}
		if es.Usage != nil {
			s.TokenUsage = es.Usage
			s.HasTokenUsage = true
			if err := db.UpsertUsage(ctx, &s); err != nil {
				continue
			}
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
	return time.Now().Format(time.RFC3339)
}

func timeNow() time.Time {
	return time.Now()
}
