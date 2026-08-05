package index

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/talea/talea/internal/model"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return db
}

func mkSession(id string) *model.Session {
	now := time.Now()
	return &model.Session{
		AgentID:          model.AgentClaudeCode,
		AgentInstanceID:  "inst-1",
		SessionID:        id,
		FirstQuestion:    "问题 " + id,
		WorkingDirectory: "/tmp/proj",
		StartedAt:        &now,
		EndedAt:          &now,
		Activity:         model.ActivityInactive,
		IndexedAt:        now,
		UpdatedAt:        now,
	}
}

func TestMigrateAndUpsert(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	s := mkSession("s1")
	in := int64(1000)
	out := int64(200)
	total := int64(1200)
	s.HasTokenUsage = true
	s.TokenUsage = &model.TokenUsage{
		InputTokens:  &in,
		OutputTokens: &out,
		TotalTokens:  &total,
		Source:       model.UsageSourceMessageMetadata,
	}

	if err := db.UpsertSession(ctx, s); err != nil {
		t.Fatal(err)
	}
	n, err := db.Count(ctx)
	if err != nil || n != 1 {
		t.Fatalf("count=%d err=%v", n, err)
	}
}

func TestUpsertMany(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	now := time.Now()
	s1 := mkSession("a")
	s1.IndexedAt = now
	s1.UpdatedAt = now
	s2 := mkSession("b")
	s2.IndexedAt = now
	s2.UpdatedAt = now

	st, err := db.UpsertMany(ctx, []*model.Session{s1, s2})
	if err != nil {
		t.Fatal(err)
	}
	if st.Added != 2 {
		t.Fatalf("added=%d", st.Added)
	}

	// 再次写入应为更新
	s2.UpdatedAt = time.Now()
	st, err = db.UpsertMany(ctx, []*model.Session{s2})
	if err != nil {
		t.Fatal(err)
	}
	if st.Updated != 1 {
		t.Fatalf("updated=%d", st.Updated)
	}
}

func TestLoadTracked(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	s := mkSession("x")
	s.SourceMtime = 12345
	s.SourceSize = 678
	if err := db.UpsertSession(ctx, s); err != nil {
		t.Fatal(err)
	}
	tracked, err := db.LoadTracked(ctx)
	if err != nil {
		t.Fatal(err)
	}
	key := s.AgentInstanceID + "\x00" + s.SessionID
	tk, ok := tracked[key]
	if !ok {
		t.Fatal("expected tracked session")
	}
	if tk.SourceMtime != 12345 || tk.SourceSize != 678 {
		t.Fatalf("tracked=%+v", tk)
	}
}

func TestTimelineEventDedup(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	e := &model.UsageTimelineEvent{
		AgentInstanceID: "i",
		SessionID:       "s",
		EventType:       model.UsageEventRequest,
		SourceIdentity:  "req-1",
		Sequence:        1,
		TotalTokens:     int64Ptr(500),
	}
	ok, err := db.UpsertTimelineEvent(ctx, e)
	if err != nil || !ok {
		t.Fatalf("first insert: ok=%v err=%v", ok, err)
	}
	// 相同 source_identity 应被忽略
	ok, err = db.UpsertTimelineEvent(ctx, e)
	if err != nil || ok {
		t.Fatalf("dup insert should be ignored: ok=%v err=%v", ok, err)
	}
}

func TestPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("permissions not 0600: %s", info.Mode().Perm())
	}
}

func int64Ptr(v int64) *int64 { return &v }

func TestBackupIfVersionChange(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	// 手动把 schema 版本改成旧版，模拟版本升级场景
	if _, err := db.sql.ExecContext(ctx,
		`UPDATE schema_meta SET value='0' WHERE key='schema_version'`); err != nil {
		t.Fatal(err)
	}

	// 备份应生成 .bak 文件
	if err := db.backupIfVersionChange(ctx); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(db.dir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range entries {
		if e.Name() == "index.db.v0.bak-"+"x" {
			continue
		}
		if strings.Contains(e.Name(), ".bak-") {
			found = true
		}
	}
	if !found {
		t.Fatalf("未找到备份文件: %v", entries)
	}
}

// 版本一致时不备份
func TestBackupIfVersionSame(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	// 版本已是 SchemaVersion，不应备份
	if err := db.backupIfVersionChange(ctx); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(db.dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".bak-") {
			t.Fatalf("不应生成备份: %s", e.Name())
		}
	}
}
