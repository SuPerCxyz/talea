package cli

import (
	"os"
	"testing"
)

func TestMapChoice(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"1", "mapped"},
		{"2", "current"},
		{"3", "view"},
		{"4", "copy"},
		{"5", "cancel"},
		{"x", "cancel"},
		{"", "cancel"},
	}
	for _, c := range cases {
		if got := mapChoice(c.in); got != c.want {
			t.Fatalf("choice %q -> %q, want %q", c.in, got, c.want)
		}
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
