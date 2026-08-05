package watch

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/fsnotify/fsnotify"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/adapters/claude"
	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/config"
	"github.com/talea/talea/internal/index"
)

func TestIsIgnored(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"dotfile", "/tmp/.tmp", true},
		{"wal", "/tmp/x.db-wal", true},
		{"shm", "/tmp/x.db-shm", true},
		{"jsonl", "/tmp/session.jsonl", false},
		{"tmp", "/tmp/session.jsonl.tmp", true},
	}
	for _, c := range cases {
		if got := isIgnored(c.in); got != c.want {
			t.Fatalf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestWatchRecursive(t *testing.T) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Skipf("inotify 实例受限，跳过: %v", err)
	}
	defer w.Close()

	root := t.TempDir()
	sub := filepath.Join(root, "a", "b")
	os.MkdirAll(sub, 0o755)
	if err := watchRecursive(w, root, 0); err != nil {
		t.Skipf("inotify watch 受限，跳过: %v", err)
	}
	// 根目录及其子目录应被监听
	for _, d := range []string{root, filepath.Join(root, "a"), sub} {
		found := false
		for _, wd := range w.WatchList() {
			if wd == d {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("目录 %s 未被监听", d)
		}
	}
}

func TestDataDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", home+"/data")

	reg := adapters.NewRegistry()
	reg.Register(claude.New())
	cfg := config.Default()
	a := &app.App{Registry: reg, Config: cfg, Paths: config.ResolvePaths()}

	dirs := dataDirs(a)
	found := false
	for _, d := range dirs {
		if d == filepath.Join(home, ".claude") {
			found = true
		}
	}
	if !found {
		t.Fatalf("未找到 claude 数据目录: %v", dirs)
	}
}

func TestRunIndexNoPanic(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", home+"/data")

	reg := adapters.NewRegistry()
	reg.Register(claude.New())
	cfg := config.Default()
	a := &app.App{Registry: reg, Config: cfg, Paths: config.ResolvePaths()}

	db, err := index.Open(a.Paths.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	// 空环境索引不应 panic
	runIndex(ctx, a, db)
}
