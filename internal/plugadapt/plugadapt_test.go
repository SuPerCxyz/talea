package plugadapt

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/model"
)

func buildPlugin(t *testing.T, out string) {
	t.Helper()
	// 测试在 internal/plugadapt 目录运行，仓库根为 ../../../
	src := filepath.Join("..", "..", "scripts", "talea-adapter-example")
	cmd := exec.Command("go", "build", "-o", out, src)
	outBytes, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build plugin: %v\n%s", err, outBytes)
	}
	_ = os.Chmod(out, 0o755)
}

func TestAdapterWrapsPlugin(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "talea-adapter-example")
	buildPlugin(t, bin)

	ad, err := New(bin)
	if err != nil {
		t.Fatal(err)
	}
	defer ad.Close()
	if ad.Info().ID != "example-plugin" {
		t.Fatalf("id=%q", ad.Info().ID)
	}
	insts, err := ad.Detect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(insts) != 0 {
		t.Fatalf("insts=%d", len(insts))
	}
}

// TestPluginFullProtocol 验证 parse/messages/usage/timeline 端到端。
func TestPluginFullProtocol(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "talea-adapter-example")
	buildPlugin(t, bin)

	// 准备插件数据目录
	pluginDir := filepath.Join(dir, "sessions")
	os.MkdirAll(pluginDir, 0o755)
	t.Setenv("EXAMPLE_PLUGIN_DIR", pluginDir)
	// 写入一个 Claude 风格 JSONL
	content := `{"type":"user","timestamp":"2026-07-17T04:57:32.179Z","cwd":"/bench","sessionId":"plug-1","message":{"role":"user","content":"插件测试问题"}}` + "\n" +
		`{"type":"assistant","timestamp":"2026-07-17T04:58:00.000Z","cwd":"/bench","sessionId":"plug-1","message":{"role":"assistant","content":[{"type":"text","text":"回复"}],"usage":{"input_tokens":500,"output_tokens":50}}}` + "\n" +
		`{"type":"assistant","timestamp":"2026-07-17T04:59:00.000Z","cwd":"/bench","sessionId":"plug-1","message":{"role":"assistant","content":[{"type":"text","text":"回复2"}],"usage":{"input_tokens":900,"output_tokens":80}}}` + "\n"
	if err := os.WriteFile(filepath.Join(pluginDir, "plug-1.jsonl"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	ad, err := New(bin)
	if err != nil {
		t.Fatal(err)
	}
	defer ad.Close()

	// Discover 应发现 1 个会话
	srcs, err := ad.Discover(t.Context(), model.AgentInstance{})
	if err != nil {
		t.Fatal(err)
	}
	if len(srcs) != 1 {
		t.Fatalf("sources=%d", len(srcs))
	}

	// ParseMetadata 完整字段
	sess, err := ad.ParseMetadata(t.Context(), model.AgentInstance{InstanceID: "example-default"}, srcs[0])
	if err != nil {
		t.Fatal(err)
	}
	if sess.FirstQuestion == "" {
		t.Fatal("expected first question from plugin parse")
	}

	// LoadUsage：input 累计 900，output 增量 130
	u, err := ad.LoadUsage(t.Context(), *sess)
	if err != nil {
		t.Fatal(err)
	}
	if u.InputTokens == nil || *u.InputTokens != 900 {
		t.Fatalf("input=%v want 900", u.InputTokens)
	}
	if u.OutputTokens == nil || *u.OutputTokens != 130 {
		t.Fatalf("output=%v want 130", u.OutputTokens)
	}

	// IterateUsageEvents：2 个 request 事件
	it, err := ad.IterateUsageEvents(t.Context(), *sess)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for {
		_, ok, err := it.Next()
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
		count++
	}
	it.Close()
	if count != 2 {
		t.Fatalf("timeline events=%d, want 2", count)
	}

	// LoadMessages：3 条消息（1 user + 2 assistant）
	mit, err := ad.LoadMessages(t.Context(), *sess, adapters.MessageLoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	mcount := 0
	for {
		_, ok, err := mit.Next()
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
		mcount++
	}
	mit.Close()
	if mcount != 3 {
		t.Fatalf("messages=%d, want 3", mcount)
	}
}
