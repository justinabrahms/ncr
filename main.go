package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// ncr CLI — port of ncr/__main__.py.
//
//	ncr <owner/repo> <pr>  [-o out/review.html] [--no-open] [--refresh] [--no-spend]
//	ncr --diff path/to.diff [--plan plan.json]        # local, no GitHub
//
// Pipeline: ingest -> index -> plan (LLM) -> normalize -> reconcile -> render.

// version is set at release time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(argv []string) int {
	fs := flag.NewFlagSet("ncr", flag.ContinueOnError)
	diff := fs.String("diff", "", "path to a unified diff (local mode, skip GitHub)")
	plan := fs.String("plan", "", "path to a reading-plan.json (skip the LLM)")
	model := fs.String("model", "", "Anthropic model id")
	out := fs.String("o", "out/review.html", "output HTML path")
	fs.StringVar(out, "out", "out/review.html", "output HTML path")
	noOpen := fs.Bool("no-open", false, "don't open the browser")
	refresh := fs.Bool("refresh", false, "bypass caches: re-fetch and re-call the model")
	noSpend := fs.Bool("no-spend", false, "never call the API; fail loudly on a plan cache miss")
	showVersion := fs.Bool("version", false, "print version and exit")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: ncr <owner/repo> <pr>  |  ncr --diff FILE [--plan FILE]")
		fs.PrintDefaults()
	}
	flags, pos := reorderArgs(argv)
	if err := fs.Parse(flags); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Println("ncr", version)
		return 0
	}
	if *refresh && *noSpend {
		fmt.Fprintln(os.Stderr, "error: --refresh forces an API call, which --no-spend forbids")
		return 2
	}

	var repo string
	var pr int
	if len(pos) > 0 {
		repo = pos[0]
	}
	if len(pos) > 1 {
		pr, _ = strconv.Atoi(pos[1])
	}

	var diffText string
	var meta Meta
	var files map[string]string
	var comments []Comment

	if *diff != "" {
		b, err := os.ReadFile(*diff)
		if err != nil {
			return fail(err)
		}
		diffText = string(b)
		meta = Meta{Title: filepath.Base(*diff)}
	} else {
		if repo == "" || pr == 0 {
			fmt.Fprintln(os.Stderr, "error: give a repo and PR, e.g. `ncr owner/name 812` (or use --diff)")
			return 2
		}
		ctx, err := ingestCached(repo, pr, *refresh)
		if err != nil {
			return fail(err)
		}
		diffText, meta, files, comments = ctx.Diff, ctx.Meta, ctx.Files, ctx.Comments
	}

	index := buildIndex(diffText)
	logf("indexed %d change blocks", len(index.BlockIDs))

	var raw rawPlan
	if *plan != "" {
		b, err := os.ReadFile(*plan)
		if err != nil {
			return fail(err)
		}
		if err := json.Unmarshal(b, &raw); err != nil {
			return fail(err)
		}
	} else {
		mdl := *model
		if mdl == "" {
			mdl = defaultModel
		}
		system, user := buildPrompt(index, files, comments, meta)
		pkey := "plan-" + cacheDigest(mdl, system, user)
		var planBytes []byte
		if !*refresh {
			if b, ok := cacheLoad(pkey); ok {
				planBytes = b
			}
		}
		if planBytes == nil {
			if *noSpend {
				fmt.Fprintf(os.Stderr, "✗ --no-spend: no cached plan for this prompt (key %s).\n", pkey)
				fmt.Fprintf(os.Stderr, "  Run once without --no-spend to populate the cache (model %s would spend API credits).\n", mdl)
				return 2
			}
			logf("asking %s to organize the reading path (spends API credits) …", mdl)
			b, err := runModel(system, user, mdl, defaultMaxToks)
			if err != nil {
				return fail(err)
			}
			planBytes = b
			_ = cacheSave(pkey, planBytes)
		} else {
			logf("using cached plan — no API call")
		}
		if err := json.Unmarshal(planBytes, &raw); err != nil {
			return fail(err)
		}
	}

	rplan := normalizePlan(raw, index)

	var commentBlocks []string
	if len(comments) > 0 {
		commentBlocks = anchorComments(index, comments)
	}
	reconcile(&rplan, index, commentBlocks)

	cov := rplan.Coverage
	status := "✓ complete"
	if !cov.OK {
		status = fmt.Sprintf("⚠ %d unplaced (auto-repaired)", len(cov.Missing))
	}
	logf("coverage: %d/%d blocks — %s", cov.Counts.Placed, cov.Counts.Indexed, status)

	html, err := BuildHTML(rplan, index)
	if err != nil {
		return fail(err)
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		return fail(err)
	}
	if err := os.WriteFile(*out, html, 0o644); err != nil {
		return fail(err)
	}
	writeJSON(filepath.Join(filepath.Dir(*out), "reading-plan.json"), rplan)
	writeJSON(filepath.Join(filepath.Dir(*out), "block-index.json"), index)
	logf("wrote %s", *out)

	if !*noOpen {
		openBrowser(*out)
	}
	if cov.OK {
		return 0
	}
	return 1
}

func ingestCached(repo string, pr int, refresh bool) (PRContext, error) {
	ikey := fmt.Sprintf("ingest-%s#%d", repo, pr)
	if !refresh {
		if b, ok := cacheLoad(ikey); ok {
			var ctx PRContext
			if json.Unmarshal(b, &ctx) == nil {
				logf("using cached ingest for %s#%d", repo, pr)
				return ctx, nil
			}
		}
	}
	logf("fetching %s#%d via gh …", repo, pr)
	ctx, err := getPRContext(pr, repo, true)
	if err != nil {
		return ctx, err
	}
	if b, err := json.Marshal(ctx); err == nil {
		_ = cacheSave(ikey, b)
	}
	return ctx, nil
}

// reorderArgs separates flags from positionals so positionals may appear anywhere
// (stdlib flag stops at the first non-flag arg; argparse-style intermixing does not).
func reorderArgs(args []string) (flags, pos []string) {
	valueFlags := map[string]bool{"--diff": true, "--plan": true, "--model": true, "-o": true, "--out": true, "-diff": true, "-plan": true, "-model": true}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") && a != "-" {
			flags = append(flags, a)
			if valueFlags[a] && !strings.Contains(a, "=") && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
		} else {
			pos = append(pos, a)
		}
	}
	return flags, pos
}

func writeJSON(path string, v any) {
	if b, err := json.MarshalIndent(v, "", "  "); err == nil {
		_ = os.WriteFile(path, b, 0o644)
	}
}

func openBrowser(path string) {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", abs)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", abs)
	default:
		cmd = exec.Command("xdg-open", abs)
	}
	_ = cmd.Start()
}

func logf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "› "+format+"\n", a...)
}

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "error:", err)
	return 1
}
