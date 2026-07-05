package main

import (
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func sampleDiff(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("tests/fixtures/sample.diff")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestIDsStableAndGapFree(t *testing.T) {
	idx := buildIndex(sampleDiff(t))
	for i, id := range idx.BlockIDs {
		if want := fmt.Sprintf("b%03d", i+1); id != want {
			t.Fatalf("id[%d] = %q, want %q", i, id, want)
		}
	}
	if !reflect.DeepEqual(buildIndex(sampleDiff(t)).BlockIDs, idx.BlockIDs) {
		t.Fatal("block ids not deterministic")
	}
}

func TestEveryChangedLineCoveredOnce(t *testing.T) {
	diff := sampleDiff(t)
	var covered []string
	for _, b := range indexDiff(diff) {
		for _, l := range strings.Split(b.Text, "\n") {
			if l != "" && (l[0] == '+' || l[0] == '-') {
				covered = append(covered, l)
			}
		}
	}
	var expected []string
	for _, l := range strings.Split(diff, "\n") {
		if l != "" && (l[0] == '+' || l[0] == '-') &&
			!strings.HasPrefix(l, "+++") && !strings.HasPrefix(l, "---") {
			expected = append(expected, l)
		}
	}
	sort.Strings(covered)
	sort.Strings(expected)
	if !reflect.DeepEqual(covered, expected) {
		t.Fatalf("changed lines not covered once:\n got %v\nwant %v", covered, expected)
	}
}

func TestEmptyDiffYieldsZeroBlocks(t *testing.T) {
	for _, in := range []string{"", "\n", "   \n"} {
		idx := buildIndex(in)
		if len(idx.Blocks) != 0 || len(idx.BlockIDs) != 0 {
			t.Fatalf("empty diff %q yielded %d blocks / %d ids, want 0/0",
				in, len(idx.Blocks), len(idx.BlockIDs))
		}
	}
}

func TestPlaceBlockCoords(t *testing.T) {
	for _, b := range indexDiff(sampleDiff(t)) {
		if strings.Contains(b.Text, "func (h *OrderHandler) Place") {
			if b.NewStart == nil || *b.NewStart != 22 {
				t.Fatalf("Place newStart = %v, want 22", b.NewStart)
			}
			if b.OldLines != 0 || b.NewLines != 14 {
				t.Fatalf("Place lines old=%d new=%d, want 0/14", b.OldLines, b.NewLines)
			}
			return
		}
	}
	t.Fatal("Place block not found")
}

func TestNewFileBlocksHaveNoOldSide(t *testing.T) {
	found := false
	for _, b := range indexDiff(sampleDiff(t)) {
		if b.Path == "internal/order/service.go" {
			found = true
			if b.OldStart != nil || b.OldLines != 0 || b.ChangeType != "added" {
				t.Fatalf("new-file block has old side: %+v", b)
			}
		}
	}
	if !found {
		t.Fatal("service.go blocks not found")
	}
}
