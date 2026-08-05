// Package index 集成测试：使用真实测试夹具完整跑通发现→解析→索引→查询。
package index_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/adapters/claude"
	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
	"github.com/talea/talea/internal/search"
)

// TestIndexFixtures 用 testdata 夹具建立索引并校验查询。
func TestIndexFixtures(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	// 指向夹具目录的 AgentInstance
	fixtureRoot, err := filepath.Abs(filepath.Join("..", "..", "testdata"))
	if err != nil {
		t.Fatal(err)
	}
	_ = fixtureRoot

	db, err := index.Open(filepath.Join(tmp, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := search.Ensure(ctx, db); err != nil {
		t.Fatal(err)
	}

	// 手工构造夹具 session 并写入（避免依赖真实 HOME 探测）
	ad := claude.New()
	inst := model.AgentInstance{
		InstanceID: "claude-test",
		AgentID:    model.AgentClaudeCode,
	}
	srcDir := filepath.Join("..", "..", "testdata", "claude")
	entries, _ := os.ReadDir(srcDir)
	var sessions []*model.Session
	for _, e := range entries {
		if !e.IsDir() {
			src := adapters.SessionSource{
				SessionID: sessionIDFromFile(e.Name()),
				Path:      filepath.Join(srcDir, e.Name()),
			}
			s, err := ad.ParseMetadata(ctx, inst, src)
			if err != nil {
				t.Fatalf("parse %s: %v", e.Name(), err)
			}
			s.IndexedAt = s.UpdatedAt
			sessions = append(sessions, s)
		}
	}
	if len(sessions) == 0 {
		t.Fatal("no fixture sessions")
	}
	st, err := db.UpsertMany(ctx, sessions)
	if err != nil {
		t.Fatal(err)
	}
	if st.Added != len(sessions) {
		t.Fatalf("added=%d want=%d", st.Added, len(sessions))
	}
	if err := search.Populate(ctx, db); err != nil {
		t.Fatal(err)
	}

	res, err := search.Search(ctx, db, search.Query{Term: "VSCode", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) == 0 {
		t.Fatal("expected VSCode search result")
	}
}

func sessionIDFromFile(name string) string {
	return name[:len(name)-len(filepath.Ext(name))]
}
