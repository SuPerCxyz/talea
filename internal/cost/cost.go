// Package cost 提供可选的费用估算。
package cost

import (
	"time"

	"github.com/talea/talea/internal/config"
	"github.com/talea/talea/internal/model"
)

// Estimate 基于价格表估算会话费用。
// 返回微货币单位（整数）。无法确认模型版本或价格缺失时返回 (nil, false)。
func Estimate(u *model.TokenUsage, cfg config.Pricing) (*int64, string, time.Time, bool) {
	if u == nil {
		return nil, "", time.Time{}, false
	}
	modelName := modelNameOf(u)
	p, ok := cfg.CustomModel[modelName]
	if !ok {
		return nil, "", time.Time{}, false
	}
	if p.InputPerMillion <= 0 && p.OutputPerMillion <= 0 {
		return nil, "", time.Time{}, false
	}

	var micros int64
	if u.InputTokens != nil {
		micros += roundMicros(float64(*u.InputTokens) * p.InputPerMillion / 1_000_000)
	}
	if u.OutputTokens != nil {
		micros += roundMicros(float64(*u.OutputTokens) * p.OutputPerMillion / 1_000_000)
	}
	if u.CacheReadTokens != nil && p.CacheReadPerMillion > 0 {
		micros += roundMicros(float64(*u.CacheReadTokens) * p.CacheReadPerMillion / 1_000_000)
	}
	if u.CacheWriteTokens != nil && p.CacheWritePerMillion > 0 {
		micros += roundMicros(float64(*u.CacheWriteTokens) * p.CacheWritePerMillion / 1_000_000)
	}
	if micros <= 0 {
		return nil, "", time.Time{}, false
	}
	now := time.Now()
	return &micros, p.Currency, now, true
}

func modelNameOf(u *model.TokenUsage) string {
	if m, ok := u.RawFields["model"].(string); ok && m != "" {
		return m
	}
	return "custom-model"
}

func roundMicros(v float64) int64 {
	// 四舍五入到微单位
	return int64(v*1_000_000 + 0.5)
}

// Format 将微货币单位格式化为可读金额。
func Format(micros int64, currency string) string {
	if micros == 0 {
		return "0"
	}
	whole := micros / 1_000_000
	frac := micros % 1_000_000
	symbol := symbolOf(currency)
	return symbol + formatInt(whole) + "." + pad2(frac/10000)
}

func symbolOf(currency string) string {
	switch currency {
	case "USD":
		return "$"
	case "CNY":
		return "¥"
	case "EUR":
		return "€"
	case "JPY":
		return "¥"
	default:
		return currency + " "
	}
}

func formatInt(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [24]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}

func pad2(n int64) string {
	s := formatInt(n)
	for len(s) < 2 {
		s = "0" + s
	}
	return s
}
