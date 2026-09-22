package opencode

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/talea/talea/internal/model"
)

// OpenCode v2 测试夹具：在 t.TempDir() 内动态生成 session_v2 / session_message
// 等价结构（表结构与 JSON 样例来自实测、已脱敏），不引入真实数据库或真实用户内容。

// sessionV2DDL 是 session_v2 测试表结构（列与实测 2.0.12 对齐，脱敏夹具）。
const sessionV2DDL = `CREATE TABLE session_v2 (
	id TEXT NOT NULL PRIMARY KEY,
	project_id TEXT, workspace_id TEXT, parent_id TEXT, slug TEXT,
	directory TEXT NOT NULL, path TEXT, title TEXT,
	version TEXT, metadata TEXT, cost REAL,
	tokens_input INTEGER, tokens_output INTEGER,
	tokens_reasoning INTEGER, tokens_cache_read INTEGER, tokens_cache_write INTEGER,
	time_created INTEGER, time_updated INTEGER,
	time_compacting INTEGER, time_archived INTEGER,
	agent TEXT, model TEXT, permission TEXT,
	fork_session_id TEXT, fork_boundary INTEGER, time_suspended INTEGER,
	resume_attempts INTEGER, time_idle INTEGER, time_viewed INTEGER, idle_outcome TEXT)`

// sessionMessageDDL 是 session_message 测试表结构。
const sessionMessageDDL = `CREATE TABLE session_message (
	id TEXT, session_id TEXT, type TEXT, seq INTEGER,
	time_created INTEGER, time_updated INTEGER, data TEXT)`

// openTestDB 以读写方式打开测试数据库，测试结束自动关闭。
func openTestDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// execAll 顺序执行 SQL，任一失败即终止测试。
func execAll(t *testing.T, db *sql.DB, stmts []string) {
	t.Helper()
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
}

// createV2FixtureDB 构造只有 session_v2 / session_message 的 v2 形态测试库（脱敏）。
func createV2FixtureDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "opencode.db")
	db, openErr := sql.Open("sqlite", "file:"+path)
	if openErr != nil {
		t.Fatal(openErr)
	}
	defer db.Close()

	execAll(t, db, []string{sessionV2DDL, sessionMessageDDL})
	execAll(t, db, []string{
		`INSERT INTO session_v2 (id, directory, path, title, tokens_input, tokens_output,
			tokens_reasoning, tokens_cache_read, tokens_cache_write,
			time_created, time_updated, agent, model)
		 VALUES ('ses_new', '/home/alice/code/nexora', 'home/alice/code/nexora', '新会话标题',
		 5000, 3000, 100, 2000, 50, 1789800000000, 1789800100000, 'build', '{"id":"m1"}')`,
		// title 为 NULL、token 全零的 v2 行：未知不得显示为已知 0。
		`INSERT INTO session_v2 (id, directory, title, tokens_input, tokens_output,
			tokens_reasoning, tokens_cache_read, tokens_cache_write, time_created, time_updated)
		 VALUES ('ses_v2null', '/home/alice/code/nexora', NULL, 0, 0, 0, 0, 0,
		 1789800200000, 1789800300000)`,
		`INSERT INTO session_v2 (id, directory, title, time_created, time_updated)
		 VALUES ('ses_v2empty', '/home/alice/code/nexora', '无消息', 1789800400000, 1789800500000)`,
		`INSERT INTO session_v2 (id, directory, title, time_created, time_updated)
		 VALUES ('ses_v2inject', '/home/alice/code/nexora', '注入过滤', 1789800600000, 1789800700000)`,
		// user 首问
		`INSERT INTO session_message (id, session_id, type, seq, time_created, time_updated, data)
		 VALUES ('sm_u1', 'ses_new', 'user', 1, 1789800001000, 1789800001000,
		  '{"time":{"created":1789800001000},"text":"新会话的第一个问题","files":[],"agents":[]}')`,
		// assistant：文本块 + 已完成的工具块 + 单次增量 tokens
		`INSERT INTO session_message (id, session_id, type, seq, time_created, time_updated, data)
		 VALUES ('sm_a1', 'ses_new', 'assistant', 2, 1789800002000, 1789800002000,
		  '{"agent":"build","model":{"id":"m1","providerID":"mock","variant":"v"},
		    "content":[{"type":"text","text":"正在查看文件。"},
		      {"type":"tool","id":"call_1","name":"bash",
		        "state":{"status":"completed","input":{"filePath":"/home/alice/code/nexora/main.go"},
		          "content":[],"metadata":{}},
		        "time":{"created":1789800002100,"completed":1789800002900}}],
		    "finish":"stop","cost":0.01,
		    "tokens":{"input":232,"output":44,"reasoning":0,"cache":{"read":100,"write":10}},
		    "time":{"created":1789800002000}}')`,
		// assistant 第二条：另一批增量
		`INSERT INTO session_message (id, session_id, type, seq, time_created, time_updated, data)
		 VALUES ('sm_a2', 'ses_new', 'assistant', 3, 1789800003000, 1789800003000,
		  '{"model":{"id":"m1"},"content":[{"type":"text","text":"完成。"}],
		    "tokens":{"input":44487,"output":1000,"reasoning":500,"cache":{"read":0,"write":0}},
		    "time":{"created":1789800003000}}')`,
		// compaction
		`INSERT INTO session_message (id, session_id, type, seq, time_created, time_updated, data)
		 VALUES ('sm_c1', 'ses_new', 'compaction', 4, 1789800004000, 1789800004000,
		  '{"status":"compacted","reason":"auto","summary":"摘要","recent":"最近内容",
		    "time":{"created":1789800004000}}')`,
		// 首问含注入块：过滤后应返回真实提问
		`INSERT INTO session_message (id, session_id, type, seq, time_created, time_updated, data)
		 VALUES ('sm_j1', 'ses_v2inject', 'user', 1, 1789800600100, 1789800600100,
		  '{"time":{"created":1789800600100},"text":"<system-reminder>忽略此提醒</system-reminder>\n真实提问"}')`,
	})
	return path
}

