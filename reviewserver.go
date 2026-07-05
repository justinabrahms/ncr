package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The interactive review backend: comment queue CRUD + validate + submit one
// GitHub review via `gh api`. State is persisted on every mutation. Phases 3–4 of
// docs/design-review-comments.md.

type reviewServer struct {
	mu       sync.Mutex
	state    *ReviewState
	index    Index
	plan     ReadingPlan     // the reconciled reading plan this session is serving
	rawPlan  json.RawMessage // the plan's raw bytes, pre normalize/reconcile (nil in tests)
	model    string          // model that produced the plan ("" when loaded from --plan)
	repo     string
	pr       int
	anchors  map[string]bool                      // valid (path,side,line) comment positions
	nowFn    func() string                        // injectable for tests
	submitFn func(payload []byte) (string, error) // posts the review; gh by default
}

func newReviewServer(repo string, pr int, headSha string, index Index, plan ReadingPlan, rawPlan json.RawMessage, model string) (*reviewServer, error) {
	st, err := loadState(repo, pr)
	if err != nil {
		return nil, err
	}
	headChanged := st.HeadSha != "" && headSha != "" && st.HeadSha != headSha
	if headChanged {
		// The PR moved under us: relocate each pending comment to its new line
		// by its snapshotted LineText; flag the ones we can't place (phase 5).
		moved, stale := reanchorPending(st, index)
		logf("note: PR head changed since last session (%s → %s); re-anchored %d pending comment(s), %d need re-placing",
			shortSha(st.HeadSha), shortSha(headSha), moved, stale)
	}
	if headSha != "" {
		st.HeadSha = headSha
	}
	if headChanged {
		if err := saveState(st); err != nil {
			return nil, err
		}
	}
	rs := &reviewServer{
		state:    st,
		index:    index,
		plan:     plan,
		rawPlan:  rawPlan,
		model:    model,
		repo:     repo,
		pr:       pr,
		anchors:  buildAnchorSet(index),
		nowFn:    func() string { return time.Now().UTC().Format(time.RFC3339) },
		submitFn: ghSubmit(repo, pr),
	}
	return rs, nil
}

func shortSha(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

func anchorKey(path, side string, line int) string {
	return path + "\x00" + side + "\x00" + strconv.Itoa(line)
}

// buildAnchorSet records every commentable (path, side, line) the diff renders:
// added/context lines on RIGHT (new-file number), removed lines on LEFT (old).
func buildAnchorSet(index Index) map[string]bool {
	set := map[string]bool{}
	for _, b := range index.Blocks {
		for _, l := range b.Lines {
			switch l.Kind {
			case "del":
				set[anchorKey(b.Path, "LEFT", l.OldNo)] = true
			default:
				set[anchorKey(b.Path, "RIGHT", l.NewNo)] = true
			}
		}
	}
	return set
}

func lineTextKey(path, side, text string) string {
	return path + "\x00" + side + "\x00" + text
}

// buildLineTextIndex maps each rendered line's (path, side, text) to the file
// line numbers where it appears in the current diff — the inverse of the anchor
// set, used to relocate a pending comment by its snapshotted LineText.
func buildLineTextIndex(index Index) map[string][]int {
	seen := map[string]map[int]bool{}
	for _, b := range index.Blocks {
		for _, l := range b.Lines {
			side, no := "RIGHT", l.NewNo
			if l.Kind == "del" {
				side, no = "LEFT", l.OldNo
			}
			k := lineTextKey(b.Path, side, strip(l.Text))
			if seen[k] == nil {
				seen[k] = map[int]bool{}
			}
			seen[k][no] = true
		}
	}
	out := map[string][]int{}
	for k, nums := range seen {
		for n := range nums {
			out[k] = append(out[k], n)
		}
	}
	return out
}

// reanchorPending relocates each pending comment after the PR head moved: it
// finds the comment's snapshotted LineText in the new diff on the same side. An
// exactly-one match follows the move (line updated, ranges shifted by the same
// delta); zero or multiple matches (or a missing snapshot) are flagged Stale so
// submit surfaces them in the "needs re-placing" tray instead of failing
// opaquely at the API call. Submitted rounds are untouched. Returns counts for
// the startup log.
func reanchorPending(st *ReviewState, index Index) (moved, stale int) {
	lookup := buildLineTextIndex(index)
	for i := range st.Pending {
		c := &st.Pending[i]
		matches := lookup[lineTextKey(c.Path, c.Side, c.LineText)]
		if c.LineText != "" && len(matches) == 1 {
			newLine := matches[0]
			if c.StartLine > 0 {
				c.StartLine += newLine - c.Line // keep the range span across the move
			}
			c.Line = newLine
			c.Stale = false
			moved++
			continue
		}
		c.Stale = true
		stale++
	}
	return moved, stale
}

func (rs *reviewServer) handler(html []byte) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(html)
	})
	mux.HandleFunc("GET /review.js", asset("text/javascript", reviewJS))
	mux.HandleFunc("GET /review.css", asset("text/css", reviewCSS))
	mux.HandleFunc("GET /api/state", rs.handleState)
	mux.HandleFunc("GET /api/debug", rs.handleDebug)
	mux.HandleFunc("POST /api/comments", rs.handleAdd)
	mux.HandleFunc("PATCH /api/comments/{id}", rs.handleEdit)
	mux.HandleFunc("DELETE /api/comments/{id}", rs.handleDelete)
	mux.HandleFunc("PUT /api/draft", rs.handleDraft)
	mux.HandleFunc("POST /api/review/submit", rs.handleSubmit)
	return mux
}

