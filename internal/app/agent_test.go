package app

import (
	"testing"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/adapters/claude"
	"github.com/talea/talea/internal/adapters/codex"
	"github.com/talea/talea/internal/adapters/opencode"
	"github.com/talea/talea/internal/config"
	"github.com/talea/talea/internal/model"
)

func testApp() *App {
	reg := adapters.NewRegistry()
	_ = reg.Register(claude.New())
	_ = reg.Register(codex.New())
	_ = reg.Register(opencode.New())
	return &App{Registry: reg, Config: config.Default()}
}

func TestResolveAgent(t *testing.T) {
	a := testApp()
	cases := []struct {
		name string
		in   string
		want model.AgentID
	}{
		{"canonical claude", "claude-code", model.AgentClaudeCode},
		{"no dash claude", "claudecode", model.AgentClaudeCode},
		{"display claude", "Claude Code", model.AgentClaudeCode},
		{"prefix claude", "claude", model.AgentClaudeCode},
		{"canonical codex", "codex-cli", model.AgentCodexCLI},
		{"short codex", "codex", model.AgentCodexCLI},
		{"display codex", "Codex CLI", model.AgentCodexCLI},
		{"opencode", "opencode", model.AgentOpenCode},
		{"opencode upper", "OpenCode", model.AgentOpenCode},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := a.ResolveAgent(c.in)
			if err != nil {
				t.Fatalf("ResolveAgent(%q): %v", c.in, err)
			}
			if got != c.want {
				t.Fatalf("ResolveAgent(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestResolveAgentUnknown(t *testing.T) {
	a := testApp()
	for _, in := range []string{"foo", "generic"} {
		if _, err := a.ResolveAgent(in); err != ErrAgentUnknown {
			t.Fatalf("ResolveAgent(%q): got %v, want ErrAgentUnknown", in, err)
		}
	}
}

func TestResolveAgentEmpty(t *testing.T) {
	a := testApp()
	if _, err := a.ResolveAgent(""); err != ErrAgentRequired {
		t.Fatal("empty agent should return ErrAgentRequired")
	}
}

func TestResolveAgentPrefixAmbiguous(t *testing.T) {
	a := testApp()
	// "c" 同时是 claude-code 与 codex-cli 的前缀
	if _, err := a.ResolveAgent("c"); err != ErrAgentAmbiguous {
		t.Fatalf("ambiguous prefix: got %v, want ErrAgentAmbiguous", err)
	}
}
