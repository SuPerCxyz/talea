//go:build !windows

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
