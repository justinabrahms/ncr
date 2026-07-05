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
