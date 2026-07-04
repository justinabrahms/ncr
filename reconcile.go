package main

import (
	"fmt"
	"sort"
)

// Deterministic reconciler — port of ncr/reconcile.py. Proves every change block
// is placed (set-equality vs. the index); auto-repairs any miss into a visible
// "Unplaced" chapter so a bad model run can't lose a hunk. See docs/completeness.md.

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func reconcile(plan *ReadingPlan, index Index, commentBlocks []string) {
	byID := map[string]Block{}
	indexSet := map[string]bool{}
	for _, b := range index.Blocks {
		byID[b.BlockID] = b
	}
	for _, id := range index.BlockIDs {
		indexSet[id] = true
	}

	placed := blockCounts(plan)

	var missing []string
	for _, id := range index.BlockIDs {
		if placed[id] == 0 {
			missing = append(missing, id)
		}
	}
	var duplicated, dangling []string
	for b, n := range placed {
		if n > 1 {
			duplicated = append(duplicated, b)
		}
		if !indexSet[b] {
			dangling = append(dangling, b)
		}
	}
	sort.Strings(duplicated)
	sort.Strings(dangling)

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
	for _, b := range commentBlocks {
		if placed[b] == 0 {
			commentGaps = append(commentGaps, b)
		}
	}

	repaired := false
	if len(missing) > 0 {
		autoRepair(plan, missing, byID)
		repaired = true
		placed = blockCounts(plan)
	}

	placedInIndex := 0
	for _, id := range index.BlockIDs {
		if placed[id] > 0 {
			placedInIndex++
		}
	}

	cov := &Coverage{
		OK: len(missing) == 0 && len(duplicated) == 0 && len(dangling) == 0 &&
			len(unplacedUnits) == 0 && len(commentGaps) == 0,
		Missing:       nonNil(missing),
		Duplicated:    nonNil(duplicated),
		UnplacedUnits: nonNil(unplacedUnits),
		DanglingRefs:  nonNil(dangling),
		CommentGaps:   nonNil(commentGaps),
		Repaired:      repaired,
	}
	cov.Counts.Indexed = len(index.BlockIDs)
	cov.Counts.Placed = placedInIndex
	plan.Coverage = cov
}

func blockCounts(plan *ReadingPlan) map[string]int {
	c := map[string]int{}
	for _, u := range plan.Units {
		for _, b := range u.Blocks {
			c[b]++
		}
	}
	return c
}

func autoRepair(plan *ReadingPlan, missing []string, byID map[string]Block) {
	existing := map[string]bool{}
	for _, u := range plan.Units {
		existing[u.ID] = true
	}
	layer5 := 5
	var nodes []Node
	n := 0
	for _, bid := range missing {
		n++
		id := fmt.Sprintf("u-unplaced-%d", n)
		for existing[id] {
			n++
			id = fmt.Sprintf("u-unplaced-%d", n)
		}
		existing[id] = true
		blk := byID[bid]
		file := blk.Path
		if file == "" {
			file = "?"
		}
		plan.Units = append(plan.Units, Unit{
			ID:          id,
			File:        file,
			Layer:       &layer5,
			LayerReason: "auto-repair: organizer did not place this block",
			Uncertain:   true,
			Summary:     fmt.Sprintf("Block %s the organizer left unplaced — shown here directly.", bid),
			Blocks:      []string{bid},
		})
		nodes = append(nodes, Node{Unit: id, Depth: 0})
	}
	plan.Chapters = append(plan.Chapters, Chapter{
		ID:      "c-unplaced",
		Title:   "⚠ Unplaced changes",
		Summary: fmt.Sprintf("%d change block(s) the organizer could not place. Shown verbatim so nothing is lost.", len(missing)),
		Nodes:   nodes,
	})
}
