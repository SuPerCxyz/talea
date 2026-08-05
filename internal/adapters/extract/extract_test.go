package extract

import (
	"os"
	"testing"
)

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

func TestLastCompleteLineOffset(t *testing.T) {
	dir := t.TempDir()

	// 完整换行结尾：offset == size
	path1 := dir + "/complete.jsonl"
	writeFile(t, path1, "{\"a\":1}\n{\"b\":2}\n")
	size1 := fileSize(t, path1)
	off1, err := LastCompleteLineOffset(path1)
	if err != nil {
		t.Fatal(err)
	}
	if off1 != size1 {
		t.Fatalf("complete: off=%d size=%d", off1, size1)
	}

	// 不完整尾行：offset 指向最后一个完整换行之后
	path2 := dir + "/tail.jsonl"
	content2 := "{\"a\":1}\n{\"b\":2}\n{\"c\":3}"
	writeFile(t, path2, content2)
	// 最后一个完整换行是第二个 \n（index 15），offset = 16
	lastNL := indexOfNl(content2, 2)
	off2, err := LastCompleteLineOffset(path2)
	if err != nil {
		t.Fatal(err)
	}
	if off2 != int64(lastNL+1) {
		t.Fatalf("tail: off=%d want=%d", off2, lastNL+1)
	}

	// 空文件
	path3 := dir + "/empty.jsonl"
	writeFile(t, path3, "")
	off3, _ := LastCompleteLineOffset(path3)
	if off3 != 0 {
		t.Fatalf("empty: off=%d", off3)
	}
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Size()
}

func indexOfNl(s string, nth int) int {
	count := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			count++
			if count == nth {
				return i
			}
		}
	}
	return -1
}
