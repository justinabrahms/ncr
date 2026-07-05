package main

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func samplePlan(t *testing.T) ReadingPlan {
	t.Helper()
	b, err := os.ReadFile("tests/fixtures/sample-plan.json")
	if err != nil {
		t.Fatal(err)
	}
	var p ReadingPlan
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func TestCompletePlanPassesClean(t *testing.T) {
	idx := buildIndex(sampleDiff(t))
	plan := samplePlan(t)
	reconcile(&plan, idx, nil)
	cov := plan.Coverage
	if !cov.OK || cov.Counts.Indexed != 6 || cov.Counts.Placed != 6 {
		t.Fatalf("expected clean 6/6, got %+v", cov)
	}
	if len(cov.Missing) != 0 || len(cov.Duplicated) != 0 || cov.Repaired {
		t.Fatalf("unexpected issues: %+v", cov)
	}
}

func TestDroppedBlockIsRescued(t *testing.T) {
	idx := buildIndex(sampleDiff(t))
	plan := samplePlan(t)
	// simulate the model forgetting the migration block b006 (unit u4)
	var kept []Unit
	for _, u := range plan.Units {
		if u.ID != "u4" {
			kept = append(kept, u)
		}
	}
	plan.Units = kept
	plan.Orphans = nil

	reconcile(&plan, idx, nil)
	cov := plan.Coverage
	if !contains(cov.Missing, "b006") || !cov.Repaired {
		t.Fatalf("b006 not reported missing/repaired: %+v", cov)
	}
	var placed []string
	for _, u := range plan.Units {
		placed = append(placed, u.Blocks...)
	}
	sort.Strings(placed)
	if !reflect.DeepEqual(placed, []string{"b001", "b002", "b003", "b004", "b005", "b006"}) {
		t.Fatalf("after repair, blocks = %v", placed)
	}
}

func TestDuplicateBlockFlagged(t *testing.T) {
	idx := buildIndex(sampleDiff(t))
	plan := samplePlan(t)
	plan.Units[0].Blocks = append(plan.Units[0].Blocks, "b002")
	reconcile(&plan, idx, nil)
	if !contains(plan.Coverage.Duplicated, "b002") || plan.Coverage.OK {
		t.Fatalf("duplicate b002 not flagged: %+v", plan.Coverage)
	}
}

func TestCommentAnchorGapFlagged(t *testing.T) {
	idx := buildIndex(sampleDiff(t))
	plan := samplePlan(t)
	reconcile(&plan, idx, []string{"b999"})
	if !reflect.DeepEqual(plan.Coverage.CommentGaps, []string{"b999"}) {
		t.Fatalf("comment gap not flagged: %+v", plan.Coverage.CommentGaps)
	}
}

// Issue #9: when coverage fails for a non-missing reason (dangling refs,
// duplicated blocks, unplaced units, comment gaps) the status message must name
// the actual failure and must not claim things were "unplaced (auto-repaired)"
// nor point at a nonexistent Unplaced chapter.
func TestCoverageStatusNonMissingReasons(t *testing.T) {
	cov := &Coverage{
		OK:           false,
		DanglingRefs: []string{"b001#2-3", "b004#1"},
		Duplicated:   []string{"b002"},
	}
	status := coverageStatus(cov)
	if strings.Contains(status, "unplaced") || strings.Contains(status, "auto-repaired") {
		t.Fatalf("status wrongly mentions unplaced/auto-repaired: %q", status)
	}
	if !strings.Contains(status, "2 dangling block refs") {
		t.Fatalf("status missing dangling count: %q", status)
	}
	if !strings.Contains(status, "1 duplicated block") {
		t.Fatalf("status missing duplicated count: %q", status)
	}
	if hasUnplacedChapter(cov) {
		t.Fatalf("no Unplaced chapter should be referenced with zero missing blocks")
	}
}

// The missing-blocks case still reports auto-repair and points at the Unplaced
// chapter.
func TestCoverageStatusMissingBlocks(t *testing.T) {
	cov := &Coverage{OK: false, Missing: []string{"b006"}, Repaired: true}
	status := coverageStatus(cov)
	if !strings.Contains(status, "1 unplaced (auto-repaired)") {
		t.Fatalf("missing-blocks status wrong: %q", status)
	}
	if !hasUnplacedChapter(cov) {
		t.Fatalf("Unplaced chapter should be referenced when blocks are missing")
	}
}

func TestCoverageStatusOK(t *testing.T) {
	if s := coverageStatus(&Coverage{OK: true}); s != "✓ complete" {
		t.Fatalf("OK status wrong: %q", s)
	}
}