// addSessionV2Tables 在既有传统测试库上补充 session_v2，构造两表并存形态：
// ses_0001 两表都有（v2 行更新且 title 为 NULL），ses_legacy_new 仅在传统表且在增量窗口内。
func addSessionV2Tables(t *testing.T, path string) {
	t.Helper()
	db := openTestDB(t, path)
	execAll(t, db, []string{
		sessionV2DDL,
		`INSERT INTO session_v2 (id, directory, title, tokens_input, tokens_output,
			tokens_reasoning, tokens_cache_read, tokens_cache_write, time_created, time_updated, agent)
		 VALUES ('ses_0001', '/home/alice/v2dir', NULL, 700, 400, 0, 300, 20,
		 1785739928049, 1785999999999, 'build')`,
		`INSERT INTO session_v2 (id, directory, title, time_created, time_updated)
		 VALUES ('ses_v2only', '/home/alice/code/nexora', '仅 v2', 1786000000000, 1786000000001)`,
		// 仅在传统表、但处于增量高水位窗口内的遗留会话
		`INSERT INTO session (id, directory, title, time_created, time_updated)
		 VALUES ('ses_legacy_new', '/home/alice/code/legacy', '窗口内遗留', 1785999800000, 1785999800000)`,
	})
}

// writeGarbageDB 写入非 SQLite 内容的文件，模拟不可读数据库。
func writeGarbageDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "opencode.db")
	if err := os.WriteFile(path, []byte("not a sqlite database"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// discoverIDs 执行完整发现并返回会话 ID 集合。
func discoverIDs(t *testing.T, dbPath string) map[string]int64 {
	t.Helper()
	t.Setenv("OPENCODE_DB", dbPath)
	sources, err := New().Discover(context.Background(), model.AgentInstance{
		InstanceID:    "t",
		AgentID:       model.AgentOpenCode,
		DataDirectory: filepath.Dir(dbPath),
	})
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string]int64, len(sources))
	for _, src := range sources {
		out[src.SessionID] = src.Mtime
	}
	return out
}
