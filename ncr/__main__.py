"""Narrative code review CLI.

    ncr <owner/repo> <pr>  [-o out/review.html] [--no-open]
    ncr --diff path/to.diff [--plan plan.json]        # local, no GitHub

Pipeline: ingest -> index -> plan (LLM) -> reconcile -> render.
The --plan flag skips the LLM and renders a supplied plan (useful for iterating
on the renderer or replaying a saved plan deterministically).
"""

from __future__ import annotations

import argparse
import json
import sys
import webbrowser
from pathlib import Path

from ncr.index import build_index
from ncr.reconcile import reconcile
from ncr.render import build_html


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(
        prog="ncr", description="Narrative, outside-in code review.",
        usage="ncr <owner/repo> <pr>  |  ncr --diff FILE [--plan FILE]")
    ap.add_argument("repo", nargs="?", help="GitHub repo as owner/name")
    ap.add_argument("pr", nargs="?", type=int, help="pull request number")
    ap.add_argument("--diff", help="path to a unified diff (local mode, skip GitHub)")
    ap.add_argument("--plan", help="path to a reading-plan.json (skip the LLM)")
    ap.add_argument("--model", help="Anthropic model id")
    ap.add_argument("-o", "--out", default="out/review.html", help="output HTML path")
    ap.add_argument("--no-open", action="store_true", help="don't open the browser")
    ap.add_argument("--refresh", action="store_true",
                    help="bypass caches: re-fetch from GitHub and re-call the model")
    args = ap.parse_args(argv)

    from ncr import cache

    if args.diff:
        diff = Path(args.diff).read_text()
        meta, files, comments = {"title": Path(args.diff).name}, {}, []
    else:
        if not args.repo or args.pr is None:
            ap.error("give a repo and PR, e.g. `ncr owner/name 812` (or use --diff)")
        ikey = f"ingest-{args.repo}#{args.pr}"
        ctx = None if args.refresh else cache.load(ikey)
        if ctx is None:
            from ncr.ingest import get_pr_context
            print(f"› fetching {args.repo}#{args.pr} via gh …", file=sys.stderr)
            ctx = get_pr_context(args.pr, repo=args.repo)
            cache.save(ikey, ctx)
        else:
            print(f"› using cached ingest for {args.repo}#{args.pr}", file=sys.stderr)
        diff, meta, files, comments = ctx["diff"], ctx["meta"], ctx["files"], ctx["comments"]

    index = build_index(diff)
    print(f"› indexed {len(index['blockIds'])} change blocks", file=sys.stderr)

    if args.plan:
        plan = json.loads(Path(args.plan).read_text())
    else:
        from ncr.plan import build_prompt, run_model, DEFAULT_MODEL
        model = args.model or DEFAULT_MODEL
        system, user = build_prompt(index, files, comments, meta)
        pkey = f"plan-{cache.digest(model, system, user)}"
        plan = None if args.refresh else cache.load(pkey)
        if plan is None:
            print(f"› asking {model} to organize the reading path (spends API credits) …",
                  file=sys.stderr)
            plan = run_model(system, user, model=model)
            cache.save(pkey, plan)
        else:
            print("› using cached plan — no API call", file=sys.stderr)

    comment_blocks = []
    if comments:
        from ncr.ingest import anchor_comments
        comment_blocks = anchor_comments(index, comments)

    plan = reconcile(index, plan, comment_blocks=comment_blocks)
    cov = plan["coverage"]
    status = "✓ complete" if cov["ok"] else f"⚠ {len(cov['missing'])} unplaced (auto-repaired)"
    print(f"› coverage: {cov['counts']['placed']}/{cov['counts']['indexed']} blocks — {status}",
          file=sys.stderr)

    out = Path(args.out)
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(build_html(plan, index))
    (out.parent / "reading-plan.json").write_text(json.dumps(plan, indent=2))
    print(f"› wrote {out}", file=sys.stderr)
    if not args.no_open:
        webbrowser.open(out.resolve().as_uri())
    return 0 if cov["ok"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
