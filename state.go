package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Review state — the local, private comment queue for a PR. Persisted under
// $NCR_STATE_DIR or ~/.ncr/reviews/<owner>__<repo>__<pr>.json. See
// docs/design-review-comments.md.

type ReviewComment struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	Side      string `json:"side"` // RIGHT (added/context) | LEFT (removed)
	Line      int    `json:"line"` // end line (GitHub `line`)
	StartLine int    `json:"startLine,omitempty"`
	StartSide string `json:"startSide,omitempty"`
	LineText  string `json:"lineText"` // snapshot of the end line, for re-anchoring (phase 5)
	Body      string `json:"body"`
	CreatedAt string `json:"createdAt"`
}

type SubmittedRound struct {
	Round       int             `json:"round"`
	ReviewURL   string          `json:"reviewUrl"`
	Verdict     string          `json:"verdict"`
	Body        string          `json:"body"`
	SubmittedAt string          `json:"submittedAt"`
	HeadSha     string          `json:"headSha"`
	Comments    []ReviewComment `json:"comments"`
}

type ReviewDraft struct {
	Body    string `json:"body"`
	Verdict string `json:"verdict"`
}

type ReviewState struct {
	Repo      string           `json:"repo"`
	PR        int              `json:"pr"`
	HeadSha   string           `json:"headSha"`
	Seq       int              `json:"seq"` // monotonic id counter
	Draft     ReviewDraft      `json:"draft"`
	Pending   []ReviewComment  `json:"pending"`
	Submitted []SubmittedRound `json:"submitted"`
}

func stateDir() string {
	if d := os.Getenv("NCR_STATE_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".ncr", "reviews")
	}
	return filepath.Join(home, ".ncr", "reviews")
}

func statePath(repo string, pr int) string {
	slug := strings.ReplaceAll(repo, "/", "__")
	return filepath.Join(stateDir(), fmt.Sprintf("%s__%d.json", slug, pr))
}

func loadState(repo string, pr int) (*ReviewState, error) {
	b, err := os.ReadFile(statePath(repo, pr))
	if err != nil {
		if os.IsNotExist(err) {
			return &ReviewState{Repo: repo, PR: pr, Draft: ReviewDraft{Verdict: "COMMENT"}}, nil
		}
		return nil, err
	}
	var s ReviewState
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	s.Repo, s.PR = repo, pr
	if s.Draft.Verdict == "" {
		s.Draft.Verdict = "COMMENT"
	}
	return &s, nil
}

func saveState(s *ReviewState) error {
	if err := os.MkdirAll(stateDir(), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statePath(s.Repo, s.PR), b, 0o644)
}
