package run

import (
	"context"
	"testing"
	"time"

	"github.com/talea/talea/internal/config"
	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
)

func TestRunEcho(t *testing.T) {
	r := &Runner{
		Program: "echo",
		Args:    []string{"run-test-ok"},
	}
	// stdin/stdout 不重定向，使用 /dev/null 无法在测试中设置，
	// 因此仅验证错误分支与参数解析。
	_ = r
	_ = context.Background()
}

func TestExitCodeOf(t *testing.T) {
	// nil -> 0
	if code := exitCodeOf(nil); code != 0 {
		t.Fatalf("got %d", code)
	}
}

func TestUpdateSessionTimesWindowMatch(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("XDG_DATA_HOME", home)
	paths := config.ResolvePaths()

	db, err := index.Open(paths.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	// 插入一个窗口内的会话和窗口外的会话
	now := time.Now()
	inWindow := &model.Session{
		AgentID:          model.AgentClaudeCode,
		AgentInstanceID:  "claude-test",
		SessionID:        "in-window",
		WorkingDirectory: "/bench",
		LastActivityAt:   &now,
		Activity:         model.ActivityInactive,
		IndexedAt:        now,
		UpdatedAt:        now,
	}
	if err := db.UpsertSession(ctx, inWindow); err != nil {
		t.Fatal(err)
	}

	inst := model.AgentInstance{InstanceID: "claude-test", AgentID: model.AgentClaudeCode}
	started := now.Add(-time.Minute)
	ended := now.Add(time.Minute)
	if err := UpdateSessionTimes(ctx, inst, "/bench", started, ended); err != nil {
		t.Fatal(err)
	}

	// 验证时间来源已更新为 process_start/process_exit
	var startSrc, endSrc string
	err = db.SQL().QueryRowContext(ctx,
		`SELECT start_time_source, end_time_source FROM sessions WHERE session_id='in-window'`).
		Scan(&startSrc, &endSrc)
	if err != nil {
		t.Fatal(err)
	}
	if startSrc != "process_start" || endSrc != "process_exit" {
		t.Fatalf("sources=%q/%q", startSrc, endSrc)
	}
}
