// Package security 提供脱敏、ANSI 清理与路径安全工具。
package security

import (
	"regexp"
	"strings"
)

// ANSI 控制序列。
var (
	ansiRE     = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\x1b\][^\x07]*(\x07|\x1b\\)|\x1b[()][0-9A-Za-z]`)
	csiRE      = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	oscRE      = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`)
	otherEscRE = regexp.MustCompile(`\x1b[()][0-9A-Za-z]`)
	controlRE  = regexp.MustCompile(`[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]`)
)

// commonSecrets 常见敏感信息正则。
var (
	apiKeyRE    = regexp.MustCompile(`(?i)(sk-|pk-|rk-|ak-)[A-Za-z0-9_\-\.]{8,}`)
	keyEqRE     = regexp.MustCompile(`(?i)(api[_-]?key|secret|password|passwd|token)\s*[:=]\s*[A-Za-z0-9_\-\.]{8,}`)
	sshKeyRE    = regexp.MustCompile(`-----BEGIN (RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----`)
	bearerRE    = regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9_\-\.]{8,}`)
	basicAuthRE = regexp.MustCompile(`(?i)(https?://)[^:/@\s]+:[^@\s]+@`)
)

// StripANSI 移除 ANSI 控制序列与终端控制字符。
func StripANSI(s string) string {
	out := s
	out = oscRE.ReplaceAllString(out, "")
	out = csiRE.ReplaceAllString(out, "")
	out = otherEscRE.ReplaceAllString(out, "")
	out = ansiRE.ReplaceAllString(out, "")
	out = controlRE.ReplaceAllString(out, "")
	return out
}

// RedactSecrets 将常见敏感信息替换为掩码。
func RedactSecrets(s string) string {
	out := s
	out = sshKeyRE.ReplaceAllString(out, "[PRIVATE KEY REDACTED]")
	out = bearerRE.ReplaceAllString(out, "Bearer [REDACTED]")
	out = basicAuthRE.ReplaceAllString(out, "${1}[REDACTED]@")
	out = apiKeyRE.ReplaceAllString(out, "$1[REDACTED]")
	out = keyEqRE.ReplaceAllString(out, "${1}=[REDACTED]")
	return out
}

// IsPathSafe 检查路径中是否含危险 shell 元字符（用于恢复/执行前的防御性检查）。
// 返回 (是否安全, 原因)。参数数组执行本身已防注入，此函数用于多一层防御。
func IsPathSafe(p string) (bool, string) {
	if p == "" {
		return true, ""
	}
	dangerous := []string{";", "&&", "||", "`", "$(", "|"}
	for _, d := range dangerous {
		if strings.Contains(p, d) {
			return false, "路径包含 shell 元字符: " + d
		}
	}
	return true, ""
}
