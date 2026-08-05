// Package extract 提供首次提问、时间、工作目录等跨 Agent 共享提取逻辑。
package extract

import (
	"regexp"
	"strings"
)

// 需要从用户消息中剥离的内部/系统注入块前缀。
var injectedPrefixes = []string{
	"<system-reminder>",
	"<environment_context>",
	"<INSTRUCTIONS>",
	"<instructions>",
	"# AGENTS.md instructions",
	"# CLAUDE.md instructions",
}

// ansiEscapeRE 匹配 ANSI/终端控制序列。
var ansiEscapeRE = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[()][0-9A-Za-z]|\x1b[0-9]`)

// StripANSI 移除 ANSI 控制序列。
func StripANSI(s string) string {
	return ansiEscapeRE.ReplaceAllString(s, "")
}

// systemReminderRE 匹配 `<system-reminder>...</system-reminder>` 整块。
var systemReminderRE = regexp.MustCompile(`(?s)<system-reminder>.*?</system-reminder>`)

// instructionsBlockRE 匹配 `<INSTRUCTIONS>...</INSTRUCTIONS>` 整块。
var instructionsBlockRE = regexp.MustCompile(`(?s)<INSTRUCTIONS>.*?</INSTRUCTIONS>`)

// envContextBlockRE 匹配 `<environment_context>...</environment_context>` 整块。
var envContextBlockRE = regexp.MustCompile(`(?s)<environment_context>.*?</environment_context>`)

// agentsHeaderRE 匹配 "# AGENTS.md instructions ..." 单行标题。
var agentsHeaderRE = regexp.MustCompile(`(?m)^#?\s*(AGENTS\.md|CLAUDE\.md)\s+instructions.*$\n?`)

// StripInjectedContent 移除系统提醒、环境上下文、AGENTS 指令注入块与 ANSI 序列。
// 返回清理后的文本。保留用户原始表达。
func StripInjectedContent(s string) string {
	if s == "" {
		return s
	}
	out := s
	out = systemReminderRE.ReplaceAllString(out, "")
	out = instructionsBlockRE.ReplaceAllString(out, "")
	out = envContextBlockRE.ReplaceAllString(out, "")
	out = agentsHeaderRE.ReplaceAllString(out, "")
	out = ansiEscapeRE.ReplaceAllString(out, "")
	return strings.TrimSpace(out)
}

// IsInjectedBlock 判断一段文本是否整体为注入块（无真实用户内容）。
func IsInjectedBlock(s string) bool {
	t := strings.TrimSpace(s)
	for _, p := range injectedPrefixes {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	return false
}

// FirstNonInjected 从文本切片中取出第一条非注入、非空的文本。
func FirstNonInjected(blocks []string) (string, bool) {
	for _, b := range blocks {
		cleaned := StripInjectedContent(b)
		if IsInjectedBlock(cleaned) || cleaned == "" {
			continue
		}
		return cleaned, true
	}
	return "", false
}

// MergeTextBlocks 将多个文本块以换行合并，跳过注入块。
func MergeTextBlocks(blocks []string) string {
	var parts []string
	for _, b := range blocks {
		cleaned := StripInjectedContent(b)
		if cleaned == "" || IsInjectedBlock(cleaned) {
			continue
		}
		parts = append(parts, cleaned)
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}
