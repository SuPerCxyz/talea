package index

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestScanStateRoundTrip(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	want := ScanState{AgentInstanceID: "inst", Fingerprint: "fp", Cursor: "cursor"}
	if err := db.SaveScanState(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, ok, err := db.LoadScanState(ctx, want.AgentInstanceID)
	if err != nil || !ok {
		t.Fatalf("load state: ok=%v err=%v", ok, err)
	}
	if got.Fingerprint != want.Fingerprint || got.Cursor != want.Cursor {
		t.Fatalf("state=%+v, want=%+v", got, want)
	}
}

func TestMigrateAddsStateToV1Index(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	stmts := []string{
		`CREATE TABLE schema_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`INSERT INTO schema_meta VALUES ('schema_version', '1')`,
		`CREATE TABLE sessions (
			agent_id TEXT NOT NULL, agent_instance_id TEXT NOT NULL, session_id TEXT NOT NULL,
			last_activity_at INTEGER, working_directory TEXT)`,
	}
	for _, stmt := range stmts {
		if _, err := db.SQL().ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	var columnCount int
	if err := db.SQL().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info('sessions') WHERE name='fts_dirty'`).Scan(&columnCount); err != nil {
		t.Fatal(err)
	}
	if columnCount != 1 {
		t.Fatalf("fts_dirty column count=%d", columnCount)
	}
	var resumeColumn int
	if err := db.SQL().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info('sessions') WHERE name='resume_launch_args_json'`).Scan(&resumeColumn); err != nil {
		t.Fatal(err)
	}
	if resumeColumn != 1 {
		t.Fatalf("resume_launch_args_json column count=%d", resumeColumn)
	}
	var stateTable int
	if err := db.SQL().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='source_scan_state'`).Scan(&stateTable); err != nil {
		t.Fatal(err)
	}
	if stateTable != 1 {
		t.Fatalf("source_scan_state table count=%d", stateTable)
	}
}

func TestFingerprintSourcesChangesWithFileAndDirectory(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "one.jsonl")
	if err := os.WriteFile(path, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	first, valid, err := FingerprintSources(root, []string{path})
	if err != nil || !valid {
		t.Fatalf("first fingerprint: valid=%v err=%v", valid, err)
	}
	second, valid, err := FingerprintSources(root, []string{path})
	if err != nil || !valid || first != second {
		t.Fatalf("stable fingerprint: first=%q second=%q valid=%v err=%v", first, second, valid, err)
	}
	if err := os.WriteFile(path, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	third, valid, err := FingerprintSources(root, []string{path})
	if err != nil || !valid || third == first {
		t.Fatalf("changed fingerprint: first=%q third=%q valid=%v err=%v", first, third, valid, err)
	}
}

func TestFingerprintMissingPathIsStable(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "missing.jsonl")
	first, valid, err := FingerprintSources(root, []string{path})
	if err != nil || !valid {
		t.Fatalf("missing fingerprint: valid=%v err=%v", valid, err)
	}
	second, valid, err := FingerprintSources(root, []string{path})
	if err != nil || !valid || first != second {
		t.Fatalf("missing fingerprint should be stable: %q != %q", first, second)
	}
}

func BenchmarkFingerprintSourcesNoChange(b *testing.B) {
	root := b.TempDir()
	path := filepath.Join(root, "sessions", "one.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("synthetic session metadata"), 0o600); err != nil {
		b.Fatal(err)
	}
	paths := []string{path}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, valid, err := FingerprintSources(root, paths); err != nil || !valid {
			b.Fatalf("fingerprint: valid=%v err=%v", valid, err)
		}
	}
}
