package main

import (
	"strings"
	"testing"
)

// A realignment (same content, different whitespace) should be flagged as noise
// on both sides; a genuine change should not.
func TestWSNoiseFlagsRealignment(t *testing.T) {
	b := Block{Lines: []DiffLine{
		{Kind: "del", Text: "-\tName string `json:\"name\"`"},
		{Kind: "del", Text: "-\tAge  int    `json:\"age\"`"},
		{Kind: "add", Text: "+\tName string `json:\"name\"`"},   // realigned only
		{Kind: "add", Text: "+\tAge int `json:\"age\"`"},        // realigned only
		{Kind: "add", Text: "+\tEmail string `json:\"email\"`"}, // genuinely new
	}}
	noise := wsNoise(b)
	want := []bool{true, true, true, true, false}
	for i := range want {
		if noise[i] != want[i] {
			t.Fatalf("line %d: noise=%v want %v (%q)", i, noise[i], want[i], b.Lines[i].Text)
		}
	}
}

func TestWSToggleRenderedWhenNoisePresent(t *testing.T) {
	// sample.diff has no realignment noise → no toggle button (the .wstoggle CSS
	// rule is always in the <style>, so match the button element specifically)
	idx := buildIndex(sampleDiff(t))
	html, _ := BuildHTML(samplePlan(t), idx, false, "owner/repo")
	if strings.Contains(string(html), `class="wstoggle"`) {
		t.Fatal("no whitespace noise in the sample, so no toggle button expected")
	}
	// a block with realignment → toggle button + wsn class
	b := Block{BlockID: "b1", Path: "x.go", Lines: []DiffLine{
		{Kind: "del", Text: "-a  b", OldNo: 1},
		{Kind: "add", Text: "+a b", NewNo: 1},
	}}
	out := string(diffHTML([]diffSeg{{block: b}}, "go", "x.go"))
	if !strings.Contains(out, `class="wstoggle"`) || !strings.Contains(out, "l add wsn") {
		t.Fatalf("expected toggle + wsn class, got: %s", out)
	}
}
