// Package i18n 提供用户可见文案的语言选择（默认英文，终端为中文 locale 时中文）。
package i18n

import (
	"fmt"
	"os"
	"strings"
)

// Lang 语言。
type Lang int

const (
	// LangEn 英文（默认）。
	LangEn Lang = iota
	// LangZh 中文。
	LangZh
)

var current = LangEn

// Detect 从环境变量检测语言。优先 LC_ALL/LC_MESSAGES/LANG/LANGUAGE，
// 以 zh 开头返回中文，其余非 C 环境返回英文（默认）。
func Detect() Lang {
	for _, e := range []string{"LC_ALL", "LC_MESSAGES", "LANG", "LANGUAGE"} {
		v := strings.ToLower(strings.TrimSpace(os.Getenv(e)))
		if v == "" || v == "c" || v == "posix" {
			continue
		}
		if strings.HasPrefix(v, "zh") {
			return LangZh
		}
		return LangEn
	}
	return LangEn
}

// Set 设置全局语言（main 入口调用 Detect 后设置）。
func Set(l Lang) { current = l }

// IsZh 当前是否为中文。
func IsZh() bool { return current == LangZh }

// Tr 按当前语言返回文案：中文返回 zh，否则返回 en（英文为默认源文案）。
func Tr(en, zh string) string {
	if current == LangZh {
		return zh
	}
	return en
}

// Trf 按当前语言返回带格式化的文案，en/zh 必须使用一致的格式化占位符。
func Trf(en, zh string, args ...any) string {
	if current == LangZh {
		return fmt.Sprintf(zh, args...)
	}
	return fmt.Sprintf(en, args...)
}
