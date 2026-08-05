package search

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
)

func newDB(t *testing.T) *index.DB {
	t.Helper()
	db, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := Ensure(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return db
}

func insertSession(t *testing.T, db *index.DB, s *model.Session) {
	t.Helper()
	if err := db.UpsertSession(context.Background(), s); err != nil {
		t.Fatal(err)
	}
}

func mkSession(id, question, cwd, agent string) *model.Session {
	now := time.Now()
	return &model.Session{
		AgentID:          model.AgentID(agent),
		AgentInstanceID:  agent + "-inst",
		SessionID:        id,
		FirstQuestion:    question,
		WorkingDirectory: cwd,
		StartedAt:        &now,
		EndedAt:          &now,
		LastActivityAt:   &now,
		Activity:         model.ActivityInactive,
		IndexedAt:        now,
		UpdatedAt:        now,
	}
}

func TestSearchByIDAndFTS(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	insertSession(t, db, mkSession("abc-123", "修复 multipath 残留问题", "/home/alice/code/cinder", "claude-code"))
	insertSession(t, db, mkSession("def-456", "分析疏散虚机后设备残留", "/home/alice/code/nov", "opencode"))
	if err := Populate(ctx, db); err != nil {
		t.Fatal(err)
	}

	// 会话 ID 精确匹配
	res, err := Search(ctx, db, Query{Term: "abc-123", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Session.SessionID != "abc-123" {
		t.Fatalf("id search: %d results", len(res))
	}

	// 中文 3 字以上 trigram 搜索
	res, err = Search(ctx, db, Query{Term: "multipath", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) == 0 {
		t.Fatal("expected multipath match")
	}

	// 中文搜索
	res, err = Search(ctx, db, Query{Term: "疏散虚机", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) == 0 {
		t.Fatal("expected 疏散虚机 match")
	}
}

func TestSearchFilters(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	insertSession(t, db, mkSession("s1", "问题一", "/home/alice/code/a", "claude-code"))
	insertSession(t, db, mkSession("s2", "问题二", "/home/alice/code/b", "opencode"))
	if err := Populate(ctx, db); err != nil {
		t.Fatal(err)
	}

	res, err := Search(ctx, db, Query{Agent: "opencode", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Session.AgentID != "opencode" {
		t.Fatalf("agent filter: %d results", len(res))
	}

	res, err = Search(ctx, db, Query{Cwd: "/home/alice/code/b", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Session.SessionID != "s2" {
		t.Fatalf("cwd filter: %d results", len(res))
	}
}

func TestListEmpty(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	res, err := List(ctx, db, Query{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 0 {
		t.Fatalf("expected empty, got %d", len(res))
	}
}
