package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReorderArgsBaseIsValueFlag(t *testing.T) {
	// --base takes a value, so the ref that follows it must land in flags, not
	// be mistaken for the "." positional.
	flags, pos := reorderArgs([]string{".", "--base", "develop"})
	if len(pos) != 1 || pos[0] != "." {
		t.Fatalf("positionals = %v, want [.]", pos)
	}
	want := []string{"--base", "develop"}
	if strings.Join(flags, " ") != strings.Join(want, " ") {
		t.Fatalf("flags = %v, want %v", flags, want)
	}
}

func TestChangedPaths(t *testing.T) {
	got := changedPaths("main.go\n\n  ingest.go \n")
	want := []string{"main.go", "ingest.go"}
	if len(got) != len(want) {
		t.Fatalf("changedPaths = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("changedPaths[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestReadWorkingTreeFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hi there"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bin.dat"), []byte{0xff, 0xfe, 0x00, 0x01}, 0o644); err != nil {
		t.Fatal(err)
	}

	if got := readWorkingTreeFile(root, "hello.txt"); got != "hi there" {
		t.Fatalf("text file = %q, want %q", got, "hi there")
	}
	if got := readWorkingTreeFile(root, "bin.dat"); got != "(binary file omitted)" {
		t.Fatalf("binary file = %q, want binary-omitted marker", got)
	}
	if got := readWorkingTreeFile(root, "missing.txt"); got != "" {
		t.Fatalf("missing file = %q, want empty", got)
	}
}
