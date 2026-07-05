package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildPromptIsDeterministic(t *testing.T) {
	// The prompt hash is the plan cache key, so it must not depend on Go's
	// randomized map iteration order over the changed files.
	idx := buildIndex(sampleDiff(t))
	files := map[string]string{
		"a.go": "package a", "z.go": "package z", "m.go": "package m",
		"b.go": "package b", "q.go": "package q",
	}
	_, first := buildPrompt(idx, files, nil, Meta{Title: "t"})
	for i := 0; i < 20; i++ {
		if _, u := buildPrompt(idx, files, nil, Meta{Title: "t"}); u != first {
			t.Fatal("buildPrompt user string is not stable across runs")
		}
	}
}

func TestExtractJSONSkipsProseSnippet(t *testing.T) {
	// The failure that motivated tool use: prose containing an incidental
	// `{ code snippet }` (invalid JSON) before the real object.
	in := "Looking at the diff I see `{ anyEnabled = true }` in the handler.\n" +
		"Here is the plan:\n{\"overview\":\"x\",\"chapters\":[]}"
	b, err := extractJSON(in)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("extracted invalid JSON: %s", b)
	}
	if _, ok := m["chapters"]; !ok {
		t.Fatalf("extracted wrong object: %s", b)
	}
}

func TestExtractJSONPrefersFencedBlock(t *testing.T) {
	in := "prose {not: json}\n```json\n{\"a\": {\"b\": 1}}\n```\ntrailing {x}"
	b, err := extractJSON(in)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"a": {"b": 1}}` {
		t.Fatalf("got %s", b)
	}
}

func TestExtractJSONBraceInString(t *testing.T) {
	b, err := extractJSON(`{"s": "has } brace"}`)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]string
	if json.Unmarshal(b, &m); m["s"] != "has } brace" {
		t.Fatalf("got %s", b)
	}
}

func TestParseModelResponseToolUse(t *testing.T) {
	body := []byte(`{"stop_reason":"tool_use","usage":{"input_tokens":500,"output_tokens":80},"content":[
		{"type":"text","text":"here you go"},
		{"type":"tool_use","name":"submit_reading_plan","input":{"overview":"o","chapters":[]}}
	]}`)
	b, usage, err := parseModelResponse(body, 1000)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil || m["overview"] != "o" {
		t.Fatalf("tool input not returned cleanly: %s (%v)", b, err)
	}
	if usage.InputTokens != 500 || usage.OutputTokens != 80 {
		t.Fatalf("usage not parsed: %+v", usage)
	}
}

func TestParseModelResponseMaxTokens(t *testing.T) {
	body := []byte(`{"stop_reason":"max_tokens","content":[{"type":"tool_use","input":{"chapters":[]}}]}`)
	if _, _, err := parseModelResponse(body, 100); err == nil {
		t.Fatal("expected truncation error on max_tokens")
	}
}

func TestParseModelResponseTextFallback(t *testing.T) {
	body := []byte(`{"stop_reason":"end_turn","content":[{"type":"text","text":"plan:\n{\"chapters\":[]}"}]}`)
	b, _, err := parseModelResponse(body, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"chapters":[]}` {
		t.Fatalf("got %s", b)
	}
}

func TestResolveMaxTokensPrecedence(t *testing.T) {
	cases := []struct {
		name    string
		flagVal int
		envVal  string
		want    int
	}{
		{"default when unset", 0, "", defaultMaxToks},
		{"env overrides default", 0, "64000", 64000},
		{"flag overrides env", 100000, "64000", 100000},
		{"flag overrides default", 50000, "", 50000},
		{"bad env falls back to default", 0, "lots", defaultMaxToks},
		{"non-positive env ignored", 0, "0", defaultMaxToks},
		{"whitespace env trimmed", 0, "  48000 ", 48000},
	}
	for _, c := range cases {
		if got := resolveMaxTokens(c.flagVal, c.envVal); got != c.want {
			t.Errorf("%s: resolveMaxTokens(%d, %q) = %d, want %d", c.name, c.flagVal, c.envVal, got, c.want)
		}
	}
}

func TestPlanToolCarriesBlockEnum(t *testing.T) {
	raw, err := json.Marshal(planTool(Index{BlockIDs: []string{"b001", "b002"}}))
	if err != nil || !json.Valid(raw) {
		t.Fatalf("planTool did not marshal to valid JSON: %v", err)
	}
	// the per-PR block-id enum constrains the model to real ids
	s := string(raw)
	if !strings.Contains(s, "b001") || !strings.Contains(s, "b002") {
		t.Fatalf("block-id enum missing from schema: %s", s)
	}
}

func TestToolShapedPlanNormalizesAndReconcilesFully(t *testing.T) {
	idx := buildIndex(sampleDiff(t))
	toolInput := []byte(`{
		"overview":"o",
		"chapters":[
			{"title":"Contract","changeUnits":[{"blocks":["b001"],"symbol":"api","layer":0,"summary":"s"}]},
			{"title":"Endpoint","changeUnits":[
				{"blocks":["b002"],"symbol":"Place","layer":1,"summary":"s"},
				{"blocks":["b003","b004","b005"],"symbol":"Service","layer":2,"summary":"s"}]}
		],
		"orphans":[{"layer":4,"changeUnits":[{"blocks":["b006"],"symbol":"","layer":4,"summary":"s"}]}]
	}`)
	var raw rawPlan
	if err := json.Unmarshal(toolInput, &raw); err != nil {
		t.Fatal(err)
	}
	plan := normalizePlan(raw, idx)
	reconcile(&plan, idx, nil)
	if !plan.Coverage.OK || plan.Coverage.Counts.Placed != 6 {
		t.Fatalf("tool-shaped plan not fully placed: %+v", plan.Coverage)
	}
}
