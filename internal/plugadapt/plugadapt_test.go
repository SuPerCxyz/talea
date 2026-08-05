package plugadapt

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
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
