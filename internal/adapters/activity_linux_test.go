//go:build linux

package adapters

import "testing"

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

func TestProcessNameMatches(t *testing.T) {
	cases := []struct {
		basename, execName string
		want               bool
	}{
		{"opencode", "opencode", true},
		{"opencode.exe", "opencode", true},
		{"opencode", "opencode.exe", true},
		{"opencode.exe", "opencode.exe", true},
		{"claude.exe", "claude", true},
		{"claude.exe", "claude.exe", true},
		{"codex", "codex", true},
		{"codex", "codex.js", false},
		{"opencode", "claude", false},
	}
	for _, c := range cases {
		if got := processNameMatches(c.basename, c.execName); got != c.want {
			t.Fatalf("processNameMatches(%q, %q) = %v, want %v", c.basename, c.execName, got, c.want)
		}
	}
}
