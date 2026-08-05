package plugin

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func buildPlugin(t *testing.T, out string) {
	t.Helper()
	// 测试在 internal/plugin 目录运行，仓库根为 ../../../
	src := filepath.Join("..", "..", "scripts", "talea-adapter-example")
	cmd := exec.Command("go", "build", "-o", out, src)
	outBytes, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build plugin: %v\n%s", err, outBytes)
	}
	_ = os.Chmod(out, 0o755)
}
