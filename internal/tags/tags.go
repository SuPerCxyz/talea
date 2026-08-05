// Package tags 管理会话标签、收藏与备注。
package tags

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/talea/talea/internal/index"
)

// Meta 是会话的用户附加元数据。
type Meta struct {
	Favorite bool
	Note     string
	Tags     []string
}

// Get 读取会话元数据。
func Get(ctx context.Context, db *index.DB, instanceID, sessionID string) (Meta, error) {
	var m Meta
	row := db.SQL().QueryRowContext(ctx,
		`SELECT favorite, COALESCE(note,'') FROM session_meta
		 WHERE agent_instance_id=? AND session_id=?`, instanceID, sessionID)
	var fav int
	if err := row.Scan(&fav, &m.Note); err == nil {
		m.Favorite = fav != 0
	} else if err != sql.ErrNoRows {
		return m, err
	}

	rows, err := db.SQL().QueryContext(ctx,
		`SELECT tag FROM session_tags WHERE agent_instance_id=? AND session_id=? ORDER BY tag`,
		instanceID, sessionID)
	if err != nil {
		return m, err
	}
	defer rows.Close()
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			continue
		}
		m.Tags = append(m.Tags, tag)
	}
	return m, nil
}

// SetTags 替换会话标签（逗号分隔）。
func SetTags(ctx context.Context, db *index.DB, instanceID, sessionID, tagsCSV string) error {
	tx, err := db.SQL().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM session_tags WHERE agent_instance_id=? AND session_id=?`,
		instanceID, sessionID); err != nil {
		return err
	}
	for _, t := range splitTags(tagsCSV) {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO session_tags (agent_instance_id, session_id, tag) VALUES (?,?,?)`,
			instanceID, sessionID, t); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// AddTag 追加单个标签。
func AddTag(ctx context.Context, db *index.DB, instanceID, sessionID, tag string) error {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return fmt.Errorf("标签不能为空")
	}
	_, err := db.SQL().ExecContext(ctx,
		`INSERT OR IGNORE INTO session_tags (agent_instance_id, session_id, tag) VALUES (?,?,?)`,
		instanceID, sessionID, tag)
	return err
}

// RemoveTag 移除标签。
func RemoveTag(ctx context.Context, db *index.DB, instanceID, sessionID, tag string) error {
	_, err := db.SQL().ExecContext(ctx,
		`DELETE FROM session_tags WHERE agent_instance_id=? AND session_id=? AND tag=?`,
		instanceID, sessionID, tag)
	return err
}

// SetFavorite 设置收藏。
func SetFavorite(ctx context.Context, db *index.DB, instanceID, sessionID string, fav bool) error {
	favInt := 0
	if fav {
		favInt = 1
	}
	_, err := db.SQL().ExecContext(ctx, `
		INSERT INTO session_meta (agent_instance_id, session_id, favorite, note)
		VALUES (?,?,?, '')
		ON CONFLICT(agent_instance_id, session_id) DO UPDATE SET favorite=excluded.favorite`,
		instanceID, sessionID, favInt)
	return err
}

// SetNote 设置备注。
func SetNote(ctx context.Context, db *index.DB, instanceID, sessionID, note string) error {
	_, err := db.SQL().ExecContext(ctx, `
		INSERT INTO session_meta (agent_instance_id, session_id, favorite, note)
		VALUES (?,?,0,?)
		ON CONFLICT(agent_instance_id, session_id) DO UPDATE SET note=excluded.note`,
		instanceID, sessionID, note)
	return err
}

// ByTag 查询带指定标签的会话。
func ByTag(ctx context.Context, db *index.DB, tag string) ([]SessionRef, error) {
	rows, err := db.SQL().QueryContext(ctx,
		`SELECT t.agent_instance_id, t.session_id
		 FROM session_tags t WHERE t.tag=? ORDER BY t.session_id`, tag)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SessionRef
	for rows.Next() {
		var r SessionRef
		if err := rows.Scan(&r.AgentInstanceID, &r.SessionID); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// Favorites 列出全部收藏会话。
func Favorites(ctx context.Context, db *index.DB) ([]SessionRef, error) {
	rows, err := db.SQL().QueryContext(ctx,
		`SELECT agent_instance_id, session_id FROM session_meta WHERE favorite=1 ORDER BY session_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SessionRef
	for rows.Next() {
		var r SessionRef
		if err := rows.Scan(&r.AgentInstanceID, &r.SessionID); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// SessionRef 是会话引用。
type SessionRef struct {
	AgentInstanceID string
	SessionID       string
}

func splitTags(csv string) []string {
	var out []string
	for _, t := range strings.Split(csv, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}
