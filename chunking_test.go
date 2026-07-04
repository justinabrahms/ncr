package main

import (
	"fmt"
	"testing"
)

func TestParseSegment(t *testing.T) {
	cases := []struct {
		in       string
		id       string
		from, to int
		ok       bool
	}{
		{"b012", "b012", 0, 0, true},
		{"b012:3-18", "b012", 3, 18, true},
		{"b007:5-5", "b007", 5, 5, true},
		{"b012:5-3", "b012", 0, 0, false}, // inverted
		{"b012:0-3", "b012", 0, 0, false}, // zero start
		{"nope!", "", 0, 0, false},        // bad shape
	}
	for _, c := range cases {
		id, from, to, ok := parseSegment(c.in)
		if id != c.id || from != c.from || to != c.to || ok != c.ok {
			t.Fatalf("parseSegment(%q) = (%q,%d,%d,%v), want (%q,%d,%d,%v)",
				c.in, id, from, to, ok, c.id, c.from, c.to, c.ok)
		}
	}
}

// helper: a plan whose units all sit in one chapter (so they count as "placed")
func planWith(units ...Unit) ReadingPlan {
	var nodes []Node
	for _, u := range units {
		nodes = append(nodes, Node{Unit: u.ID})
	}
	return ReadingPlan{Units: units, Chapters: []Chapter{{ID: "c1", Nodes: nodes}}}
}

func blockK(t *testing.T, idx Index, id string) int {
	for _, b := range idx.Blocks {
		if b.BlockID == id {
			return changedCount(b)
		}
	}
	t.Fatalf("block %s not found", id)
	return 0
}

func TestSplitBlockCoversExactlyOnce(t *testing.T) {
	idx := buildIndex(sampleDiff(t))
	k := blockK(t, idx, "b002")
	mid := k / 2
	plan := planWith(
		Unit{ID: "u1", Blocks: []string{"b001"}},
		Unit{ID: "u2", Blocks: []string{fmt.Sprintf("b002:1-%d", mid)}},
		Unit{ID: "u3", Blocks: []string{fmt.Sprintf("b002:%d-%d", mid+1, k)}},
		Unit{ID: "u4", Blocks: []string{"b003"}},
		Unit{ID: "u5", Blocks: []string{"b004"}},
	)
	reconcile(&plan, idx, nil)
	cov := plan.Coverage
	if !cov.OK || cov.Repaired || cov.Counts.Placed != 4 {
		t.Fatalf("split block should cover cleanly: %+v", cov)
	}
}

func TestSplitGapIsAutoRepaired(t *testing.T) {
	idx := buildIndex(sampleDiff(t))
	k := blockK(t, idx, "b002")
	plan := planWith(
		Unit{ID: "u1", Blocks: []string{"b001"}},
		Unit{ID: "u2", Blocks: []string{"b002:1-5"}}, // leaves b002:6-k uncovered
		Unit{ID: "u4", Blocks: []string{"b003"}},
		Unit{ID: "u5", Blocks: []string{"b004"}},
	)
	reconcile(&plan, idx, nil)
	want := fmt.Sprintf("b002:6-%d", k)
	if !contains(plan.Coverage.Missing, want) || !plan.Coverage.Repaired {
		t.Fatalf("gap %s should be reported + repaired: %+v", want, plan.Coverage)
	}
	// after repair every block is fully covered
	if plan.Coverage.Counts.Placed != 4 {
		t.Fatalf("placed after repair = %d, want 4", plan.Coverage.Counts.Placed)
	}
}

func TestSplitOverlapIsFlagged(t *testing.T) {
	idx := buildIndex(sampleDiff(t))
	k := blockK(t, idx, "b002")
	plan := planWith(
		Unit{ID: "u1", Blocks: []string{"b001"}},
		Unit{ID: "u2", Blocks: []string{"b002:1-8"}},
		Unit{ID: "u3", Blocks: []string{fmt.Sprintf("b002:6-%d", k)}}, // overlaps 6-8
		Unit{ID: "u4", Blocks: []string{"b003"}},
		Unit{ID: "u5", Blocks: []string{"b004"}},
	)
	reconcile(&plan, idx, nil)
	if !contains(plan.Coverage.Duplicated, "b002:6-8") || plan.Coverage.OK {
		t.Fatalf("overlap b002:6-8 should be flagged: %+v", plan.Coverage)
	}
}

func TestDanglingSegmentReported(t *testing.T) {
	idx := buildIndex(sampleDiff(t))
	plan := planWith(
		Unit{ID: "u1", Blocks: []string{"b001", "b002", "b003", "b004"}},
		Unit{ID: "u2", Blocks: []string{"bZZZ"}}, // nonexistent
	)
	reconcile(&plan, idx, nil)
	if !contains(plan.Coverage.DanglingRefs, "bZZZ") || plan.Coverage.OK {
		t.Fatalf("dangling bZZZ should be reported: %+v", plan.Coverage)
	}
}

func TestSegLinesRestrictsToRange(t *testing.T) {
	idx := buildIndex(sampleDiff(t))
	var b002 Block
	for _, b := range idx.Blocks {
		if b.BlockID == "b002" {
			b002 = b
		}
	}
	_, changed, _ := splitBlockLines(b002)
	// whole block renders leading context + all changed + trailing context
	if got := len(segLines(b002, 0, 0)); got != len(b002.Lines) {
		t.Fatalf("whole seg = %d lines, want %d", got, len(b002.Lines))
	}
	// a mid range renders exactly its changed lines, no surrounding context
	sub := segLines(b002, 2, 4)
	if len(sub) != 3 {
		t.Fatalf("range 2-4 = %d lines, want 3", len(sub))
	}
	for _, l := range sub {
		if l.Kind == "ctx" {
			t.Fatal("a mid sub-range should carry no context lines")
		}
	}
	_ = changed
}
