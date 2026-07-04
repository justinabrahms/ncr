package main

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestServer(t *testing.T) *reviewServer {
	t.Helper()
	t.Setenv("NCR_STATE_DIR", t.TempDir())
	rs, err := newReviewServer("owner/repo", 7, "headsha", buildIndex(sampleDiff(t)), ReadingPlan{}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	return rs
}

func (rs *reviewServer) do(method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	rs.handler([]byte("PAGE")).ServeHTTP(rec, req)
	return rec
}

// a real added-line anchor from the sample diff
func validAnchor(idx Index) (string, string, int) {
	for _, b := range idx.Blocks {
		for _, l := range b.Lines {
			if l.Kind == "add" {
				return b.Path, "RIGHT", l.NewNo
			}
		}
	}
	return "", "", 0
}

func TestServesPageAndAssets(t *testing.T) {
	rs := newTestServer(t)
	if rec := rs.do("GET", "/", ""); rec.Code != 200 || rec.Body.String() != "PAGE" {
		t.Fatalf("page: %d %q", rec.Code, rec.Body.String())
	}
	if rec := rs.do("GET", "/review.js", ""); rec.Code != 200 || rec.Body.Len() == 0 {
		t.Fatalf("review.js: %d len=%d", rec.Code, rec.Body.Len())
	}
	if rec := rs.do("GET", "/review.css", ""); rec.Code != 200 {
		t.Fatalf("review.css: %d", rec.Code)
	}
}

func TestAddCommentValidatesAndPersists(t *testing.T) {
	rs := newTestServer(t)
	p, side, line := validAnchor(rs.index)

	// invalid anchor rejected
	bad := fmt.Sprintf(`{"path":%q,"side":"RIGHT","line":99999,"body":"x"}`, p)
	if rec := rs.do("POST", "/api/comments", bad); rec.Code != 422 {
		t.Fatalf("bad anchor: want 422, got %d", rec.Code)
	}
	// empty body rejected
	empty := fmt.Sprintf(`{"path":%q,"side":%q,"line":%d,"body":"  "}`, p, side, line)
	if rec := rs.do("POST", "/api/comments", empty); rec.Code != 422 {
		t.Fatalf("empty body: want 422, got %d", rec.Code)
	}
	// valid comment stored
	ok := fmt.Sprintf(`{"path":%q,"side":%q,"line":%d,"body":"looks off"}`, p, side, line)
	rec := rs.do("POST", "/api/comments", ok)
	if rec.Code != 200 {
		t.Fatalf("valid add: %d %s", rec.Code, rec.Body.String())
	}
	// persisted to disk
	loaded, err := loadState("owner/repo", 7)
	if err != nil || len(loaded.Pending) != 1 || loaded.Pending[0].Body != "looks off" {
		t.Fatalf("not persisted: %+v (%v)", loaded, err)
	}
	// visible via /api/state
	var st struct {
		Pending []ReviewComment `json:"pending"`
	}
	json.Unmarshal(rs.do("GET", "/api/state", "").Body.Bytes(), &st)
	if len(st.Pending) != 1 {
		t.Fatalf("state pending = %d", len(st.Pending))
	}
}

func TestEditAndDeleteComment(t *testing.T) {
	rs := newTestServer(t)
	p, side, line := validAnchor(rs.index)
	add := fmt.Sprintf(`{"path":%q,"side":%q,"line":%d,"body":"first"}`, p, side, line)
	var c ReviewComment
	json.Unmarshal(rs.do("POST", "/api/comments", add).Body.Bytes(), &c)

	if rec := rs.do("PATCH", "/api/comments/"+c.ID, `{"body":"edited"}`); rec.Code != 200 {
		t.Fatalf("edit: %d", rec.Code)
	}
	if loaded, _ := loadState("owner/repo", 7); loaded.Pending[0].Body != "edited" {
		t.Fatalf("edit not saved")
	}
	if rec := rs.do("DELETE", "/api/comments/"+c.ID, ""); rec.Code != 204 {
		t.Fatalf("delete: %d", rec.Code)
	}
	if loaded, _ := loadState("owner/repo", 7); len(loaded.Pending) != 0 {
		t.Fatalf("delete left %d", len(loaded.Pending))
	}
	if rec := rs.do("DELETE", "/api/comments/nope", ""); rec.Code != 404 {
		t.Fatalf("missing delete: %d", rec.Code)
	}
}

func TestSubmitDryRunFlagsUnanchorable(t *testing.T) {
	rs := newTestServer(t)
	// inject a comment whose anchor isn't in the diff (as if a push moved it)
	rs.state.Pending = []ReviewComment{{ID: "c1", Path: "gone.go", Side: "RIGHT", Line: 5, Body: "x"}}
	rec := rs.do("POST", "/api/review/submit", `{"verdict":"COMMENT","dryRun":true}`)
	var out struct {
		OK      bool     `json:"ok"`
		Invalid []string `json:"invalid"`
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	if out.OK || len(out.Invalid) != 1 || out.Invalid[0] != "c1" {
		t.Fatalf("dry-run should flag c1: %+v", out)
	}
}

func TestSubmitArchivesAndClears(t *testing.T) {
	rs := newTestServer(t)
	var captured []byte
	rs.submitFn = func(payload []byte) (string, error) {
		captured = payload
		return "https://github.com/owner/repo/pull/7#r1", nil
	}

	p, side, line := validAnchor(rs.index)
	add := fmt.Sprintf(`{"path":%q,"side":%q,"line":%d,"body":"nit"}`, p, side, line)
	rs.do("POST", "/api/comments", add)

	rec := rs.do("POST", "/api/review/submit", `{"verdict":"REQUEST_CHANGES","body":"take a look"}`)
	if rec.Code != 200 {
		t.Fatalf("submit: %d %s", rec.Code, rec.Body.String())
	}
	// payload shape
	var payload map[string]any
	json.Unmarshal(captured, &payload)
	if payload["event"] != "REQUEST_CHANGES" || payload["commit_id"] != "headsha" {
		t.Fatalf("payload: %v", payload)
	}
	if cs, _ := payload["comments"].([]any); len(cs) != 1 {
		t.Fatalf("expected 1 comment in payload, got %v", payload["comments"])
	}
	// pending cleared, round archived, persisted
	loaded, _ := loadState("owner/repo", 7)
	if len(loaded.Pending) != 0 || len(loaded.Submitted) != 1 || loaded.Submitted[0].ReviewURL == "" {
		t.Fatalf("archive state: %+v", loaded)
	}
	if len(loaded.Submitted[0].Comments) != 1 {
		t.Fatalf("round should keep its comments")
	}
}

func TestSubmitEmptyBlockedButApproveOK(t *testing.T) {
	rs := newTestServer(t)
	rs.submitFn = func(payload []byte) (string, error) { return "https://x", nil }

	// COMMENT with no comments and no body → blocked
	if rec := rs.do("POST", "/api/review/submit", `{"verdict":"COMMENT"}`); rec.Code != 409 {
		t.Fatalf("empty comment review: want 409, got %d", rec.Code)
	}
	// APPROVE with no comments and no body → allowed
	if rec := rs.do("POST", "/api/review/submit", `{"verdict":"APPROVE"}`); rec.Code != 200 {
		t.Fatalf("bare approve: want 200, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestDebugDumpsPlanCoverageAndReview(t *testing.T) {
	t.Setenv("NCR_STATE_DIR", t.TempDir())
	idx := buildIndex(sampleDiff(t))
	layer := 1
	plan := ReadingPlan{
		Title:    "My PR",
		Overview: "does a thing",
		Chapters: []Chapter{{ID: "ch1", Title: "cap"}},
		Units:    []Unit{{ID: "u1", File: "a.go", Symbol: "F", Layer: &layer}},
		Coverage: &Coverage{OK: false, Missing: []string{"b1"}},
	}
	rawPlan := json.RawMessage(`{"overview":"raw model output","chapters":[]}`)
	rs, err := newReviewServer("owner/repo", 7, "headsha", idx, plan, rawPlan, "claude-test-model")
	if err != nil {
		t.Fatal(err)
	}

	// default dump: plan + coverage + review, no index
	rec := rs.do("GET", "/api/debug", "")
	if rec.Code != 200 {
		t.Fatalf("debug: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Version  string          `json:"version"`
		Model    string          `json:"model"`
		HeadSha  string          `json:"headSha"`
		Plan     ReadingPlan     `json:"plan"`
		RawPlan  json.RawMessage `json:"rawPlan"`
		Coverage *Coverage       `json:"coverage"`
		Index    *Index          `json:"index"`
		Review   struct {
			Pending []ReviewComment `json:"pending"`
		} `json:"review"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Model != "claude-test-model" || out.HeadSha != "headsha" || out.Version == "" {
		t.Fatalf("meta wrong: %+v", out)
	}
	if out.Plan.Title != "My PR" || len(out.Plan.Chapters) != 1 {
		t.Fatalf("plan not dumped: %+v", out.Plan)
	}
	if out.Coverage == nil || out.Coverage.OK || len(out.Coverage.Missing) != 1 {
		t.Fatalf("coverage not dumped: %+v", out.Coverage)
	}
	if !strings.Contains(string(out.RawPlan), "raw model output") {
		t.Fatalf("rawPlan not dumped verbatim: %s", out.RawPlan)
	}
	if out.Review.Pending == nil {
		t.Fatalf("review.pending should be [] not null")
	}
	if out.Index != nil {
		t.Fatalf("index should be omitted without ?verbose=1")
	}

	// verbose includes the block index
	rec = rs.do("GET", "/api/debug?verbose=1", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.Index == nil || len(out.Index.BlockIDs) == 0 {
		t.Fatalf("verbose should include a non-empty index")
	}
}
