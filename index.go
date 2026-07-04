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

func splitFiles(diff string) []fileDiff {
	var files []fileDiff
	cur := -1
	oldPath, changeType := "", "modified"
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git"):
			cur, oldPath, changeType = -1, "", "modified"
		case strings.HasPrefix(line, "new file mode"):
			changeType = "added"
		case strings.HasPrefix(line, "deleted file mode"):
			changeType = "deleted"
		case strings.HasPrefix(line, "rename from"), strings.HasPrefix(line, "rename to"):
			changeType = "renamed"
		case strings.HasPrefix(line, "--- "):
			oldPath = strings.TrimSpace(line[4:])
		case strings.HasPrefix(line, "+++ "):
			files = append(files, fileDiff{path: cleanPath(strings.TrimSpace(line[4:]), oldPath), changeType: changeType})
			cur = len(files) - 1
		default:
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
			recs = append(recs, rec{kind: "add", raw: line, newNo: newNo})
			newNo++
		case '-':
			recs = append(recs, rec{kind: "del", raw: line, oldNo: oldNo})
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

func neighbors(recs []rec, start, step int) []string {
	var out []string
	for k := start; k >= 0 && k < len(recs) && recs[k].kind == "ctx" && len(out) < contextLines; k += step {
		out = append(out, recs[k].raw)
	}
	if step < 0 {
		for l, r := 0, len(out)-1; l < r; l, r = l+1, r-1 {
			out[l], out[r] = out[r], out[l]
		}
	}
	return out
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
			counter++
			var texts []string
			oldLines, newLines := 0, 0
			var oldStart, newStart *int
			for _, x := range run {
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
			text := strings.Join(texts, "\n")
			blocks = append(blocks, Block{
				BlockID:       fmt.Sprintf("b%03d", counter),
				Path:          fd.path,
				ChangeType:    fd.changeType,
				OldStart:      oldStart,
				OldLines:      oldLines,
				NewStart:      newStart,
				NewLines:      newLines,
				Header:        header,
				Text:          text,
				Sha:           shaOf(text),
				ContextBefore: neighbors(recs, i-1, -1),
				ContextAfter:  neighbors(recs, j, 1),
			})
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
