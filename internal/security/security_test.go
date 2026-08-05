package security

import "testing"

func TestStripANSI(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"color codes", "\x1b[31mred\x1b[0m", "red"},
		{"bold", "\x1b[1mtext", "text"},
		{"osc title", "\x1b]0;title\x07content", "content"},
		{"control chars", "a\x00b\x1fc", "abc"},
		{"plain", "普通文本", "普通文本"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := StripANSI(c.in); got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}
}

func TestRedactSecrets(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"api key", "sk-abcdefghijklmnop", "sk-[REDACTED]"},
		{"password", "password=hunter2secret", "password=[REDACTED]"},
		{"private key", "-----BEGIN RSA PRIVATE KEY-----\nfoo", "[PRIVATE KEY REDACTED]\nfoo"},
		{"bearer", "Authorization: Bearer abcdefgh1234", "Authorization: Bearer [REDACTED]"},
		{"url auth", "https://user:pass123@example.com/x", "https://[REDACTED]@example.com/x"},
		{"no secret", "正常内容 123", "正常内容 123"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := RedactSecrets(c.in); got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}
}

func TestIsPathSafe(t *testing.T) {
	bad := []string{"/tmp/x;rm -rf /", "/tmp/y$(id)", "/a|b"}
	for _, p := range bad {
		ok, _ := IsPathSafe(p)
		if ok {
			t.Fatalf("expected unsafe: %q", p)
		}
	}
	good := []string{"/tmp/安全 目录", "/home/user/code/cinder", "/tmp/with'quote", `"/tmp/dir with space"`}
	for _, p := range good {
		ok, _ := IsPathSafe(p)
		if !ok {
			t.Fatalf("expected safe: %q", p)
		}
	}
}
