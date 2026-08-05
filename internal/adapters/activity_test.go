package adapters

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/talea/talea/internal/model"
)

func TestProcessDetectorInactive(t *testing.T) {
	ctx := context.Background()
	d := ProcessActivityDetector{Executable: "definitely-not-running-exe"}
	s := model.Session{
		SourcePath:  "/nonexistent/file.jsonl",
		SourceMtime: 0,
	}
	state, err := d.DetectActivity(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	if state != model.ActivityInactive {
		t.Fatalf("got %q, want inactive", state)
	}
}

func TestProcessDetectorPossiblyActive(t *testing.T) {
	ctx := context.Background()
	d := ProcessActivityDetector{Executable: "definitely-not-running-exe"}
	// 最近 30 秒更新的文件 -> possibly_active
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	s := model.Session{SourcePath: path, SourceMtime: info.ModTime().Unix()}
	state, err := d.DetectActivity(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	if state != model.ActivityPossiblyActive {
		t.Fatalf("got %q, want possibly_active", state)
	}
}

func TestLastSlash(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/usr/bin/claude", "claude"},
		{"claude", "claude"},
		{"/a/b/c", "c"},
	}
	for _, c := range cases {
		base := c.in
		if idx := lastSlash(c.in); idx >= 0 {
			base = c.in[idx+1:]
		}
		if base != c.want {
			t.Fatalf("got %q want %q", base, c.want)
		}
	}
}
