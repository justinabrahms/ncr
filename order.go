package main

// Outside-in guard (issue #41): the organizer sometimes opens the plan with a
// data-model/migration chapter despite the prompt. Re-sorting every chapter by
// layer would wreck deliberately interleaved narratives (curated plans revisit
// outer layers mid-story), so this only fires when the plan opens inside-out:
// the leading run of chapters that sit strictly deeper than a later chapter is
// demoted, each to just before the first chapter at or inside its own layer.
// Runs between normalize and reconcile, so the auto-repaired "Unplaced"
// chapter is appended afterward and stays last.

// unlayeredDepth stands in for a missing layer: as inside as real code gets,
// so an unlayered chapter never claims the opening slot.
const unlayeredDepth = 5

// chapterOutermost is the minimum layer across a chapter's units; unknown
// units, nil layers, and empty chapters count as unlayeredDepth.
func chapterOutermost(ch Chapter, layerOf map[string]*int) int {
	out := unlayeredDepth
	for _, n := range ch.Nodes {
		l := unlayeredDepth
		if p := layerOf[n.Unit]; p != nil {
			l = *p
		}
		if l < out {
			out = l
		}
	}
	return out
}

// enforceOutsideIn demotes an inside-out opening: the maximal leading run of
// chapters whose outermost layer is strictly deeper than some later chapter's
// moves, in original order, to just before the first surviving chapter at or
// inside its layer (or the end). Everything after the run keeps its order.
// Returns the demoted chapters' titles; nil means the plan was untouched.
func enforceOutsideIn(plan *ReadingPlan) []string {
	if len(plan.Chapters) < 2 {
		return nil
	}
	layerOf := make(map[string]*int, len(plan.Units))
	for i := range plan.Units {
		layerOf[plan.Units[i].ID] = plan.Units[i].Layer
	}
	n := len(plan.Chapters)
	outer := make([]int, n)
	for i, ch := range plan.Chapters {
		outer[i] = chapterOutermost(ch, layerOf)
	}
	suffixMin := make([]int, n+1)
	suffixMin[n] = unlayeredDepth + 1
	for i := n - 1; i >= 0; i-- {
		suffixMin[i] = min(outer[i], suffixMin[i+1])
	}
	k := 0
	for k < n-1 && outer[k] > suffixMin[k+1] {
		k++
	}
	if k == 0 {
		return nil
	}

	type entry struct {
		ch    Chapter
		layer int
		spine bool // survived in place, as opposed to demoted here
	}
	result := make([]entry, 0, n)
	for i := k; i < n; i++ {
		result = append(result, entry{plan.Chapters[i], outer[i], true})
	}
	demoted := make([]string, 0, k)
	for i := 0; i < k; i++ {
		l := outer[i]
		// Skip past more-outside chapters; among equals, spine chapters yield
		// (spec: "just before the first chapter at or inside its layer") but
		// previously demoted equals keep their original relative order.
		j := 0
		for j < len(result) && (result[j].layer < l || (result[j].layer == l && !result[j].spine)) {
			j++
		}
		result = append(result, entry{})
		copy(result[j+1:], result[j:])
		result[j] = entry{plan.Chapters[i], l, false}
		demoted = append(demoted, plan.Chapters[i].Title)
	}
	for i, e := range result {
		plan.Chapters[i] = e.ch
	}
	return demoted
}
