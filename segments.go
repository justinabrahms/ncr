package main

import (
	"regexp"
	"strconv"
)

// A unit references its coverage as "segments": either a whole block ("b012") or
// a 1-based inclusive line range within a block's changed lines ("b012:3-18").
// This lets the narrative split a large block — while the reconciler still proves
// every changed line is covered exactly once (docs/completeness.md).

var segRe = regexp.MustCompile(`^([A-Za-z0-9_.-]+?)(?::(\d+)-(\d+))?$`)

// parseSegment parses "b012" or "b012:3-18". from==0 means the whole block. ok is
// false for malformed input (unknown shape or an inverted/zero range) — callers
// treat that as "covers nothing", so the reconciler's line accounting catches it.
func parseSegment(s string) (id string, from, to int, ok bool) {
	m := segRe.FindStringSubmatch(s)
	if m == nil {
		return "", 0, 0, false
	}
	id = m[1]
	if m[2] == "" {
		return id, 0, 0, true
	}
	from, _ = strconv.Atoi(m[2])
	to, _ = strconv.Atoi(m[3])
	if from < 1 || to < from {
		return id, 0, 0, false
	}
	return id, from, to, true
}

// splitBlockLines separates a block's rendered lines into leading context, the
// changed (+/-) lines, and trailing context. A block is always [ctx*, changed*,
// ctx*] by construction (see indexDiff).
func splitBlockLines(b Block) (before, changed, after []DiffLine) {
	i := 0
	for i < len(b.Lines) && b.Lines[i].Kind == "ctx" {
		i++
	}
	j := len(b.Lines)
	for j > i && b.Lines[j-1].Kind == "ctx" {
		j--
	}
	return b.Lines[:i], b.Lines[i:j], b.Lines[j:]
}

// changedCount is the number of changed (+/-) lines in a block — the domain over
// which segment ranges are addressed.
func changedCount(b Block) int {
	_, changed, _ := splitBlockLines(b)
	return len(changed)
}

// segLines returns the DiffLines a segment renders: its changed lines, plus the
// block's leading/trailing context only when the segment touches the block's
// start/end. Ranges are clamped defensively so bad input can't panic.
func segLines(b Block, from, to int) []DiffLine {
	before, changed, after := splitBlockLines(b)
	k := len(changed)
	lo, hi := from, to
	if from == 0 {
		lo, hi = 1, k
	}
	if lo < 1 {
		lo = 1
	}
	if hi > k {
		hi = k
	}
	if lo > hi || k == 0 {
		return nil
	}
	var out []DiffLine
	if lo == 1 {
		out = append(out, before...)
	}
	out = append(out, changed[lo-1:hi]...)
	if hi == k {
		out = append(out, after...)
	}
	return out
}
