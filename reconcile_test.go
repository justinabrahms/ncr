package main

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
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