func asset(ct string, body []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", ct+"; charset=utf-8")
		_, _ = w.Write(body)
	}
}

func writeJSONResp(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (rs *reviewServer) handleState(w http.ResponseWriter, r *http.Request) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	writeJSONResp(w, 200, map[string]any{
		"headSha":   rs.state.HeadSha,
		"draft":     rs.state.Draft,
		"pending":   nonNilComments(rs.state.Pending),
		"submitted": rs.state.Submitted,
	})
}

// handleDebug dumps the full in-memory session state — the reconciled reading
// plan, the raw pre-reconcile plan the model produced, the coverage report, and
// the review-comment queue — for an external MCP server or bug-report triage to
// introspect without scraping the rendered HTML. rawPlan lets triage see what the
// model returned before normalize/reconcile touched it (null when unavailable).
// The block index (large: full rendered diff lines) is included only with
// ?verbose=1, so the default payload stays small and MCP-friendly. Read-only.
func (rs *reviewServer) handleDebug(w http.ResponseWriter, r *http.Request) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	out := map[string]any{
		"version":  versionString(),
		"repo":     rs.repo,
		"pr":       rs.pr,
		"headSha":  rs.state.HeadSha,
		"model":    rs.model,
		"plan":     rs.plan,
		"rawPlan":  rs.rawPlan,
		"coverage": rs.plan.Coverage,
		"review": map[string]any{
			"draft":     rs.state.Draft,
			"pending":   nonNilComments(rs.state.Pending),
			"submitted": rs.state.Submitted,
		},
	}
	if r.URL.Query().Get("verbose") == "1" {
		out["index"] = rs.index
	}
	writeJSONResp(w, 200, out)
}

func (rs *reviewServer) handleAdd(w http.ResponseWriter, r *http.Request) {
	var c ReviewComment
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeJSONResp(w, 400, map[string]string{"error": "bad request"})
		return
	}
	if strings.TrimSpace(c.Body) == "" {
		writeJSONResp(w, 422, map[string]string{"error": "comment body is empty"})
		return
	}
	if bad := rs.anchorError(c); bad != "" {
		writeJSONResp(w, 422, map[string]string{"error": bad})
		return
	}
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.state.Seq++
	c.ID = fmt.Sprintf("c%d", rs.state.Seq)
	c.CreatedAt = rs.nowFn()
	rs.state.Pending = append(rs.state.Pending, c)
	if err := saveState(rs.state); err != nil {
		writeJSONResp(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSONResp(w, 200, c)
}

func (rs *reviewServer) handleEdit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var patch struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil || strings.TrimSpace(patch.Body) == "" {
		writeJSONResp(w, 400, map[string]string{"error": "body required"})
		return
	}
	rs.mu.Lock()
	defer rs.mu.Unlock()
	for i := range rs.state.Pending {
		if rs.state.Pending[i].ID == id {
			rs.state.Pending[i].Body = patch.Body
			_ = saveState(rs.state)
			writeJSONResp(w, 200, rs.state.Pending[i])
			return
		}
	}
	writeJSONResp(w, 404, map[string]string{"error": "not found"})
}

func (rs *reviewServer) handleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rs.mu.Lock()
	defer rs.mu.Unlock()
	kept := rs.state.Pending[:0]
	found := false
	for _, c := range rs.state.Pending {
		if c.ID == id {
			found = true
			continue
		}
		kept = append(kept, c)
	}
	rs.state.Pending = kept
	if !found {
		writeJSONResp(w, 404, map[string]string{"error": "not found"})
		return
	}
	_ = saveState(rs.state)
	w.WriteHeader(204)
}

func (rs *reviewServer) handleDraft(w http.ResponseWriter, r *http.Request) {
	var d ReviewDraft
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		writeJSONResp(w, 400, map[string]string{"error": "bad request"})
		return
	}
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.state.Draft = d
	_ = saveState(rs.state)
	writeJSONResp(w, 200, d)
}

