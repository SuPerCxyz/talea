package transfer

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
	"github.com/talea/talea/internal/tags"
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
	return db
}

func TestExportImportRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	now := time.Now()
	in := int64(1000)
	s := &model.Session{
		AgentID:          model.AgentClaudeCode,
		AgentInstanceID:  "i",
		SessionID:        "s1",
		FirstQuestion:    "测试问题",
		WorkingDirectory: "/tmp",
		StartedAt:        &now,
		EndedAt:          &now,
		IndexedAt:        now,
		UpdatedAt:        now,
		TokenUsage: &model.TokenUsage{
			TotalTokens: &in,
			InputTokens: &in,
		},
		HasTokenUsage: true,
	}
	if err := db.UpsertSession(ctx, s); err != nil {
		t.Fatal(err)
	}
	tags.SetTags(ctx, db, "i", "s1", "重要")
	tags.SetFavorite(ctx, db, "i", "s1", true)

	// 导出
	out := filepath.Join(t.TempDir(), "export.json")
	if err := Export(ctx, db, out); err != nil {
		t.Fatal(err)
	}

	// 导入到新库
	db2 := newDB(t)
	n, err := Import(ctx, db2, out)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("imported=%d", n)
	}

	// 验证
	var count int
	db2.SQL().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sessions WHERE session_id='s1'`).Scan(&count)
	if count != 1 {
		t.Fatalf("sessions=%d", count)
	}
	m, _ := tags.Get(ctx, db2, "i", "s1")
	if !m.Favorite || len(m.Tags) != 1 {
		t.Fatalf("meta=%+v", m)
	}

	// 重复导入跳过
	n, _ = Import(ctx, db2, out)
	if n != 0 {
		t.Fatalf("dup import=%d, want 0", n)
	}
}

func TestImportMissingFile(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	if _, err := Import(ctx, db, "/nonexistent/file.json"); err == nil {
		t.Fatal("expected error")
	}
}
