package cli

import (
	"os"
	"testing"

	"github.com/talea/talea/internal/model"
)

func TestHandleMissingDirNonTTY(t *testing.T) {
	// 非 TTY 时默认 /tmp
	sess := &model.Session{SessionID: "abc", FirstQuestion: "q"}
	dir, action, err := handleMissingDir(sess, "/gone/path", nil)
	if err != nil {
		t.Fatal(err)
	}
	if action != "mapped" {
		t.Fatalf("action: %q", action)
	}
	if dir != "/tmp" {
		t.Fatalf("dir: %q", dir)
	}
}

func TestIsTTY(t *testing.T) {
	if isTTY(nil) {
		t.Fatal("nil file should not be tty")
	}
	// 普通文件不是 TTY
	f, err := os.CreateTemp(t.TempDir(), "x")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if isTTY(f) {
		t.Fatal("regular file should not be tty")
	}
}
