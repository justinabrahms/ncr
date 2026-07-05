package main

import (
	"fmt"
	"sort"
	"strings"
)

// Deterministic reconciler — the completeness guarantee, now at LINE granularity so
// the narrative may split a block into sub-ranges (docs/completeness.md). It proves
// every changed line of every block is covered by exactly one unit; gaps are
// auto-repaired into a visible "Unplaced" chapter, overlaps are flagged.

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func reconcile(plan *ReadingPlan, index Index, commentBlocks []string) {
	byID := map[string]Block{}
	kOf := map[string]int{} // changed-line count per block
	for _, b := range index.Blocks {
		byID[b.BlockID] = b
		kOf[b.BlockID] = changedCount(b)
	}

	cover, dangling := computeCover(plan, kOf)
	missing := coverageGaps(index, kOf, cover, func(c int) bool { return c == 0 })
	duplicated := coverageGaps(index, kOf, cover, func(c int) bool { return c > 1 })

	unitIDs := map[string]bool{}
	for _, u := range plan.Units {
		unitIDs[u.ID] = true
	}
	placement := map[string]int{}
	for _, ch := range plan.Chapters {
		for _, n := range ch.Nodes {
			placement[n.Unit]++
		}
	}
	for _, o := range plan.Orphans {
		for _, id := range o.Units {
			placement[id]++
		}
	}
	var unplacedUnits []string
	for id := range unitIDs {
		if placement[id] == 0 {
			unplacedUnits = append(unplacedUnits, id)
		}
	}
	sort.Strings(unplacedUnits)

	var commentGaps []string
	for _, cb := range commentBlocks {
		if !anyCovered(cover[cb]) {
			commentGaps = append(commentGaps, cb)
		}
	}

	ok := len(missing) == 0 && len(duplicated) == 0 && len(unplacedUnits) == 0 &&
		len(dangling) == 0 && len(commentGaps) == 0

	repaired := false
	if len(missing) > 0 {
		autoRepair(plan, missing, byID)
		repaired = true
		cover, _ = computeCover(plan, kOf)
	}

	placed := 0
	for _, b := range index.Blocks {
		if !hasGap(cover[b.BlockID], kOf[b.BlockID]) {
			placed++
		}
	}

	cov := &Coverage{
		OK:            ok,
		Missing:       nonNil(missing),
		Duplicated:    nonNil(duplicated),
		UnplacedUnits: nonNil(unplacedUnits),
		DanglingRefs:  nonNil(dangling),
		CommentGaps:   nonNil(commentGaps),
		Repaired:      repaired,
	}
	cov.Counts.Indexed = len(index.Blocks)
	cov.Counts.Placed = placed
	plan.Coverage = cov
}

// coverageIssues returns a human-readable phrase for each failed sub-check of a
// Coverage, in a stable order. A block being "missing" is auto-repaired into the
// Unplaced chapter, so it is described separately (see hasUnplacedChapter). The
// slice is empty when cov.OK is true.
func coverageIssues(cov *Coverage) []string {
	var out []string
	if n := len(cov.Missing); n > 0 {
		out = append(out, fmt.Sprintf("%d unplaced (auto-repaired)", n))
	}
	if n := len(cov.DanglingRefs); n > 0 {
		out = append(out, fmt.Sprintf("%d dangling block ref%s", n, plural(n)))
	}
	if n := len(cov.Duplicated); n > 0 {
		out = append(out, fmt.Sprintf("%d duplicated block%s", n, plural(n)))
	}
	if n := len(cov.UnplacedUnits); n > 0 {
		out = append(out, fmt.Sprintf("%d unplaced unit%s", n, plural(n)))
	}
	if n := len(cov.CommentGaps); n > 0 {
		out = append(out, fmt.Sprintf("%d comment gap%s", n, plural(n)))
	}
	return out
}

