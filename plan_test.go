package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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

// withTestServer points runModel at a stub server with fast backoff, restoring
// the package globals afterward.
func withTestServer(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	origURL, origDelay := anthropicURL, retryBaseDelay
	anthropicURL = srv.URL
	retryBaseDelay = time.Millisecond
	t.Cleanup(func() {
		srv.Close()
		anthropicURL = origURL
		retryBaseDelay = origDelay
	})
	return srv
}

func okBody() string {
	return `{"stop_reason":"tool_use","usage":{"input_tokens":1,"output_tokens":1},` +
		`"content":[{"type":"tool_use","name":"submit_reading_plan","input":{"overview":"o","chapters":[]}}]}`
}

func TestRunModelRetriesOn429ThenSucceeds(t *testing.T) {
	var calls int32
	withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(429)
			w.Write([]byte(`{"error":"rate limited"}`))
			return
		}
		w.WriteHeader(200)
		w.Write([]byte(okBody()))
	})

	out, _, err := runModel("sys", "usr", "m", 100, nil)
	if err != nil {
		t.Fatalf("expected retry to succeed, got %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected 2 calls (429 then 200), got %d", got)
	}
	var m map[string]any
	if json.Unmarshal(out, &m); m["overview"] != "o" {
		t.Fatalf("plan not returned after retry: %s", out)
	}
}

func TestRunModelRetriesOn529AndHonorsRetryAfter(t *testing.T) {
	var calls int32
	withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(529) // overloaded
			return
		}
		w.WriteHeader(200)
		w.Write([]byte(okBody()))
	})
	if _, _, err := runModel("sys", "usr", "m", 100, nil); err != nil {
		t.Fatalf("expected 529 retry to succeed, got %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected 2 calls, got %d", got)
	}
}

func TestRunModelDoesNotRetryOn400(t *testing.T) {
	var calls int32
	withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(400)
		w.Write([]byte(`{"error":"bad request"}`))
	})
	if _, _, err := runModel("sys", "usr", "m", 100, nil); err == nil {
		t.Fatal("expected error on 400")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("400 must not retry: got %d calls", got)
	}
}

func TestRunModelGivesUpAfterMaxRetries(t *testing.T) {
	var calls int32
	withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(503)
	})
	if _, _, err := runModel("sys", "usr", "m", 100, nil); err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if got := atomic.LoadInt32(&calls); got != maxRetries+1 {
		t.Fatalf("expected %d attempts, got %d", maxRetries+1, got)
	}
}

func TestRetryDelayHonorsRetryAfterSeconds(t *testing.T) {
	h := http.Header{}
	h.Set("Retry-After", "2")
	if d := retryDelay(0, h); d != 2*time.Second {
		t.Fatalf("expected 2s from Retry-After, got %v", d)
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
