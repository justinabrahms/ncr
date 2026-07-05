package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Ingest a GitHub PR via the `gh` CLI (reuses the user's auth) — port of
// ncr/ingest.py. Read-only. See docs/ingest.md.

const maxFileBytes = 200_000

// largePRFiles is the changed-file count above which we note that file context
// is large. It matches the REST API's per-page size, the boundary at which the
// old `gh pr view --json files` query silently truncated the list.
const largePRFiles = 100

func gh(args ...string) (string, error) {
	cmd := exec.Command("gh", args...)
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("gh %s failed: %s", strings.Join(args, " "), strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

func repoSlug(repo string) (string, error) {
	if repo != "" {
		return repo, nil
	}
	out, err := gh("repo", "view", "--json", "nameWithOwner")
	if err != nil {
		return "", err
	}
	var v struct {
		NameWithOwner string `json:"nameWithOwner"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		return "", err
	}
	return v.NameWithOwner, nil
}

func git(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s failed: %s", strings.Join(args, " "), strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

// getLocalContext builds a PRContext from the working tree instead of GitHub:
// the diff of the current branch against its merge-base with base, plus the
// working-tree contents of every changed file. No GitHub round-trip — this backs
// `ncr .` for pre-PR self-review. Comments are always empty (there's no PR).
func getLocalContext(base string) (PRContext, error) {
	var ctx PRContext
	root, err := git("rev-parse", "--show-toplevel")
	if err != nil {
		return ctx, err
	}
	root = strings.TrimSpace(root)
	mergeBase, err := git("merge-base", base, "HEAD")
	if err != nil {
		return ctx, err
	}
	mergeBase = strings.TrimSpace(mergeBase)
	if ctx.Diff, err = git("diff", mergeBase+"...HEAD"); err != nil {
		return ctx, err
	}
	ctx.Meta = Meta{Title: fmt.Sprintf("%s...HEAD (local)", base)}
	ctx.Files = map[string]string{}
	names, err := git("diff", "--name-only", mergeBase+"...HEAD")
	if err != nil {
		return ctx, err
	}
	for _, path := range changedPaths(names) {
		ctx.Files[path] = readWorkingTreeFile(root, path)
	}
	return ctx, nil
}

// changedPaths splits `git diff --name-only` output into non-empty paths.
func changedPaths(nameOnly string) []string {
	var paths []string
	for _, line := range strings.Split(nameOnly, "\n") {
		if p := strings.TrimSpace(line); p != "" {
			paths = append(paths, p)
		}
	}
	return paths
}

// readWorkingTreeFile reads a changed file's current on-disk contents, mirroring
// fetchFile's size/binary guards so the model sees the same shape of context it
// would for a GitHub PR.
func readWorkingTreeFile(root, path string) string {
	raw, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		return ""
	}
	if len(raw) > maxFileBytes {
		return fmt.Sprintf("(file too large: %d bytes; omitted from context)", len(raw))
	}
	if !utf8.Valid(raw) {
		return "(binary file omitted)"
	}
	return string(raw)
}

func getPRContext(pr int, repo string, fetchFiles bool) (PRContext, error) {
	var ctx PRContext
	repo, err := repoSlug(repo)
	if err != nil {
		return ctx, err
	}
	n := strconv.Itoa(pr)

	if ctx.Diff, err = gh("pr", "diff", n, "--repo", repo); err != nil {
		return ctx, err
	}
	metaOut, err := gh("pr", "view", n, "--repo", repo,
		"--json", "title,body,number,headRefOid,baseRefName,headRefName,author,files")
	if err != nil {
		return ctx, err
	}
	if err := json.Unmarshal([]byte(metaOut), &ctx.Meta); err != nil {
		return ctx, err
	}
	commentsOut, err := gh("api", fmt.Sprintf("repos/%s/pulls/%d/comments", repo, pr), "--paginate")
	if err != nil {
		return ctx, err
	}
	if strings.TrimSpace(commentsOut) != "" {
		_ = json.Unmarshal([]byte(commentsOut), &ctx.Comments)
	}

	ctx.Files = map[string]string{}
	if fetchFiles {
		// Page the full changed-file list via the REST API. `gh pr view --json
		// files` (used above for Meta) is backed by a GraphQL connection that
		// caps at 100, silently truncating file context on large PRs (#13).
		paths, err := getPRFiles(repo, pr)
		if err != nil {
			return ctx, err
		}
		if len(paths) >= largePRFiles {
			logf("large PR: fetching context for %d changed files", len(paths))
		} else {
			logf("fetching context for %d changed files", len(paths))
		}
		for _, p := range paths {
			if p == "" {
				continue
			}
			ctx.Files[p] = fetchFile(repo, p, ctx.Meta.HeadRefOid)
		}
	}
	return ctx, nil
}

// getPRFiles returns every changed file path for a PR, paging past the REST
// API's 100-per-page limit via `gh api --paginate`.
func getPRFiles(repo string, pr int) ([]string, error) {
	out, err := gh("api", fmt.Sprintf("repos/%s/pulls/%d/files", repo, pr), "--paginate")
	if err != nil {
		return nil, err
	}
	return parsePRFiles(out)
}

// parsePRFiles parses the output of `gh api .../files --paginate`. With
// multiple pages, gh concatenates one JSON array per page (…][…]), so we stream
// successive top-level arrays rather than a single Unmarshal.
func parsePRFiles(out string) ([]string, error) {
	dec := json.NewDecoder(strings.NewReader(out))
	var paths []string
	for {
		var page []struct {
			Filename string `json:"filename"`
		}
		if err := dec.Decode(&page); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		for _, f := range page {
			if f.Filename != "" {
				paths = append(paths, f.Filename)
			}
		}
	}
	return paths, nil
}

func fetchFile(repo, path, ref string) string {
	endpoint := fmt.Sprintf("repos/%s/contents/%s", repo, path)
	if ref != "" {
		endpoint += "?ref=" + ref
	}
	out, err := gh("api", endpoint)
	if err != nil {
		return ""
	}
	var payload struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil || payload.Encoding != "base64" {
		return ""
	}
	raw, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(payload.Content, "\n", ""))
	if err != nil {
		return ""
	}
	if len(raw) > maxFileBytes {
		return fmt.Sprintf("(file too large: %d bytes; omitted from context)", len(raw))
	}
	if !utf8.Valid(raw) {
		return "(binary file omitted)"
	}
	return string(raw)
}

func anchorComments(index Index, comments []Comment) []string {
	var hits []string
	for _, c := range comments {
		// A live comment's `line` is a NEW-file line number; when it's null we
		// fall back to `original_line`, which is an OLD-file line number. Each
		// must be compared against the matching coordinate space of the block.
		line := 0
		oldSide := false
		if c.Line != nil {
			line = *c.Line
		} else if c.OriginalLine != nil {
			line = *c.OriginalLine
			oldSide = true
		}
		if c.Path == "" || line == 0 {
			continue
		}
		for _, b := range index.Blocks {
			if b.Path != c.Path {
				continue
			}
			start, span := b.NewStart, b.NewLines
			if oldSide {
				start, span = b.OldStart, b.OldLines
			}
			if start == nil {
				continue
			}
			if span < 1 {
				span = 1
			}
			if *start <= line && line < *start+span {
				hits = append(hits, b.BlockID)
				break
			}
		}
	}
	return hits
}
