package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/talea/talea/internal/config"
	"github.com/talea/talea/internal/status"
)

func runHook(t *testing.T, input string) error {
	t.Helper()
	cmd := newHookCmd()
	cmd.SetIn(strings.NewReader(input))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(nil)
	return cmd.ExecuteContext(context.Background())
}

func TestHookMalformedInputExitsZeroWithoutFile(t *testing.T) {
	paths := isolateTaleaEnv(t)
	if err := runHook(t, "not json at all"); err != nil {
		t.Fatalf("malformed input must exit 0: %v", err)
	}
	eventPath := filepath.Join(paths.DataDir, status.EventFileName)
	if _, err := os.Stat(eventPath); !os.IsNotExist(err) {
		t.Fatalf("malformed input 不应写事件文件: %v", err)
	}
}

func TestHookMissingSessionExitsZero(t *testing.T) {
	isolateTaleaEnv(t)
	if err := runHook(t, `{"hook_event_name":"Stop"}`); err != nil {
		t.Fatalf("missing session must exit 0: %v", err)
	}
}

func TestHookClaudeAuthSuccessDoesNotWriteEvent(t *testing.T) {
	paths := isolateTaleaEnv(t)
	input := `{"session_id":"s1","hook_event_name":"Notification","notification_type":"auth_success"}`
	if err := runHook(t, input); err != nil {
		t.Fatalf("auth_success 也必须退出 0: %v", err)
	}
	eventPath := filepath.Join(paths.DataDir, status.EventFileName)
	if _, err := os.Stat(eventPath); !os.IsNotExist(err) {
		t.Fatalf("auth_success 不应写事件文件: %v", err)
	}
}

func TestHookClaudePermissionWritesEvent(t *testing.T) {
	paths := isolateTaleaEnv(t)
	input := `{"session_id":"s1","hook_event_name":"Notification","notification_type":"permission_prompt"}`
	if err := runHook(t, input); err != nil {
		t.Fatalf("hook 应成功: %v", err)
	}
	idx, err := status.LoadLatestEvents(filepath.Join(paths.DataDir, status.EventFileName))
	if err != nil {
		t.Fatal(err)
	}
	event, ok := idx.Latest("claude-code", "", "s1")
	if !ok || event.State != "waiting_input" {
		t.Fatalf("事件=%+v ok=%v", event, ok)
	}
}

func TestHookWriteFailureExitsZero(t *testing.T) {
	home := t.TempDir()
	blocked := filepath.Join(home, "blocked")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", blocked)
	t.Setenv("PATH", "")
	// XDG_DATA_HOME 是普通文件，创建 <blocked>/talea 必然失败。
	input := `{"agent":"opencode","session_id":"s1","state":"completed"}`
	if err := runHook(t, input); err != nil {
		t.Fatalf("写入失败必须退出 0: %v", err)
	}
}

func TestHookRejectsInvalidStateFlagOverride(t *testing.T) {
	paths := isolateTaleaEnv(t)
	cmd := newHookCmd()
	cmd.SetIn(strings.NewReader(`{"agent":"opencode","session_id":"s1","state":"completed"}`))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--state", "bogus"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("无效 state 也必须退出 0: %v", err)
	}
	eventPath := filepath.Join(paths.DataDir, status.EventFileName)
	if _, err := os.Stat(eventPath); !os.IsNotExist(err) {
		t.Fatalf("无效 state 不应写事件: %v", err)
	}
}

func TestHookRawOpenCodeEventWritesEvent(t *testing.T) {
	paths := isolateTaleaEnv(t)
	input := `{"type":"session.idle","properties":{"sessionID":"ses_raw"}}`
	if err := runHook(t, input); err != nil {
		t.Fatalf("原始 OpenCode 事件应成功: %v", err)
	}
	idx, err := status.LoadLatestEvents(filepath.Join(paths.DataDir, status.EventFileName))
	if err != nil {
		t.Fatal(err)
	}
	event, ok := idx.Latest("opencode", "", "ses_raw")
	if !ok || event.State != "completed" {
		t.Fatalf("事件=%+v ok=%v", event, ok)
	}
}

func TestHookFlagsOnlyWritesEvent(t *testing.T) {
	paths := isolateTaleaEnv(t)
	cmd := newHookCmd()
	cmd.SetIn(strings.NewReader(""))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--agent", "opencode", "--session", "ses_1", "--state", "completed"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("flags-only hook 应成功: %v", err)
	}
	idx, err := status.LoadLatestEvents(filepath.Join(paths.DataDir, status.EventFileName))
	if err != nil {
		t.Fatal(err)
	}
	if event, ok := idx.Latest("opencode", "", "ses_1"); !ok || event.State != "completed" {
		t.Fatalf("事件=%+v ok=%v", event, ok)
	}
}

func TestHookFilePermissions(t *testing.T) {
	paths := isolateTaleaEnv(t)
	if err := runHook(t, `{"session_id":"s1","hook_event_name":"Stop"}`); err != nil {
		t.Fatal(err)
	}
	eventPath := filepath.Join(paths.DataDir, status.EventFileName)
	fileInfo, err := os.Stat(eventPath)
	if err != nil {
		t.Fatal(err)
	}
	if fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("事件文件权限=%s", fileInfo.Mode().Perm())
	}
	dirInfo, err := os.Stat(config.ResolvePaths().DataDir)
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("数据目录权限=%s", dirInfo.Mode().Perm())
	}
}
