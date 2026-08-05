package tags

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/talea/talea/internal/index"
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

func TestSetAndGetTags(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	if err := SetTags(ctx, db, "i", "s1", "重要, 待办"); err != nil {
		t.Fatal(err)
	}
	m, err := Get(ctx, db, "i", "s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Tags) != 2 || m.Tags[0] != "待办" || m.Tags[1] != "重要" {
		t.Fatalf("tags=%v", m.Tags)
	}
	// 追加与移除
	if err := AddTag(ctx, db, "i", "s1", "bug"); err != nil {
		t.Fatal(err)
	}
	if err := RemoveTag(ctx, db, "i", "s1", "待办"); err != nil {
		t.Fatal(err)
	}
	m, _ = Get(ctx, db, "i", "s1")
	if len(m.Tags) != 2 {
		t.Fatalf("after add/remove tags=%v", m.Tags)
	}
}

func TestFavoriteAndNote(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	if err := SetFavorite(ctx, db, "i", "s1", true); err != nil {
		t.Fatal(err)
	}
	if err := SetNote(ctx, db, "i", "s1", "说明"); err != nil {
		t.Fatal(err)
	}
	m, err := Get(ctx, db, "i", "s1")
	if err != nil {
		t.Fatal(err)
	}
	if !m.Favorite {
		t.Fatal("expected favorite")
	}
	if m.Note != "说明" {
		t.Fatalf("note=%q", m.Note)
	}
	favs, err := Favorites(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(favs) != 1 {
		t.Fatalf("favorites=%d", len(favs))
	}
}

func TestByTag(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	SetTags(ctx, db, "i", "s1", "x")
	SetTags(ctx, db, "i", "s2", "x")
	refs, err := ByTag(ctx, db, "x")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 2 {
		t.Fatalf("refs=%d", len(refs))
	}
}
