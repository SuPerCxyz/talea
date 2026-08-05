package extract

import "testing"

func TestStripInjectedContent(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"system-reminder block", "<system-reminder>请忽略</system-reminder>\n真实提问", "真实提问"},
		{"instructions block", "# AGENTS.md instructions\n<INSTRUCTIONS>规则</INSTRUCTIONS>\n真实提问", "真实提问"},
		{"env context block", "<environment_context>\n<cwd>/x</cwd>\n</environment_context>\n提问", "提问"},
		{"plain text unchanged", "正常提问内容", "正常提问内容"},
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := StripInjectedContent(c.in); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestIsInjectedBlock(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"instructions header", "# AGENTS.md instructions for /x", true},
		{"env context", "<environment_context>...", true},
		{"real question", "请分析问题原因", false},
		{"empty", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsInjectedBlock(c.in); got != c.want {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestFirstNonInjected(t *testing.T) {
	blocks := []string{
		"# AGENTS.md instructions\n<INSTRUCTIONS>rules</INSTRUCTIONS>",
		"<environment_context></environment_context>",
		"第一条真实提问",
	}
	got, ok := FirstNonInjected(blocks)
	if !ok {
		t.Fatal("expected ok")
	}
	if got != "第一条真实提问" {
		t.Fatalf("got %q", got)
	}
}

func TestMergeTextBlocks(t *testing.T) {
	got := MergeTextBlocks([]string{
		"<INSTRUCTIONS>skip</INSTRUCTIONS>",
		"部分一",
		"部分二",
	})
	if got != "部分一\n部分二" {
		t.Fatalf("got %q", got)
	}
}

func TestJSONLLineReaderBadLine(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/bad.jsonl"
	writeFile(t, path, "{\"type\":\"user\"}\nnot-json\n{\"type\":\"assistant\"}\n")
	r, err := OpenJSONL(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	obj, ok, err := r.Next()
	if err != nil || !ok {
		t.Fatalf("first line: ok=%v err=%v", ok, err)
	}
	if obj["type"] != "user" {
		t.Fatalf("got %v", obj["type"])
	}
	if _, ok, err := r.Next(); err == nil || ok {
		t.Fatalf("bad line should error, got ok=%v err=%v", ok, err)
	}
	obj, ok, err = r.Next()
	if err != nil || !ok {
		t.Fatalf("third line: ok=%v err=%v", ok, err)
	}
	if obj["type"] != "assistant" {
		t.Fatalf("got %v", obj["type"])
	}
}
