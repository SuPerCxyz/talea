package generic

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/model"
)

func TestParseMetadataSimple(t *testing.T) {
	ctx := context.Background()
	a := New()
	path := filepath.Join("..", "..", "..", "testdata", "claude", "simple.jsonl")
	src := adapters.SessionSource{Path: path}
	inst := model.AgentInstance{InstanceID: "g", AgentID: "generic-jsonl"}
	s, err := a.ParseMetadata(ctx, inst, src)
	if err != nil {
		t.Fatal(err)
	}
	if s.FirstQuestion != "请设计一个 VSCode 插件，支持 Git 历史可视化。" {
		t.Fatalf("first question: %q", s.FirstQuestion)
	}
	if s.WorkingDirectory != "/home/alice/code/pentimento" {
		t.Fatalf("cwd: %q", s.WorkingDirectory)
	}
	if s.SessionID != "11111111-aaaa-4f46-99eb-32346319749e" {
		t.Fatalf("session id: %q", s.SessionID)
	}
}

func TestSessionIDOf(t *testing.T) {
	if got := sessionIDOf(adapters.SessionSource{Path: "/tmp/session-abc.jsonl"}); got != "session-abc" {
		t.Fatalf("got %q", got)
	}
}

func TestDiscoverEmptyDir(t *testing.T) {
	a := New()
	inst := model.AgentInstance{DataDirectory: t.TempDir()}
	srcs, err := a.Discover(context.Background(), inst)
	if err != nil {
		t.Fatal(err)
	}
	if len(srcs) != 0 {
		t.Fatalf("expected empty, got %d", len(srcs))
	}
}
