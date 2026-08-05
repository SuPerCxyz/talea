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

// TestMaliciousSessionIDPath 验证恶意 Session ID 与 cwd 被路径安全检查拦截。
func TestMaliciousSessionIDPath(t *testing.T) {
	// 恶意 session id 通过路径安全检查（它们不进入 shell）
	// 但参数数组执行保证不解释。这里验证 IsPathSafe 拦截 shell 元字符路径。
	maliciousPaths := []string{
		"/tmp/$(rm -rf /)",
		"/tmp/`id`",
		"/tmp/x; reboot",
		"/tmp/a && rm -rf .",
		"/tmp/p|nc -e /bin/sh",
	}
	for _, p := range maliciousPaths {
		ok, reason := IsPathSafe(p)
		if ok {
			t.Fatalf("expected unsafe path %q, got safe", p)
		}
		if reason == "" {
			t.Fatalf("expected reason for %q", p)
		}
	}
}

// TestAnsiInjection 验证 ANSI 注入序列被剥离。
func TestAnsiInjection(t *testing.T) {
	payloads := []string{
		"\x1b[31mred\x1b[0m",
		"\x1b]0;title\x07content",
		"\x1b[2J\x1b[H",
	}
	for _, p := range payloads {
		cleaned := StripANSI(p)
		if containsEscape(cleaned) {
			t.Fatalf("ANSI escape survived: %q -> %q", p, cleaned)
		}
	}
}

func containsEscape(s string) bool {
	for _, r := range s {
		if r == 0x1b {
			return true
		}
	}
	return false
}
