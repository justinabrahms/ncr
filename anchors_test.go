package main

import (
	"strings"
	"testing"
)

// Phase 2: every rendered diff line must carry a resolvable GitHub anchor.
func TestRenderedLinesCarryAnchors(t *testing.T) {
	idx := buildIndex(sampleDiff(t))
	html, err := BuildHTML(samplePlan(t), idx, true, "owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	h := string(html)
	for _, want := range []string{
		`data-path="internal/order/handler.go"`,
		`data-side="RIGHT"`,
		`data-line="23"`, // the added `func ... Place` line
		`/review.js`,     // interactive assets included
	} {
		if !strings.Contains(h, want) {
			t.Fatalf("rendered HTML missing %q", want)
		}
	}
}

func TestStaticOmitsInteractiveAssets(t *testing.T) {
	html, err := BuildHTML(samplePlan(t), buildIndex(sampleDiff(t)), false, "owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	h := string(html)
	if strings.Contains(h, "/review.js") {
		t.Fatal("static render should not include the review script")
	}
	// anchors are still present (harmless in static output)
	if !strings.Contains(h, `data-line=`) {
		t.Fatal("anchors should still be emitted")
	}
}

// anchorComments must match coordinate spaces: a live comment's `line` is a
// NEW-file position (compare vs NewStart/NewLines); a fallback `original_line`
// (used when `line` is null, e.g. outdated/LEFT-side comments) is an OLD-file
// position (compare vs OldStart/OldLines).
func TestAnchorCommentsCoordinateSpaces(t *testing.T) {
	ptr := func(n int) *int { return &n }
	// Two blocks for the same file whose old/new line ranges do NOT overlap,
	// so anchoring against the wrong coordinate space would land on the wrong
	// block (or miss entirely).
	idx := Index{Blocks: []Block{
		{
			BlockID:  "b001",
			Path:     "foo.go",
			OldStart: ptr(10), OldLines: 5, // old lines 10..14
			NewStart: ptr(100), NewLines: 5, // new lines 100..104
		},
		{
			BlockID:  "b002",
			Path:     "foo.go",
			OldStart: ptr(50), OldLines: 5, // old lines 50..54
			NewStart: ptr(200), NewLines: 5, // new lines 200..204
		},
	}}

	// A normal comment anchors via NewStart/NewLines.
	got := anchorComments(idx, []Comment{{Path: "foo.go", Line: ptr(201)}})
	if len(got) != 1 || got[0] != "b002" {
		t.Fatalf("live line comment: got %v, want [b002]", got)
	}

	// A comment with null `line` and an OLD-file `original_line` anchors via
	// OldStart/OldLines — here old line 12 belongs to b001. (Under the old
	// bug it would be compared against NewStart 100/200 and miss entirely.)
	got = anchorComments(idx, []Comment{{Path: "foo.go", OriginalLine: ptr(12)}})
	if len(got) != 1 || got[0] != "b001" {
		t.Fatalf("outdated original_line comment: got %v, want [b001]", got)
	}
}

// buildAnchorSet must accept every rendered anchor (add/context RIGHT, del LEFT).
func TestAnchorSetMatchesRenderedLines(t *testing.T) {
	idx := buildIndex(sampleDiff(t))
	set := buildAnchorSet(idx)
	for _, b := range idx.Blocks {
		for _, l := range b.Lines {
			side, line := "RIGHT", l.NewNo
			if l.Kind == "del" {
				side, line = "LEFT", l.OldNo
			}
			if !set[anchorKey(b.Path, side, line)] {
				t.Fatalf("anchor missing for %s %s %d", b.Path, side, line)
			}
		}
	}
}
