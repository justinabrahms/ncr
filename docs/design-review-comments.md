# Design: inline review comments (`ncr serve`)

Add the ability to comment on the diff in the `ncr` view, queue comments locally, hit
"review done," and submit them to the GitHub PR as one review — with support for revisions
(the PR moving under you) and iterating across multiple review rounds.

This turns `ncr` from a one-shot static-HTML generator into an interactive local tool.

## Decisions (from the design interview, 2026-07-04)

| Question | Decision |
|----------|----------|
| Persistence / architecture | **Local server.** `ncr` runs a localhost backend; the page talks to it. |
| Command shape | **Serve by default.** `ncr owner/repo N` launches the server. `--static` writes the HTML file (today's behavior) and exits. |
| Submit action | **One GitHub review** batching all comments, with a summary body + a verdict: Comment / Approve / Request changes. |
| Preview | **Preview + confirm**, and the preview *is* the composer — where you write the summary body and pick the verdict. |
| Existing comments | **Write-only.** We don't fetch or manage others' / prior comments from GitHub. |
| Comment targets | Single line and **multi-line ranges**, on **added/context (RIGHT)** and **removed (LEFT)** lines. No file-level comments. |
| Submit failures | **Validate up-front, block.** GitHub rejects a whole review if one comment can't anchor, so nothing posts until every comment is valid. |
| Revisions (unsubmitted queue vs. new commits) | **Re-anchor by line text.** Clean matches follow the line; movers/deletions go to a "needs re-placing" tray. |
| After submit | **Archive locally, show as submitted.** Submitted comments render faintly as "submitted · round N"; new comments form the next round. |
| Queue storage | **Local-only, private.** A per-user file, not shared/committed. |

## Command surface

```
ncr owner/repo N            # default: build the review, start the server, open the browser
ncr owner/repo N --static   # write out/review.html and exit (no server, no comments)
ncr --diff X --plan Y       # local render; --static implied (no PR to submit to)
```

`--static` keeps `-o`. Serve mode ignores `-o` (the page is served, not written). All the
existing flags (`--refresh`, `--no-spend`, `--model`) still apply to the plan step.

## Architecture

`ncr serve` runs the existing pipeline (ingest → index → plan → normalize → reconcile →
render) once, then holds the result in memory and serves:

- `GET /` — the rendered page + review JS.
- `GET /api/state` — `{ headSha, pending[], submitted[], stale[], draft }`.
- `GET /api/debug` — read-only session dump for an external MCP server / bug-report
  triage: `{ version, repo, pr, headSha, model, plan, coverage, review{draft,pending,submitted} }`.
  Add `?verbose=1` to also include the (large) block index. Introspection only — it
  scrapes nothing from the rendered HTML and mutates no state.
- `POST /api/comments` — add `{ path, side, line, startLine?, startSide?, body }` → assigns
  an id, snapshots the anchored line text(s), returns the comment.
- `PATCH /api/comments/{id}` — edit body / re-place a stale comment.
- `DELETE /api/comments/{id}`.
- `POST /api/review/preview` — re-validate all pending comments against the *current* head;
  return what would post + any blockers (stale, or verdict/body rules).
- `POST /api/review/submit` — `{ verdict, body }`; validate, POST the review to GitHub,
  archive the round, clear pending, return the review URL.

Single user, localhost only — no auth on the server itself. GitHub auth is the user's `gh`
token; submission is `gh api --method POST /repos/{o}/{r}/pulls/{N}/reviews --input -`.

## Storage schema

Local-only, private, durable (not the disposable plan cache). Default
`~/.ncr/reviews/<owner>__<repo>__<pr>.json` (override `NCR_STATE_DIR`). Keyed by PR, so the
same review resumes from any working directory.

```jsonc
{
  "repo": "owner/repo",
  "pr": 145,
  "headSha": "abc123…",              // the sha the pending comments are anchored against
  "draft": { "body": "", "verdict": "COMMENT" },
  "pending": [
    {
      "id": "c1",
      "path": "internal/order/handler.go",
      "side": "RIGHT",               // RIGHT = added/context (new file), LEFT = removed (old file)
      "line": 42,                    // file line number on that side
      "startLine": 40, "startSide": "RIGHT",   // optional, for a range
      "lineText": "id, err := h.svc.Place(...)",  // snapshot for re-anchoring
      "rangeText": ["…","…"],        // optional, the range's lines
      "body": "no validation before the service call",
      "createdAt": "…"
    }
  ],
  "submitted": [
    { "round": 1, "reviewUrl": "https://github.com/…", "verdict": "REQUEST_CHANGES",
      "submittedAt": "…", "headSha": "…", "comments": [ /* anchors + bodies as posted */ ] }
  ]
}
```

## Comment anchoring (UI line → GitHub anchor)

GitHub review comments attach to `(path, line, side, commit_id)`, with `start_line`/
`start_side` for ranges. `ncr` reorders the diff but every rendered line still maps to a
real file position, so the renderer emits per-line data attributes:

- The diff renderer already knows each block's `newStart`/`oldStart`; walking a block's
  lines yields, per line, `(newLine, oldLine)`. Added/context → `side=RIGHT`, use `newLine`;
  removed → `side=LEFT`, use `oldLine`.
- Each rendered line carries `data-path`, `data-side`, `data-line`, `data-text`.
- Clicking a line's gutter opens an inline composer; shift-click (or drag) selects a range
  on one side.

**Local validation mirrors GitHub's rule:** a comment is anchorable iff its `(path, side,
line)` falls within a hunk we rendered (blocks + their 3 context lines ≈ GitHub's hunk
context). We validate locally first to catch problems before the API call, then still treat
a GitHub rejection as authoritative.

## Submit flow

"Review done" → `POST /api/review/preview` → modal that is also the composer:

1. **Summary body** textarea (the overall PR-level review comment).
2. **Verdict** picker: Comment / Approve / Request changes.
3. The list of pending comments with their resolved anchors; any **stale** comment (failed
   re-anchor) is flagged and **blocks submit** until re-placed or deleted.
4. Rules enforced before the button enables: `REQUEST_CHANGES`/`COMMENT` need a non-empty
   body; a review with zero comments is allowed if it has a body + verdict (a plain
   approve/comment).
5. **Confirm** → `POST /api/review/submit` → builds the review JSON, pipes it to
   `gh api … /reviews --input -`, archives the round, clears pending, shows the review URL.

## Revisions: re-anchor by line text

On `ncr serve` startup (and on an explicit "refresh from GitHub"), compare the stored
`headSha` to the current PR head:

- **Unchanged** → load pending as-is.
- **Changed** → for each pending comment, find `lineText` in the new file for that `path`:
  - exactly one match in the diff on the same side → update `line`, keep it (a "mover,"
    shown briefly as relocated);
  - zero or multiple matches → mark **stale** → into the "needs re-placing" tray.
- Update stored `headSha`. Submitted rounds are untouched (they live on GitHub against their
  own sha; write-only).

Full file contents are already fetched during ingest, so this needs no extra data.

## Iteration: rounds & archive

- The queue is "pending" (unsubmitted) comments. Submitting moves them into `submitted` as a
  numbered round with the review URL + verdict.
- Submitted comments render **faintly, labeled "submitted · round N · <verdict>"** so you
  don't re-comment the same spot; they're read-only locally (edit/resolve happens on GitHub).
- New comments after a submit form the next pending round. Submit again → round N+1.

## Edge cases

- **Cross-side range** (added + removed in one selection): unsupported by GitHub; restrict a
  range to a single side in the UI.
- **PR moved between load and submit:** the submit path re-fetches head + re-validates; if it
  moved, run re-anchoring and re-show the preview rather than posting against a stale sha.
- **Approving your own PR:** GitHub rejects it; surface the error, don't lose the queue.
- **Double submit:** the round is archived and pending cleared atomically; the button
  disables during the in-flight request.
- **Empty body on REQUEST_CHANGES/COMMENT:** blocked in the composer (GitHub requires it).
- **Comment on a context line:** allowed (`side=RIGHT`, new-file line) — it's in the hunk.
- **Server killed mid-review:** state is persisted on every mutation, so restarting `ncr
  serve` resumes the exact pending queue.
- **`--static` + PR:** allowed (just renders); commenting/submit are server-only features.

## Open / to confirm

- **Submission backend:** direct `gh api` (default — no deps) vs. the existing `crit` tool,
  which already does inline-comment ↔ PR sync. `crit` could save work but adds an external
  dependency; leaning `gh api`. Revisit at implementation.
- **State dir:** `~/.ncr/reviews/` (durable, cwd-independent) vs. project-local `./.ncr/`.
  Proposed home dir so in-progress reviews aren't tied to a checkout or wiped with the cache.

## Rough build phases

1. **Server skeleton + `--static` flip:** default serve, `--static` = today's output; serve
   the existing HTML unchanged over HTTP.
2. **Anchor data in the renderer:** per-line `data-*` attributes; local anchor validation.
3. **Commenting UI + queue API + persistence:** add/edit/delete, inline composer, tray,
   state file.
4. **Preview/composer + submit:** verdict + body, validation, `gh api` review POST, archive.
5. **Revisions:** head-sha detection + re-anchor-by-line-text + stale tray.
6. **Iteration polish:** submitted-round rendering, multi-round flow.
