package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Deterministic diff indexer — port of ncr/index.py. Splits a unified diff into
// stable-ID'd change blocks (maximal runs of +/- lines), each with up to
// contextLines surrounding unchanged lines for display. text/sha cover only the
// changed lines, so coverage accounting == changed lines. See docs/completeness.md.

const contextLines = 3

var hunkRe = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(.*)$`)

type fileDiff struct {
	path, changeType string
	body             []string
}

type rec struct {
	kind         string // ctx | add | del | hunk
	raw          string
	oldNo, newNo int
}

func shaOf(text string) string {
	h := sha256.Sum256([]byte(text))
	return "sha256:" + hex.EncodeToString(h[:])
}

func cleanPath(newPath, oldPath string) string {
	p := newPath
	if p == "" || p == "/dev/null" {
		p = oldPath
	}
	if p == "" {
		p = "?"
	}
	for _, pre := range []string{"a/", "b/"} {
		if strings.HasPrefix(p, pre) {
			return p[len(pre):]
		}
	}
	return p
}

// splitFiles is stateful like git's own diff parser: `---` / `+++` / mode lines
// are only recognized as file headers while between a `diff --git` header and the
// file's first hunk (`@@`). Once inside a hunk body every line is content, so a
// changed line whose content starts with `-- ` or `++ ` (SQL/Lua/Haskell comments,
// `++ [x]`, etc.) is preserved instead of being misparsed as a header. See #5.
func splitFiles(diff string) []fileDiff {
	var files []fileDiff
	cur := -1
	oldPath, changeType := "", "modified"
	inHunk := false // true once the current file's first hunk has started
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git"):
			cur, oldPath, changeType, inHunk = -1, "", "modified", false
		case !inHunk && strings.HasPrefix(line, "new file mode"):
			changeType = "added"
		case !inHunk && strings.HasPrefix(line, "deleted file mode"):
			changeType = "deleted"
		case !inHunk && (strings.HasPrefix(line, "rename from") || strings.HasPrefix(line, "rename to")):
			changeType = "renamed"
		case !inHunk && strings.HasPrefix(line, "--- "):
			oldPath = strings.TrimSpace(line[4:])
		case !inHunk && strings.HasPrefix(line, "+++ "):
			files = append(files, fileDiff{path: cleanPath(strings.TrimSpace(line[4:]), oldPath), changeType: changeType})
			cur = len(files) - 1
		default:
			if hunkRe.MatchString(line) {
				inHunk = true
			}
			if cur >= 0 {
				files[cur].body = append(files[cur].body, line)
			}
		}
	}
	return files
}

func records(body []string) []rec {
	var recs []rec
	oldNo, newNo := 0, 0
	for _, line := range body {
		if m := hunkRe.FindStringSubmatch(line); m != nil {
			oldNo, _ = strconv.Atoi(m[1])
			newNo, _ = strconv.Atoi(m[3])
			recs = append(recs, rec{kind: "hunk", raw: line})
			continue
		}
		tag := byte(' ')
		if len(line) > 0 {
			tag = line[0]
		}
		switch tag {
		case '+':
			recs = append(recs, rec{kind: "add", raw: line, oldNo: oldNo, newNo: newNo})
			newNo++
		case '-':
			recs = append(recs, rec{kind: "del", raw: line, oldNo: oldNo, newNo: newNo})
			oldNo++
		case '\\':
			// "\ No newline at end of file" — not a code line
		default:
			recs = append(recs, rec{kind: "ctx", raw: line, oldNo: oldNo, newNo: newNo})
			oldNo++
			newNo++
		}
	}
	return recs
}

func neighborRecs(recs []rec, start, step int) []rec {
	var out []rec
	for k := start; k >= 0 && k < len(recs) && recs[k].kind == "ctx" && len(out) < contextLines; k += step {
		out = append(out, recs[k])
	}
	if step < 0 {
		for l, r := 0, len(out)-1; l < r; l, r = l+1, r-1 {
			out[l], out[r] = out[r], out[l]
		}
	}
	return out
}

// content strips a diff line's +/-/space marker.
func content(raw string) string {
	if raw == "" {
		return ""
	}
	return raw[1:]
}

// declStart reports whether a line begins a new top-level construct: column 0 (a
// function/type body is indented, so this is never inside one) and not a closer.
func declStart(code string) bool {
	if code == "" || code[0] == ' ' || code[0] == '\t' {
		return false
	}
	switch code[0] {
	case '}', ')', ']', ',', ';':
		return false
	}
	return true
}

// declEnds reports whether a line ends the previous construct — blank, or a
// column-0 closing brace/paren.
func declEnds(code string) bool {
	if strings.TrimSpace(code) == "" {
		return true
	}
	return code[0] == '}' || code[0] == ')'
}

func hasRealContent(part []rec) bool {
	for _, r := range part {
		if strings.TrimSpace(content(r.raw)) != "" {
			return true
		}
	}
	return false
}

// splitRunAtDecls breaks a run of changed lines at top-level declaration
// boundaries, so a block that adds several functions becomes one block per
// function. Deterministic and conservative: it only cuts where a new column-0
// declaration begins right after the previous one ended, so it can never split a
// single function (whose body is indented). Worst case it under-splits.
func splitRunAtDecls(run []rec) [][]rec {
	var parts [][]rec
	var cur []rec
	for i := range run {
		if i > 0 && hasRealContent(cur) &&
			declStart(content(run[i].raw)) && declEnds(content(run[i-1].raw)) {
			parts = append(parts, cur)
			cur = nil
		}
		cur = append(cur, run[i])
	}
	if len(cur) > 0 {
		parts = append(parts, cur)
	}
	if len(parts) == 0 {
		return [][]rec{run}
	}
	return parts
}

func diffLineOf(r rec) DiffLine {
	return DiffLine{Kind: r.kind, Text: r.raw, OldNo: r.oldNo, NewNo: r.newNo}
}

func indexDiff(diff string) []Block {
	blocks := []Block{}
	counter := 0
	for _, fd := range splitFiles(diff) {
		recs := records(fd.body)
		header := ""
		for i := 0; i < len(recs); {
			switch recs[i].kind {
			case "hunk":
				header = recs[i].raw
				i++
				continue
			case "ctx":
				i++
				continue
			}
			j := i
			for j < len(recs) && (recs[j].kind == "add" || recs[j].kind == "del") {
				j++
			}
			run := recs[i:j]
			before := neighborRecs(recs, i-1, -1)
			after := neighborRecs(recs, j, 1)
			parts := splitRunAtDecls(run)
			for pi, part := range parts {
				counter++
				var texts []string
				oldLines, newLines := 0, 0
				var oldStart, newStart *int
				for _, x := range part {
					texts = append(texts, x.raw)
					if x.kind == "del" {
						if oldStart == nil {
							v := x.oldNo
							oldStart = &v
						}
						oldLines++
					} else {
						if newStart == nil {
							v := x.newNo
							newStart = &v
						}
						newLines++
					}
				}
				var lines []DiffLine
				if pi == 0 { // leading context only on the first sub-block
					for _, r := range before {
						lines = append(lines, diffLineOf(r))
					}
				}
				for _, r := range part {
					lines = append(lines, diffLineOf(r))
				}
				if pi == len(parts)-1 { // trailing context only on the last
					for _, r := range after {
						lines = append(lines, diffLineOf(r))
					}
				}
				text := strings.Join(texts, "\n")
				blocks = append(blocks, Block{
					BlockID:    fmt.Sprintf("b%03d", counter),
					Path:       fd.path,
					ChangeType: fd.changeType,
					OldStart:   oldStart,
					OldLines:   oldLines,
					NewStart:   newStart,
					NewLines:   newLines,
					Header:     header,
					Text:       text,
					Sha:        shaOf(text),
					Lines:      lines,
				})
			}
			i = j
		}
	}
	return blocks
}

func buildIndex(diff string) Index {
	blocks := indexDiff(diff)
	ids := make([]string, len(blocks))
	for i, b := range blocks {
		ids[i] = b.BlockID
	}
	return Index{Blocks: blocks, BlockIDs: ids}
}
