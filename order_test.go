package main

import (
	"encoding/json"
	"reflect"
	"testing"
)

// orderPlan builds a plan of single-unit chapters, one per layer value; a nil
// entry makes a unit with no layer.
func orderPlan(layers ...*int) ReadingPlan {
	var p ReadingPlan
	for i, l := range layers {
		id := string(rune('a' + i))
		p.Units = append(p.Units, Unit{ID: "u-" + id, Layer: l, Blocks: []string{}})
		p.Chapters = append(p.Chapters, Chapter{
			ID:    "c-" + id,
			Title: "ch-" + id,
			Nodes: []Node{{Unit: "u-" + id}},
		})
	}
	return p
}

func lp(l int) *int { return &l }

func chapterIDs(p ReadingPlan) []string {
	var ids []string
	for _, ch := range p.Chapters {
		ids = append(ids, ch.ID)
	}
	return ids
}

func TestInsideOutOpeningIsDemoted(t *testing.T) {
	// Observed failure shape (gitea#37500): model/migration chapter first,
	// entrypoints after. The layer-4 opener lands just before the first
	// chapter at or inside layer 4 (the tests chapter).
	p := orderPlan(lp(4), lp(0), lp(1), lp(6))
	demoted := enforceOutsideIn(&p)
	if !reflect.DeepEqual(demoted, []string{"ch-a"}) {
		t.Fatalf("demoted = %v", demoted)
	}
	if got := chapterIDs(p); !reflect.DeepEqual(got, []string{"c-b", "c-c", "c-a", "c-d"}) {
		t.Fatalf("chapter order = %v", got)
	}
}

func TestInsideOutRunOfTwoKeepsRelativeOrder(t *testing.T) {
	p := orderPlan(lp(4), lp(4), lp(0))
	demoted := enforceOutsideIn(&p)
	if !reflect.DeepEqual(demoted, []string{"ch-a", "ch-b"}) {
		t.Fatalf("demoted = %v", demoted)
	}
	if got := chapterIDs(p); !reflect.DeepEqual(got, []string{"c-c", "c-a", "c-b"}) {
		t.Fatalf("chapter order = %v", got)
	}
}

func TestMonotonicPlanUntouched(t *testing.T) {
	p := orderPlan(lp(0), lp(1), lp(3), lp(3), lp(6))
	before, _ := json.Marshal(p.Chapters)
	if demoted := enforceOutsideIn(&p); demoted != nil {
		t.Fatalf("demoted = %v", demoted)
	}
	after, _ := json.Marshal(p.Chapters)
	if string(before) != string(after) {
		t.Fatalf("chapters changed:\n%s\n%s", before, after)
	}
}

func TestInterleavedButOutsideFirstUntouched(t *testing.T) {
	// A deliberately interleaved narrative (the supabase demo shape) revisits
	// outer layers mid-plan; as long as nothing later is more outside than the
	// opening chapter, the order stands.
	p := orderPlan(lp(1), lp(3), lp(4), lp(2), lp(2), lp(6))
	before, _ := json.Marshal(p.Chapters)
	if demoted := enforceOutsideIn(&p); demoted != nil {
		t.Fatalf("demoted = %v", demoted)
	}
	after, _ := json.Marshal(p.Chapters)
	if string(before) != string(after) {
		t.Fatalf("chapters changed:\n%s\n%s", before, after)
	}
}

func TestNilLayersAndTiesDontPanic(t *testing.T) {
	// All-nil layers tie at the default depth: nothing later is strictly more
	// outside, so nothing moves.
	p := orderPlan(nil, nil, nil)
	if demoted := enforceOutsideIn(&p); demoted != nil {
		t.Fatalf("demoted = %v", demoted)
	}
	// A nil-layer opener (treated as 5) is deeper than a layer-1 chapter.
	p = orderPlan(nil, lp(1), lp(5))
	if demoted := enforceOutsideIn(&p); !reflect.DeepEqual(demoted, []string{"ch-a"}) {
		t.Fatalf("demoted = %v", demoted)
	}
	if got := chapterIDs(p); !reflect.DeepEqual(got, []string{"c-b", "c-a", "c-c"}) {
		t.Fatalf("chapter order = %v", got)
	}
	// Chapters with no nodes and nodes pointing at unknown units.
	p = ReadingPlan{Chapters: []Chapter{
		{ID: "c-1", Nodes: nil},
		{ID: "c-2", Nodes: []Node{{Unit: "u-missing"}}},
	}}
	if demoted := enforceOutsideIn(&p); demoted != nil {
		t.Fatalf("demoted = %v", demoted)
	}
	// Single chapter.
	p = orderPlan(lp(4))
	if demoted := enforceOutsideIn(&p); demoted != nil {
		t.Fatalf("demoted = %v", demoted)
	}
}

func TestUnplacedChapterStaysLastAfterGuard(t *testing.T) {
	idx := buildIndex(sampleDiff(t))
	plan := samplePlan(t)
	// Invert the opening: move the last chapter (deepest) to the front, then
	// drop unit u4 so reconcile has to append the Unplaced chapter.
	chs := plan.Chapters
	plan.Chapters = append([]Chapter{chs[len(chs)-1]}, chs[:len(chs)-1]...)
	var kept []Unit
	for _, u := range plan.Units {
		if u.ID != "u4" {
			kept = append(kept, u)
		}
	}
	plan.Units = kept
	plan.Orphans = nil

	if demoted := enforceOutsideIn(&plan); len(demoted) == 0 {
		t.Fatal("expected the inverted opening to be demoted")
	}
	reconcile(&plan, idx, nil)
	if !plan.Coverage.Repaired {
		t.Fatalf("expected auto-repair: %+v", plan.Coverage)
	}
	last := plan.Chapters[len(plan.Chapters)-1]
	if last.ID != "c-unplaced" {
		t.Fatalf("Unplaced chapter not last: %v", chapterIDs(plan))
	}
}