func (rs *reviewServer) handleSubmit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Verdict string `json:"verdict"`
		Body    string `json:"body"`
		DryRun  bool   `json:"dryRun"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONResp(w, 400, map[string]string{"error": "bad request"})
		return
	}
	rs.mu.Lock()
	defer rs.mu.Unlock()

	rs.state.Draft = ReviewDraft{Body: req.Body, Verdict: req.Verdict}
	_ = saveState(rs.state)

	invalid, errs := rs.validate(req.Verdict, req.Body)
	if req.DryRun {
		writeJSONResp(w, 200, map[string]any{"ok": len(invalid) == 0 && len(errs) == 0, "invalid": invalid, "errors": errs})
		return
	}
	if len(invalid) > 0 || len(errs) > 0 {
		writeJSONResp(w, 409, map[string]any{"invalid": invalid, "errors": errs})
		return
	}

	url, err := rs.submitReview(req.Verdict, req.Body)
	if err != nil {
		writeJSONResp(w, 502, map[string]string{"error": err.Error()})
		return
	}

	round := len(rs.state.Submitted) + 1
	rs.state.Submitted = append(rs.state.Submitted, SubmittedRound{
		Round: round, ReviewURL: url, Verdict: req.Verdict, Body: req.Body,
		SubmittedAt: rs.nowFn(), HeadSha: rs.state.HeadSha, Comments: rs.state.Pending,
	})
	rs.state.Pending = nil
	rs.state.Draft = ReviewDraft{Verdict: "COMMENT"}
	_ = saveState(rs.state)
	writeJSONResp(w, 200, map[string]any{"reviewUrl": url, "round": round})
}

// validate returns the ids of comments that can't anchor and any submission-rule
// errors. GitHub rejects the whole review if one comment is invalid, so we block.
func (rs *reviewServer) validate(verdict, body string) (invalid []string, errs []string) {
	for _, c := range rs.state.Pending {
		if c.Stale || rs.anchorError(c) != "" {
			invalid = append(invalid, c.ID)
		}
	}
	switch verdict {
	case "APPROVE", "COMMENT", "REQUEST_CHANGES":
	default:
		errs = append(errs, "pick a verdict")
	}
	if len(rs.state.Pending) == 0 && verdict != "APPROVE" && strings.TrimSpace(body) == "" {
		errs = append(errs, "nothing to submit — add a comment or a summary")
	}
	return invalid, errs
}

// anchorError returns "" if the comment anchors in the current diff, else why not.
func (rs *reviewServer) anchorError(c ReviewComment) string {
	if !rs.anchors[anchorKey(c.Path, c.Side, c.Line)] {
		return "line is not part of the diff"
	}
	if c.StartLine > 0 {
		ss := c.StartSide
		if ss == "" {
			ss = c.Side
		}
		if ss != c.Side {
			return "a range must be on one side"
		}
		if c.StartLine > c.Line {
			return "range start is after its end"
		}
		if !rs.anchors[anchorKey(c.Path, ss, c.StartLine)] {
			return "range start is not part of the diff"
		}
	}
	return ""
}

func (rs *reviewServer) submitReview(verdict, body string) (string, error) {
	type ghComment struct {
		Path      string `json:"path"`
		Line      int    `json:"line"`
		Side      string `json:"side"`
		StartLine int    `json:"start_line,omitempty"`
		StartSide string `json:"start_side,omitempty"`
		Body      string `json:"body"`
	}
	var comments []ghComment
	for _, c := range rs.state.Pending {
		gc := ghComment{Path: c.Path, Line: c.Line, Side: c.Side, Body: c.Body}
		if c.StartLine > 0 {
			gc.StartLine = c.StartLine
			gc.StartSide = c.StartSide
			if gc.StartSide == "" {
				gc.StartSide = c.Side
			}
		}
		comments = append(comments, gc)
	}
	payload := map[string]any{"commit_id": rs.state.HeadSha, "event": verdict, "comments": comments}
	if strings.TrimSpace(body) != "" {
		payload["body"] = body
	}
	buf, _ := json.Marshal(payload)
	return rs.submitFn(buf)
}

// ghSubmit posts a review to GitHub via the gh CLI (reuses the user's auth).
func ghSubmit(repo string, pr int) func([]byte) (string, error) {
	return func(payload []byte) (string, error) {
		cmd := exec.Command("gh", "api", "--method", "POST",
			fmt.Sprintf("repos/%s/pulls/%d/reviews", repo, pr), "--input", "-")
		cmd.Stdin = bytes.NewReader(payload)
		var out, errb bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &errb
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("gh review submit failed: %s", strings.TrimSpace(errb.String()))
		}
		var resp struct {
			HTMLURL string `json:"html_url"`
		}
		_ = json.Unmarshal(out.Bytes(), &resp)
		return resp.HTMLURL, nil
	}
}

func nonNilComments(c []ReviewComment) []ReviewComment {
	if c == nil {
		return []ReviewComment{}
	}
	return c
}
