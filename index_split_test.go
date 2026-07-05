package main

import (
	"strings"
	"testing"
)

const twoFuncDiff = `diff --git a/x.go b/x.go
new file mode 100644
index 0000000..1111111
--- /dev/null
+++ b/x.go
@@ -0,0 +1,9 @@
+package x
+
+func Foo() int {
+	return 1
+}
+
+func Bar() int {
+	return 2
+}
`

func blockText(b Block) string {
	var sb strings.Builder
	for _, l := range b.Lines {
		if l.Kind != "ctx" {
			sb.WriteString(l.Text + "\n")
		}
	}
	return sb.String()
}

// A block adding several functions is split into one block per function — and a
// single function is never split (its indented body stays whole).
func TestIndexSplitsAtFunctionBoundaries(t *testing.T) {
	blocks := indexDiff(twoFuncDiff)

	var foo, bar *Block
	for i := range blocks {
		txt := blockText(blocks[i])
		if strings.Contains(txt, "func Foo") {
			foo = &blocks[i]
		}
		if strings.Contains(txt, "func Bar") {
			bar = &blocks[i]
		}
	}
	if foo == nil || bar == nil {
		t.Fatalf("expected separate Foo and Bar blocks, got %d blocks", len(blocks))
	}
	if foo == bar {
		t.Fatal("Foo and Bar must be different blocks")
	}
	// each function is whole (declaration + body), not split mid-function
	if ft := blockText(*foo); !strings.Contains(ft, "return 1") || strings.Contains(ft, "func Bar") {
		t.Fatalf("Foo block should hold its whole body and only Foo: %q", ft)
	}
	if bt := blockText(*bar); !strings.Contains(bt, "return 2") {
		t.Fatalf("Bar block should hold its whole body: %q", bt)
	}
}

// A deleted line whose content starts with "-- " (SQL/Lua/Haskell comment) shows
// up as "--- ..." in a unified diff. It must be preserved as a changed line, not
// consumed as a file header. Symmetric case: an added "++ ..." line -> "+++ ...".
// Regression for #5.
func TestIndexPreservesCommentLikeChangedLines(t *testing.T) {
	diff := `diff --git a/schema.sql b/schema.sql
index 1111111..2222222 100644
--- a/schema.sql
+++ b/schema.sql
@@ -1,4 +1,3 @@
 CREATE TABLE t (
--- drop this comment
-  old_col INT,
+  new_col INT,
);
`
	blocks := indexDiff(diff)

	var all strings.Builder
	paths := map[string]bool{}
	for _, b := range blocks {
		paths[b.Path] = true
		all.WriteString(b.Text + "\n")
	}
	if len(paths) != 1 || !paths["schema.sql"] {
		t.Fatalf("expected exactly one file schema.sql, got paths %v", paths)
	}
	if !strings.Contains(all.String(), "-- drop this comment") {
		t.Fatalf("deleted comment line was dropped; block text: %q", all.String())
	}

	// symmetric added-line case: adding Haskell content "++ [x]" appears in the
	// diff as "+++ [x]" and must not be misparsed as a "+++ " file header.
	addDiff := `diff --git a/List.hs b/List.hs
--- a/List.hs
+++ b/List.hs
@@ -1,2 +1,3 @@
 xs = base
+++ [x]
 done
`
	var addAll strings.Builder
	addPaths := map[string]bool{}
	for _, b := range indexDiff(addDiff) {
		addPaths[b.Path] = true
		addAll.WriteString(b.Text + "\n")
	}
	if len(addPaths) != 1 || !addPaths["List.hs"] {
		t.Fatalf("expected exactly one file List.hs, got paths %v", addPaths)
	}
	if !strings.Contains(addAll.String(), "++ [x]") {
		t.Fatalf("added ++ line was dropped or split; block text: %q", addAll.String())
	}
}

// Changes wholly inside one function (indented) must NOT be split.
func TestIndexDoesNotSplitInsideFunction(t *testing.T) {
	diff := `diff --git a/y.go b/y.go
--- a/y.go
+++ b/y.go
@@ -1,5 +1,5 @@
 func Foo() int {
-	x := 1
-	return x
+	x := 2
+	return x + 1
 }
`
	n := 0
	for _, b := range indexDiff(diff) {
		if b.Path == "y.go" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("indented in-function change split into %d blocks, want 1", n)
	}
}
