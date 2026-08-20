package syncer

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/config"
	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
	"github.com/talea/talea/internal/search"
)

// mkSession 构造测试会话。
func mkSession(id, cwd string) *model.Session {
	now := time.Now()
	return &model.Session{
		AgentID:          model.AgentOpenCode,
		AgentInstanceID:  "opencode-test",
		SessionID:        id,
		FirstQuestion:    "sync test " + id,
		WorkingDirectory: cwd,
		StartedAt:        &now,
		EndedAt:          &now,
		LastActivityAt:   &now,
		Activity:         model.ActivityInactive,
		IndexedAt:        now,
		UpdatedAt:        now,
	}
}

// TestSyncNoRegistryIsNoop 验证空注册表（无可探测 Agent）时 Sync 幂等且不报错。
func TestSyncNoRegistryIsNoop(t *testing.T) {
	ctx := context.Background()
	db, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	a := &app.App{Registry: adapters.NewRegistry(), Config: config.Default()}
	if err := Sync(ctx, a, db); err != nil {
		t.Fatalf("Sync on empty registry: %v", err)
	}
}

// TestSyncMakesNewSessionVisible 验证关键环节：DB 中已有会话但 FTS 尚未同步时，
// 调用 Sync 后 search.List 即可见（增量索引写入的新会话通过本函数完成 FTS 同步）。
func TestSyncMakesNewSessionVisible(t *testing.T) {
	ctx := context.Background()
	db, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	a := &app.App{Registry: adapters.NewRegistry(), Config: config.Default()}

	// 模拟上次索引后出现的新会话：已入库但 FTS 未同步
	if err := db.UpsertSession(ctx, mkSession("ses_new", "/home/user/x")); err != nil {
		t.Fatal(err)
	}
	// Sync 前不应可见
	if err := Sync(ctx, a, db); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	results, err := search.List(ctx, db, search.Query{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Session.SessionID != "ses_new" {
		t.Fatalf("after Sync, got %d results, want ses_new visible", len(results))
	}
}