// coverageStatus builds the one-line "⚠ …" status describing why coverage is
// not OK. Returns "✓ complete" when cov.OK.
func coverageStatus(cov *Coverage) string {
	if cov.OK {
		return "✓ complete"
	}
	issues := coverageIssues(cov)
	if len(issues) == 0 {
		return "⚠ coverage incomplete"
	}
	return "⚠ " + strings.Join(issues, ", ")
}

// hasUnplacedChapter reports whether a visible "Unplaced" chapter was created,
// which only happens when there were missing blocks to auto-repair.
func hasUnplacedChapter(cov *Coverage) bool {
	return len(cov.Missing) > 0
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// computeCover tallies, per block, how many units cover each changed line. Bad
// segments (unknown block, out-of-range) are collected as dangling.
func computeCover(plan *ReadingPlan, kOf map[string]int) (map[string][]int, []string) {
	cover := map[string][]int{}
	var dangling []string
	for _, u := range plan.Units {
		for _, seg := range u.Blocks {
			id, from, to, okSeg := parseSegment(seg)
			k, exists := kOf[id]
			if !okSeg || !exists {
				dangling = append(dangling, seg)
				continue
			}
			lo, hi := from, to
			if from == 0 {
				lo, hi = 1, k
			}
			if lo < 1 || hi > k {
				dangling = append(dangling, seg)
				if lo < 1 {
					lo = 1
				}
				if hi > k {
					hi = k
				}
			}
			if lo > hi {
				continue
			}
			if cover[id] == nil {
				cover[id] = make([]int, k)
			}
			for i := lo - 1; i < hi; i++ {
				cover[id][i]++
			}
		}
	}
	return cover, dangling
}

// coverageGaps returns the line ranges (in index order) where pred(count) holds,
// formatted as "id" for a whole block or "id:a-b" for a partial range.
func coverageGaps(index Index, kOf map[string]int, cover map[string][]int, pred func(int) bool) []string {
	var out []string
	for _, b := range index.Blocks {
		id, k, arr := b.BlockID, kOf[b.BlockID], cover[b.BlockID]
		at := func(i int) int {
			if arr == nil {
				return 0
			}
			return arr[i]
		}
		for i := 0; i < k; {
			if !pred(at(i)) {
				i++
				continue
			}
			j := i
			for j < k && pred(at(j)) {
				j++
			}
			if i == 0 && j == k {
				out = append(out, id)
			} else {
				out = append(out, fmt.Sprintf("%s:%d-%d", id, i+1, j))
			}
			i = j
		}
	}
	return out
}

func hasGap(arr []int, k int) bool {
	for i := 0; i < k; i++ {
		if arr == nil || arr[i] == 0 {
			return true
		}
	}
	return false
}

func anyCovered(arr []int) bool {
	for _, c := range arr {
		if c > 0 {
			return true
		}
	}
	return false
}

func autoRepair(plan *ReadingPlan, missing []string, byID map[string]Block) {
	existing := map[string]bool{}
	for _, u := range plan.Units {
		existing[u.ID] = true
	}
	layer5 := 5
	var nodes []Node
	n := 0
	for _, seg := range missing {
		n++
		id := fmt.Sprintf("u-unplaced-%d", n)
		for existing[id] {
			n++
			id = fmt.Sprintf("u-unplaced-%d", n)
		}
		existing[id] = true
		bid, _, _, _ := parseSegment(seg)
		file := byID[bid].Path
		if file == "" {
			file = "?"
		}
		plan.Units = append(plan.Units, Unit{
			ID:          id,
			File:        file,
			Layer:       &layer5,
			LayerReason: "auto-repair: organizer did not place these lines",
			Uncertain:   true,
			Summary:     fmt.Sprintf("%s — lines the organizer left unplaced; shown here directly.", seg),
			Blocks:      []string{seg},
		})
		nodes = append(nodes, Node{Unit: id, Depth: 0})
	}
	plan.Chapters = append(plan.Chapters, Chapter{
		ID:      "c-unplaced",
		Title:   "⚠ Unplaced changes",
		Summary: fmt.Sprintf("%d change segment(s) the organizer could not place. Shown verbatim so nothing is lost.", len(missing)),
		Nodes:   nodes,
	})
}
