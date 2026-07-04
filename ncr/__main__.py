"""Narrative code review CLI.

    python -m ncr <pr-number> [--repo owner/name] [-o out/review.html] [--open]
    python -m ncr --diff path/to.diff [--plan plan.json]   # local, no GitHub

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
    ap = argparse.ArgumentParser(prog="ncr", description="Narrative, outside-in code review.")
    src = ap.add_mutually_exclusive_group(required=True)
    src.add_argument("pr", nargs="?", type=int, help="GitHub PR number")
    src.add_argument("--diff", help="path to a unified diff (skip GitHub)")
    ap.add_argument("--repo", help="owner/name (default: current repo)")
    ap.add_argument("--plan", help="path to a reading-plan.json (skip the LLM)")
    ap.add_argument("--model", help="Anthropic model id")
    ap.add_argument("-o", "--out", default="out/review.html", help="output HTML path")
    ap.add_argument("--no-open", action="store_true", help="don't open the browser")
    args = ap.parse_args(argv)

    if args.diff:
        diff = Path(args.diff).read_text()
        meta, files, comments = {"title": Path(args.diff).name}, {}, []
    else:
        from ncr.ingest import get_pr_context
        print(f"› fetching PR #{args.pr} via gh …", file=sys.stderr)
        ctx = get_pr_context(args.pr, repo=args.repo)
        diff, meta, files, comments = ctx["diff"], ctx["meta"], ctx["files"], ctx["comments"]

    index = build_index(diff)
    print(f"› indexed {len(index['blockIds'])} change blocks", file=sys.stderr)

    if args.plan:
        plan = json.loads(Path(args.plan).read_text())
    else:
        from ncr.plan import make_plan
        print("› asking the model to organize the reading path …", file=sys.stderr)
        kwargs = {"model": args.model} if args.model else {}
        plan = make_plan(index, files, comments, meta, **kwargs)

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
    print(f"› wrote {out}", file=sys.stderr)
    if not args.no_open:
        webbrowser.open(out.resolve().as_uri())
    return 0 if cov["ok"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
