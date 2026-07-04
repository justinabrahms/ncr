package main

import (
	"fmt"
	"strconv"
	"strings"
)

// Estimated cost of the plan step. Prices are approximate list prices in USD per
// million tokens — easy to update here. Cache writes bill at 1.25x the input rate,
// cache reads at 0.1x. See TODO.md.

type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

type modelPrice struct{ in, out float64 } // USD per million tokens

func priceFor(model string) modelPrice {
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "opus"):
		return modelPrice{15, 75}
	case strings.Contains(m, "haiku"):
		return modelPrice{1, 5}
	default: // sonnet family
		return modelPrice{3, 15}
	}
}

func (u Usage) cost(model string) float64 {
	p := priceFor(model)
	in := float64(u.InputTokens) +
		float64(u.CacheCreationInputTokens)*1.25 +
		float64(u.CacheReadInputTokens)*0.1
	return in*p.in/1e6 + float64(u.OutputTokens)*p.out/1e6
}

func fmtTokens(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return strconv.Itoa(n)
}

// summary is the one-line cost report, e.g.
// "57.9k in / 11.2k out (cache 1.1k write / 0 read) — ~$0.34 (claude-sonnet-4-6)".
func (u Usage) summary(model string) string {
	cache := ""
	if u.CacheCreationInputTokens > 0 || u.CacheReadInputTokens > 0 {
		cache = fmt.Sprintf(" (cache %s write / %s read)",
			fmtTokens(u.CacheCreationInputTokens), fmtTokens(u.CacheReadInputTokens))
	}
	return fmt.Sprintf("%s in / %s out%s — ~$%.2f (%s)",
		fmtTokens(u.InputTokens), fmtTokens(u.OutputTokens), cache, u.cost(model), model)
}
