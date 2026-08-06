package i18n

import "testing"

func TestDetectZh(t *testing.T) {
	cases := []struct {
		env  string
		want Lang
	}{
		{"zh_CN.UTF-8", LangZh},
		{"zh_TW.UTF-8", LangZh},
		{"zh", LangZh},
		{"en_US.UTF-8", LangEn},
		{"C", LangEn},
		{"POSIX", LangEn},
		{"ja_JP.UTF-8", LangEn},
		{"", LangEn},
	}
	for _, c := range cases {
		t.Setenv("LC_ALL", c.env)
		if got := Detect(); got != c.want {
			t.Fatalf("Detect(LC_ALL=%q)=%v want %v", c.env, got, c.want)
		}
	}
}

func TestDetectPriority(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LANG", "zh_CN.UTF-8")
	if got := Detect(); got != LangZh {
		t.Fatalf("Detect via LANG=%v want zh", got)
	}
}

func TestTr(t *testing.T) {
	Set(LangEn)
	if got := Tr("Start", "开始"); got != "Start" {
		t.Fatalf("Tr(en)=%q", got)
	}
	Set(LangZh)
	if got := Tr("Start", "开始"); got != "开始" {
		t.Fatalf("Tr(zh)=%q", got)
	}
	Set(LangEn)
}

func TestTrf(t *testing.T) {
	Set(LangZh)
	if got := Trf("added %d", "新增 %d", 5); got != "新增 5" {
		t.Fatalf("Trf(zh)=%q", got)
	}
	Set(LangEn)
	if got := Trf("added %d", "新增 %d", 5); got != "added 5" {
		t.Fatalf("Trf(en)=%q", got)
	}
}
