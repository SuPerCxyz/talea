package opencode

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/model"
)

// createFixtureDB 生成 OpenCode 测试数据库。
func createFixtureDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "opencode.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE session (
			id TEXT, project_id TEXT, workspace_id TEXT, parent_id TEXT, slug TEXT,
			directory TEXT, path TEXT, title TEXT, version TEXT, metadata TEXT,
			cost REAL, tokens_input INTEGER, tokens_output INTEGER,
			tokens_reasoning INTEGER, tokens_cache_read INTEGER, tokens_cache_write INTEGER,
			time_created INTEGER, time_updated INTEGER, time_compacting INTEGER,
			time_archived INTEGER, agent TEXT, model TEXT)`,
		`CREATE TABLE message (
			id TEXT, session_id TEXT, time_created INTEGER, time_updated INTEGER, data TEXT)`,
		`CREATE TABLE part (
			id TEXT, message_id TEXT, session_id TEXT, time_created INTEGER, time_updated INTEGER, data TEXT)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatal(err)
		}
	}

	_, err = db.Exec(`INSERT INTO session VALUES
		('ses_0001','','','','','/home/alice/code/nexora','home/alice/code/nexora',
		 '未完成任务','','',0.0,123456,23456,1024,45678,0,
		 1785739928049,1785919370604,NULL,NULL,'build','{"id":"m1"}')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO message VALUES
		('msg_001','ses_0001',1785739928049,1785739928049,
		 '{"role":"user","time":{"created":1785739928049}}'),
		('msg_002','ses_0001',1785740000000,1785740000000,
		 '{"role":"assistant","time":{"created":1785740000000},"tokens":{"total":150000,"input":120000,"output":30000}}')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO part VALUES
		('prt_001','msg_001','ses_0001',1785739928049,1785739928049,
		 '{"type":"text","text":"请查看还有哪些未完成的任务"}'),
		('prt_002','msg_002','ses_0001',1785740000000,1785740000000,
		 '{"type":"text","text":"好的，已查看。"}')`)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseMetadataSimple(t *testing.T) {
	ctx := context.Background()
	path := createFixtureDB(t)
	a := New()
	src := adapters.SessionSource{
		SessionID: "ses_0001",
		Path:      path,
		SourceID:  "opencode-session:ses_0001",
	}
	inst := model.AgentInstance{InstanceID: "t", AgentID: model.AgentOpenCode}
	s, err := a.ParseMetadata(ctx, inst, src)
	if err != nil {
		t.Fatal(err)
	}
	if s.FirstQuestion != "请查看还有哪些未完成的任务" {
		t.Fatalf("first question: %q", s.FirstQuestion)
	}
	if s.WorkingDirectory != "/home/alice/code/nexora" {
		t.Fatalf("cwd: %q", s.WorkingDirectory)
	}
	if s.StartTimeSource != model.TimeSourceSessionMeta {
		t.Fatalf("start source: %q", s.StartTimeSource)
	}
	if !s.HasTokenUsage || s.TokenUsage == nil {
		t.Fatal("expected token usage")
	}
	if s.TokenUsage.InputTokens == nil || *s.TokenUsage.InputTokens != 123456 {
		t.Fatalf("input: %v", s.TokenUsage.InputTokens)
	}
	if s.TokenUsage.CacheReadTokens == nil || *s.TokenUsage.CacheReadTokens != 45678 {
		t.Fatalf("cache read: %v", s.TokenUsage.CacheReadTokens)
	}
	if s.TokenUsage.TotalTokens == nil || *s.TokenUsage.TotalTokens != 146912 {
		t.Fatalf("total: %v", s.TokenUsage.TotalTokens)
	}
}

func TestBuildResumeCommand(t *testing.T) {
	a := New()
	cmd, err := a.BuildResumeCommand(model.Session{SessionID: "ses_0001"}, "/work")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Program != "opencode" {
		t.Fatalf("program: %q", cmd.Program)
	}
	if len(cmd.Args) != 2 || cmd.Args[0] != "-s" {
		t.Fatalf("args: %v", cmd.Args)
	}
}
