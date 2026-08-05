package plugin

import (
	"context"
	"path/filepath"
	"testing"
)

func TestPluginProtocolEndToEnd(t *testing.T) {
	// 构建示例插件
	dir := t.TempDir()
	bin := filepath.Join(dir, "talea-adapter-example")
	buildPlugin(t, bin)

	client := NewClient(bin)
	if err := client.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	info, err := client.Info()
	if err != nil {
		t.Fatal(err)
	}
	if info.ID != "example-plugin" {
		t.Fatalf("id=%q", info.ID)
	}

	insts, err := client.Detect()
	if err != nil {
		t.Fatal(err)
	}
	if len(insts) != 0 {
		t.Fatalf("insts=%d", len(insts))
	}
}
