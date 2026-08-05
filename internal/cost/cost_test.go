package cost

import (
	"testing"

	"github.com/talea/talea/internal/config"
	"github.com/talea/talea/internal/model"
)

func TestEstimate(t *testing.T) {
	in := int64(1_000_000)
	out := int64(500_000)
	u := &model.TokenUsage{
		InputTokens:  &in,
		OutputTokens: &out,
		RawFields:    map[string]any{"model": "claude-sonnet"},
	}
	cfg := config.Pricing{
		CustomModel: map[string]config.ModelPrice{
			"claude-sonnet": {
				Currency:            "USD",
				InputPerMillion:     3.0,
				OutputPerMillion:    15.0,
				CacheReadPerMillion: 0.3,
			},
		},
	}
	micros, currency, _, ok := Estimate(u, cfg)
	if !ok {
		t.Fatal("expected estimate")
	}
	// 输入 1M * 3 = 3USD, 输出 0.5M * 15 = 7.5USD => 10.5 USD = 10,500,000 micros
	if *micros != 10_500_000 {
		t.Fatalf("micros=%d", *micros)
	}
	if currency != "USD" {
		t.Fatalf("currency=%s", currency)
	}
}

func TestEstimateMissingPrice(t *testing.T) {
	in := int64(1000)
	u := &model.TokenUsage{InputTokens: &in, RawFields: map[string]any{"model": "unknown-model"}}
	cfg := config.Pricing{CustomModel: map[string]config.ModelPrice{}}
	if _, _, _, ok := Estimate(u, cfg); ok {
		t.Fatal("expected no estimate for missing price")
	}
}

func TestEstimateCache(t *testing.T) {
	in := int64(1_000_000)
	cache := int64(1_000_000)
	u := &model.TokenUsage{InputTokens: &in, CacheReadTokens: &cache,
		RawFields: map[string]any{"model": "m"}}
	cfg := config.Pricing{CustomModel: map[string]config.ModelPrice{
		"m": {Currency: "USD", InputPerMillion: 3.0, CacheReadPerMillion: 0.3},
	}}
	micros, _, _, ok := Estimate(u, cfg)
	if !ok {
		t.Fatal("expected estimate")
	}
	// 3 + 0.3 = 3.3 USD
	if *micros != 3_300_000 {
		t.Fatalf("micros=%d", *micros)
	}
}

func TestFormat(t *testing.T) {
	cases := []struct {
		micros   int64
		currency string
		want     string
	}{
		{10_500_000, "USD", "$10.50"},
		{3_000_000, "USD", "$3.00"},
		{5_000_000, "CNY", "¥5.00"},
		{0, "USD", "0"},
	}
	for _, c := range cases {
		if got := Format(c.micros, c.currency); got != c.want {
			t.Fatalf("got %q want %q", got, c.want)
		}
	}
}
