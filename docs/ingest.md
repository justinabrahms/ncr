# Ingest (via `gh` CLI)

MVP input is a GitHub PR, pulled through the user's existing `gh` auth — no token
management, no OAuth app. Everything below is non-LLM and deterministic.

## What we pull

| Need | Command |
|------|---------|
| Unified diff | `gh pr diff {N}` (add `--repo owner/name` when outside the checkout) |
| Title / body / metadata | `gh pr view {N} --json title,body,author,baseRefName,headRefName,files` |
| Existing review comments (with line anchors) | `gh api repos/{owner}/{repo}/pulls/{N}/comments --paginate` |
| Full current contents of a changed file | `gh api repos/{owner}/{repo}/contents/{path}?ref={headSha} -q .content \| base64 -d` |

Review comments carry `path`, `line`/`original_line`, and `diff_hunk` — enough to map each
comment to a change block in the index (see `docs/completeness.md`, comment anchoring).

## Why full file contents

The diff shows changed lines, not the bodies of the *unchanged* functions they call. For
call-path ordering we want to resolve callees; the current head contents of each changed
file is a cheap, high-signal context source. For 3k-line diffs, sending whole files may
blow the budget — the fallback is symbol skeletons only (still an open decision, see
`docs/design.md`).

## Output of ingest

A `pr-context.json` bundle: `{ meta, files: {path: text}, comments: [...] }`, plus the raw
diff handed to the deterministic indexer to produce `block-index.json`.

## Notes

- Reuse `gh auth status`; fail fast with a clear message if unauthenticated.
- Everything here is read-only against GitHub. We never write to the PR.
- Pin to `headSha` when fetching file contents so the diff and the file bodies agree.
