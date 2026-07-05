package main

import (
	"strings"
	"testing"
)

func TestParsePRFilesSinglePage(t *testing.T) {
	out := `[{"filename":"a.go","status":"modified"},{"filename":"b.go","status":"added"}]`
	paths, err := parsePRFiles(out)
	if err != nil {
		t.Fatalf("parsePRFiles: %v", err)
	}
	want := []string{"a.go", "b.go"}
	if len(paths) != len(want) {
		t.Fatalf("got %v, want %v", paths, want)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("paths[%d] = %q, want %q", i, paths[i], want[i])
		}
	}
}

// gh api --paginate concatenates one JSON array per page (…][…]); parsePRFiles
// must recover files from every page, not just the first.
func TestParsePRFilesMultiPage(t *testing.T) {
	out := `[{"filename":"a.go"},{"filename":"b.go"}]` +
		`[{"filename":"c.go"},{"filename":"d.go"}]` + "\n" +
		`[{"filename":"e.go"}]`
	paths, err := parsePRFiles(out)
	if err != nil {
		t.Fatalf("parsePRFiles: %v", err)
	}
	if got := strings.Join(paths, ","); got != "a.go,b.go,c.go,d.go,e.go" {
		t.Fatalf("multi-page parse = %q", got)
	}
}

func TestParsePRFilesEmpty(t *testing.T) {
	paths, err := parsePRFiles(`[]`)
	if err != nil {
		t.Fatalf("parsePRFiles: %v", err)
	}
	if len(paths) != 0 {
		t.Fatalf("expected no paths, got %v", paths)
	}
}
