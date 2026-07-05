package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
)

// ncr CLI.
//
//	ncr <owner/repo> <pr>            # build the review and serve it on localhost
//	ncr <owner/repo> <pr> --static  # write the HTML file and exit (no server)
//	ncr .  [--base main]            # self-review the current branch vs merge-base (served, no commenting)
//	ncr --diff path/to.diff [--plan plan.json]   # local render (implies --static)
//
// Pipeline: ingest -> index -> plan (LLM) -> normalize -> reconcile -> render.

// version is set at release time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:]))
}

// versionString reports the release tag when built by goreleaser, else the git
// revision (+ "-dirty" for uncommitted changes) that Go embeds for local builds.
func versionString() string {
	if version != "" && version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		rev, dirty := "", ""
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				rev = s.Value
			case "vcs.modified":
				if s.Value == "true" {
					dirty = "-dirty"
				}
			}
		}
		if rev != "" {
			if len(rev) > 12 {
				rev = rev[:12]
			}
			return rev + dirty
		}
	}
	return "dev"
}

func run(argv []string) int {
	fs := flag.NewFlagSet("ncr", flag.ContinueOnError)
	diff := fs.String("diff", "", "path to a unified diff (local mode, skip GitHub)")
	base := fs.String("base", "main", "base ref to diff against for local review (`ncr .`)")
	plan := fs.String("plan", "", "path to a reading-plan.json (skip the LLM)")
	model := fs.String("model", "", "Anthropic model id")
	out := fs.String("o", "out/review.html", "output HTML path (with --static)")
	fs.StringVar(out, "out", "out/review.html", "output HTML path (with --static)")
	static := fs.Bool("static", false, "write the HTML file and exit instead of serving")
	noOpen := fs.Bool("no-open", false, "don't open the browser")
	refresh := fs.Bool("refresh", false, "bypass caches: re-fetch and re-call the model")
	noSpend := fs.Bool("no-spend", false, "never call the API; fail loudly on a plan cache miss")
	showVersion := fs.Bool("version", false, "print version and exit")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: ncr <owner/repo> <pr>  |  ncr .  |  ncr --diff FILE [--plan FILE]")
		fs.PrintDefaults()
	}
	flags, pos := reorderArgs(argv)
	if err := fs.Parse(flags); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Println("ncr", versionString())
		return 0
	}
	if *refresh && *noSpend {
		fmt.Fprintln(os.Stderr, "error: --refresh forces an API call, which --no-spend forbids")
		return 2
	}

	// `ncr .` is a bare "." positional: self-review the current branch against
	// its merge-base with --base, served with the commenting UI disabled.
	local := len(pos) == 1 && pos[0] == "."

	var repo string
	var pr int
	if !local {
		if len(pos) > 0 {
			repo = pos[0]
		}
		if len(pos) > 1 {
			pr, _ = strconv.Atoi(pos[1])
		}
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
	} else if local {
		logf("diffing current branch against %s (merge-base) …", *base)
		ctx, err := getLocalContext(*base)
		if err != nil {
			return fail(err)
		}
		diffText, meta, files, comments = ctx.Diff, ctx.Meta, ctx.Files, ctx.Comments
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
	var planModel string            // model that produced the plan; "" when loaded from --plan
	var rawPlanJSON json.RawMessage // the plan's raw bytes, pre normalize/reconcile (for /api/debug)
	if *plan != "" {
		b, err := os.ReadFile(*plan)
		if err != nil {
			return fail(err)
		}
		rawPlanJSON = b
		if err := json.Unmarshal(b, &raw); err != nil {
			return fail(err)
		}
	} else {
		mdl := *model
		if mdl == "" {
			mdl = defaultModel
		}
		planModel = mdl
		system, user := buildPrompt(index, files, comments, meta)
		// schemaVersion salts the key so a change in request shape (e.g. the
		// forced-tool schema) doesn't collide with older cached responses.
		pkey := "plan-" + cacheDigest(mdl, schemaVersion, system, user)
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
			b, usage, err := runModel(system, user, mdl, defaultMaxToks, planTool(index))
			if err != nil {
				return fail(err)
			}
			planBytes = b
			_ = cacheSave(pkey, planBytes)
			logf("plan cost: %s", usage.summary(mdl))
		} else {
			logf("using cached plan — no API call ($0.00)")
		}
		rawPlanJSON = planBytes
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

	// Local self-review is served but has no PR to post to, so the commenting UI
	// (which drives the GitHub review-submit flow) stays off.
	interactive := !(*static || *diff != "" || local)
	html, err := BuildHTML(rplan, index, interactive)
	if err != nil {
		return fail(err)
	}

	// --static (and local --diff mode, which has no PR to review) writes the file
	// and exits; otherwise serve interactively.
	if *static || *diff != "" {
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

	rs, err := newReviewServer(repo, pr, meta.HeadRefOid, index, rplan, rawPlanJSON, planModel)
	if err != nil {
		return fail(err)
	}
	if err := serve(html, rs, !*noOpen); err != nil {
		return fail(err)
	}
	return 0
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
	valueFlags := map[string]bool{"--diff": true, "--plan": true, "--model": true, "-o": true, "--out": true, "--base": true, "-diff": true, "-plan": true, "-model": true, "-base": true}
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
	openTarget(abs)
}

// openTarget opens a file path or URL in the OS default handler.
func openTarget(target string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", target)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	default:
		cmd = exec.Command("xdg-open", target)
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
