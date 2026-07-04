# Design sketch

## Goal restated

Input: a diff of ~300–3,000 lines (a GitHub PR, or a raw unified diff).

Output: a **reordered, layered, annotated reading plan** for that diff — a tree of
**chapters**, each a theme (a capability's call path, or a shared concern for a refactor),
read outside-in, with per-node plain-language explanations. Rendered in a UI that supports
progressive disclosure.

The tool is an **explainer, not a reviewer**: it describes what the change does and how the
pieces connect, in a good reading order. It does *not* critique the code, flag bugs, or
score risk — the human forms their own judgment. We're just making the read faster and
better-sequenced.

## The reading model (the thing that makes this different)

We sort every change into a **layer** on an outside-in axis. The layer ordering *is* the
reading order.

| # | Layer | Examples |
|---|-------|----------|
| 0 | **Contract** | OpenAPI/GraphQL/proto schema, route table, public types, request/response DTOs, event schema |
| 1 | **Entrypoint / Port** | HTTP handler, controller, CLI command, queue consumer, cron, webhook |
| 2 | **Application / Use-case** | service orchestration, command handlers, transaction script |
| 3 | **Domain** | entities, value objects, pure business logic, invariants |
| 4 | **Adapter / Infrastructure** | repository impls, DB queries, migrations, HTTP/SDK clients, cache |
| 5 | **Cross-cutting** | config, DI/wiring, middleware, utils, feature flags |
| 6 | **Tests & docs** | unit/integration tests, fixtures, README/docs |

Two orthogonal structures on top of layers:

- **Units** = concerns, not symbols. A unit is one idea the reader holds at once; it may
  span several functions, types, or files if they advance the same concern. We deliberately
  group coarsely — a handful of concern-units per PR, not one per function — because
  reviewers comprehend a concern, not a function at a time (ClusterChanges, ICSE'15; Baum
  et al., ICSME'17). The deterministic indexer still splits blocks at function boundaries,
  but that's the completeness-accounting grain; units regroup those blocks by concern.
- **Chapters** = themes. When the change adds behavior, a chapter is a *capability*: rooted
  at an entrypoint (or the highest layer touched), it contains the changed nodes reachable
  from it, ordered by call depth. When there is no call path — a refactor or tooling change —
  a chapter is the *shared concern* its units advance. **Never one chapter per file.** A
  change reachable from two entrypoints is placed under its *primary* one and cross-linked.
- **Orphans** = changed nodes with no caller in the diff (e.g. a new util, a migration
  nobody in the diff references). Collected into their own end-of-report chapters, grouped
  by layer, so they don't pollute the narrative but aren't lost.

## Pipeline (MVP: single-shot LLM, `gh` CLI input)

```
gh CLI ─▶ [I] Index ─▶ [P] Plan (LLM, single-shot) ─▶ [R] Reconcile ─▶ reading-plan.json ─▶ UI
 diff +     diff →        block-index + PR ctx  →       line-coverage     (+coverage badge)
 files +    stable ID'd   → reading plan of         →   check; auto-repair
 comments   blocks (det.) chapters/units (ids only)    gaps into Unplaced
            ▲ source of truth for completeness                ▲ deterministic, no model
```

- **Ingest** — non-LLM, via `gh` (see `docs/ingest.md`): `gh pr diff N` for the unified
  diff, `gh pr view N --json` for title/body, `gh api …/comments` for existing review
  comments, and `gh api` to fetch *full current contents* of each changed file (callee
  bodies the diff omits). No token juggling — reuse the user's `gh` auth.
- **[I] Index** — non-LLM, deterministic. Parse the diff into stable-ID'd **change blocks**
  (`block-index.json`). This is the completeness source of truth. See `docs/completeness.md`.
- **[P] Plan** — LLM, single-shot (`prompts/00-single-shot.md`). Given the block index +
  full files + PR context, emit the reading plan (chapters, units, layers, edges,
  narrative), grouping blocks into units by concern. Units reference **block ids**, never
  raw code.
- **[R] Reconcile** — non-LLM, deterministic. Assert every changed *line* of every block is
  placed exactly once (line granularity, so a block may be split across units);
  auto-repair any gap into an `Unplaced` chapter; attach PR comments to their blocks;
  write the `coverage` report. See `docs/completeness.md`.

The staged prompts (`01`–`04`) are kept for when single-shot gets unreliable at 3k lines,
or when we splice static-analysis edges in — but the MVP is Index → single-shot → Reconcile.

The contract between stages is JSON; the schemas live in `docs/schema.md`.

## Where the LLM is load-bearing vs. where static analysis should take over

The LLM is doing three jobs: (a) segmenting the diff into symbols, (b) *classifying* layers
(this is genuinely judgment / semantic), (c) inferring call edges from names. (a) and (c)
are things a language server / tree-sitter + scope resolution does more reliably and
cheaply. So the intended evolution:

- **MVP:** LLM does everything, diff + full changed files as context. Fast to build,
  language-agnostic, good enough to validate the *reading model*.
- **v2:** tree-sitter for segmentation (accurate symbol spans), LLM keeps layer
  classification + narrative. Deterministic edges where the AST gives them; LLM fills gaps.
- **v3:** full LSP / SCIP index for real cross-file call graphs including into unchanged
  code. LLM is narrative-only.

Layer classification stays LLM-shaped the whole way — it's the part that's actually
semantic and repo-idiomatic.

## UI (undecided, but here's the shape either way)

The renderer consumes `reading-plan.json` and shows:

- A left rail of chapters (the reading order) with layer badges.
- A main column: expandable nodes. Collapsed = summary + signature + layer. Expanded =
  the actual diff hunk + review notes. Click a callee reference to jump/expand it inline.
- A "depth" control (show me layers 0–2 only / everything) for progressive disclosure.
- Original file/line links back to GitHub so nothing is lost.

The reading plan is UI-agnostic on purpose, so the desktop-vs-web decision doesn't block
prompt work. See open questions.

## Decisions (2026-07-04)

- **Pipeline:** single-shot first (`prompts/00-single-shot.md`), validate the reading
  model on real PRs before staging.
- **Input:** GitHub PR via the `gh` CLI — reuse the user's existing auth, no token
  management. See `docs/ingest.md`.
- **Analysis:** LLM-only MVP; tree-sitter/LSP is the documented v2/v3 path above.
- **Completeness:** non-negotiable guarantee via deterministic index + reconciler, so the
  LLM-only single-shot path can't silently drop a hunk. See `docs/completeness.md`.

## Still open

1. **Platform** — web (shareable URL per PR, GitHub OAuth), desktop (private code stays
   local), or VS Code extension (lives where reviewers are). Reading plan is UI-agnostic,
   so this doesn't block prompt work. Leaning web for reach; desktop for the privacy story.
2. **Context budget** — for 3k-line diffs, send full changed files or just the block index +
   symbol skeletons? Affects cost and call-path resolution quality.
3. **Re-ask on coverage miss** — auto-repair only (always safe), or also spend a targeted
   re-ask round to place missing blocks properly? Cost vs. polish.
