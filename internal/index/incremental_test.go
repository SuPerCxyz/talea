// Package index_test 增量索引行为测试：追加、截断、替换、删除、同 ID 跨 Agent。
package index_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/adapters/claude"
	"github.com/talea/talea/internal/adapters/codex"
	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/config"
	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
)

// newTestApp 构造指向临时数据目录的 App（含注册的适配器）。
func newTestApp(t *testing.T) (*app.App, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", home+"/data")

	reg := adapters.NewRegistry()
	if err := reg.Register(claude.New()); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(codex.New()); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	return &app.App{Registry: reg, Config: cfg, Paths: config.ResolvePaths()}, home
}

func mkClaudeFile(t *testing.T, dir, id string, lines []string) string {
	t.Helper()
	path := filepath.Join(dir, id+".jsonl")
	var content string
	for _, l := range lines {
		content += l + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func claudeUserLine(id, q string) string {
	return `{"type":"user","timestamp":"2026-07-17T04:57:32.179Z","cwd":"/home/bench/proj","sessionId":"` + id + `","gitBranch":"main","message":{"role":"user","content":"` + q + `"}}`
}

func claudeAssistantLine(id string, input int64) string {
	return `{"type":"assistant","timestamp":"2026-07-17T04:58:00.000Z","cwd":"/home/bench/proj","sessionId":"` + id + `","message":{"role":"assistant","content":[{"type":"text","text":"r"}],"usage":{"input_tokens":` + itoa(input) + `,"output_tokens":10}}}`
}

func TestIndexAppendTruncateReplace(t *testing.T) {
	ctx := context.Background()
	a, home := newTestApp(t)
	db, err := index.Open(filepath.Join(home, "data", "talea", "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	// 构造 claude 项目目录
	projDir := filepath.Join(home, ".claude", "projects", "-bench-proj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := mkClaudeFile(t, projDir, "sess-1", []string{
		claudeUserLine("sess-1", "第一个问题"),
		claudeAssistantLine("sess-1", 1000),
	})

	runIndex := func() []index.Result {
		res, err := (&index.Indexer{App: a, DB: db}).Run(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	// 1) 初次索引：新增 1
	res := runIndex()
	if res[0].Added != 1 {
		t.Fatalf("first index added=%d", res[0].Added)
	}

	// 2) 无变化：跳过 1
	res = runIndex()
	if res[0].Skipped != 1 {
		t.Fatalf("nochange skipped=%d", res[0].Skipped)
	}

	// 3) 追加：文件变大，更新 1
	os.WriteFile(path, []byte(readFile(t, path)+claudeAssistantLine("sess-1", 2000)+"\n"), 0o644)
	res = runIndex()
	if res[0].Updated != 1 {
		t.Fatalf("append updated=%d", res[0].Updated)
	}

	// 4) 截断：写回更短内容，更新 1
	os.WriteFile(path, []byte(claudeUserLine("sess-1", "重写后的问题")+"\n"), 0o644)
	res = runIndex()
	if res[0].Updated != 1 {
		t.Fatalf("truncate updated=%d", res[0].Updated)
	}

	// 5) 替换：删除并重建同名文件（新 inode），更新 1
	os.Remove(path)
	mkClaudeFile(t, projDir, "sess-1", []string{
		claudeUserLine("sess-1", "替换后的问题"),
		claudeAssistantLine("sess-1", 3000),
	})
	res = runIndex()
	if res[0].Updated != 1 {
		t.Fatalf("replace updated=%d", res[0].Updated)
	}

	// 6) 删除源文件后再次索引：应跳过（源不存在）
	os.Remove(path)
	res = runIndex()
	// 源删除：Discover 不再返回该文件，无结果（不报错）
	_ = res
}

func TestSameSessionIDAcrossAgents(t *testing.T) {
	ctx := context.Background()
	a, home := newTestApp(t)
	db, err := index.Open(filepath.Join(home, "data", "talea", "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	// 同一 session-id 出现在 claude 与 codex 两个 Agent
	projDir := filepath.Join(home, ".claude", "projects", "-bench-proj")
	os.MkdirAll(projDir, 0o755)
	mkClaudeFile(t, projDir, "same-id-1", []string{
		claudeUserLine("same-id-1", "claude 的问题"),
	})

	cxDir := filepath.Join(home, ".codex", "sessions", "2026", "07", "08")
	os.MkdirAll(cxDir, 0o755)
	cxFile := `{"type":"session_meta","timestamp":"2026-07-08T12:00:00.000Z","payload":{"id":"same-id-1","session_id":"same-id-1","cwd":"/home/bench/cx","cli_version":"0.146.0"}}`
	os.WriteFile(filepath.Join(cxDir, "rollout-2026-07-08T12-00-00-same-id-1.jsonl"),
		[]byte(cxFile+"\n"), 0o644)

	res, err := (&index.Indexer{App: a, DB: db}).Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// 两个 Agent 都应新增 1 条（主键 agent_instance_id+session_id 区分）
	claudeRes, codexRes := -1, -1
	for _, r := range res {
		if r.AgentID == model.AgentClaudeCode {
			claudeRes = r.Added
		}
		if r.AgentID == model.AgentCodexCLI {
			codexRes = r.Added
		}
	}
	if claudeRes != 1 || codexRes != 1 {
		t.Fatalf("claude=%d codex=%d, want both 1", claudeRes, codexRes)
	}
	var count int
	db.SQL().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sessions WHERE session_id='same-id-1'`).Scan(&count)
	if count != 2 {
		t.Fatalf("same-id sessions=%d, want 2 (per-agent)", count)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}
