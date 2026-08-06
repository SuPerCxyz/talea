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

// TestByIDPrefix 验证按 session_id 前缀查找（不经 FTS，短前缀可用），
// 以及 agent 过滤与不存在的场景。
func TestByIDPrefix(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	insertSession(t, db, mkSession("ses_0a037a156ffeHPmbe15W", "q1", "/home/alice", "opencode"))
	insertSession(t, db, mkSession("ses_0a037a156ffeHPmbe15X", "q2", "/home/alice", "opencode"))
	insertSession(t, db, mkSession("abc-123", "q3", "/home/alice/code", "claude-code"))
	if err := Populate(ctx, db); err != nil {
		t.Fatal(err)
	}

	// 前缀唯一
	res, err := ByIDPrefix(ctx, db, "ses_0a037a156ffeHPmbe15W", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Session.SessionID != "ses_0a037a156ffeHPmbe15W" {
		t.Fatalf("unique prefix: got %d results", len(res))
	}

	// 前缀多候选 + agent 过滤
	res, err = ByIDPrefix(ctx, db, "ses_0a037a156ffeHPmbe15", "opencode", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 {
		t.Fatalf("prefix+agent: got %d results", len(res))
	}

	// 前缀不存在
	res, err = ByIDPrefix(ctx, db, "nope", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 0 {
		t.Fatalf("missing prefix: got %d results", len(res))
	}
}

// TestSearchTermAndAgent 回归：Term（FTS）与 Agent 过滤组合时参数占位符不得错位，
// 否则 agent 值会传给 EXISTS 的 MATCH、ftsTerm 传给 agent_id 比较导致 0 结果。
func TestSearchTermAndAgent(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	insertSession(t, db, mkSession("ses_0a037a156ffeHPmbe15W", "multipath 残留", "/home/alice", "opencode"))
	insertSession(t, db, mkSession("abc-123", "multipath 清理", "/home/alice/code/cinder", "claude-code"))
	if err := Populate(ctx, db); err != nil {
		t.Fatal(err)
	}

	res, err := Search(ctx, db, Query{Term: "ses_0a037a156ffeHPmbe15W", Agent: "opencode", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Session.SessionID != "ses_0a037a156ffeHPmbe15W" {
		t.Fatalf("term+agent: got %d results", len(res))
	}

	res, err = Search(ctx, db, Query{Term: "multipath", Agent: "claude-code", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Session.AgentID != "claude-code" {
		t.Fatalf("keyword+agent: got %d results", len(res))
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
