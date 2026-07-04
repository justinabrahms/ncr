package main

import (
	"math"
	"strings"
	"testing"
)

func TestUsageCost(t *testing.T) {
	u := Usage{InputTokens: 58000, OutputTokens: 11000, CacheCreationInputTokens: 1100, CacheReadInputTokens: 200}
	// sonnet: in $3/M, out $15/M; cache write ×1.25, read ×0.1
	want := (58000*3 + 1100*1.25*3 + 200*0.1*3 + 11000*15) / 1e6
	if got := u.cost("claude-sonnet-4-6"); math.Abs(got-want) > 1e-9 {
		t.Fatalf("sonnet cost = %v, want %v", got, want)
	}
	// opus is pricier than sonnet for the same usage
	if u.cost("claude-opus-4-8") <= u.cost("claude-sonnet-4-6") {
		t.Fatal("opus should cost more than sonnet")
	}
	// haiku is cheaper
	if u.cost("claude-haiku-4-5") >= u.cost("claude-sonnet-4-6") {
		t.Fatal("haiku should cost less than sonnet")
	}
}

func TestUsageSummary(t *testing.T) {
	s := Usage{InputTokens: 58000, OutputTokens: 11200, CacheCreationInputTokens: 1100}.summary("claude-sonnet-4-6")
	for _, want := range []string{"58.0k in", "11.2k out", "cache 1.1k write", "$", "claude-sonnet-4-6"} {
		if !strings.Contains(s, want) {
			t.Fatalf("summary %q missing %q", s, want)
		}
	}
	// no cache segment when there's no caching, small counts render exact
	s2 := Usage{InputTokens: 500, OutputTokens: 80}.summary("m")
	if strings.Contains(s2, "cache") || !strings.Contains(s2, "500 in") {
		t.Fatalf("unexpected summary: %q", s2)
	}
}
