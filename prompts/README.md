# Prompts

The LLM pipeline. Two ways to run it:

- **Single-shot** (`00-single-shot.md`) — one call, diff in, full `reading-plan.json` out.
  Start here. Cheapest to build, good enough to validate the reading model on real PRs.
- **Staged** (`01`–`04`) — one call per pipeline stage. Reach for this when single-shot
  gets unreliable on large diffs, or when you want to splice static-analysis edges in
  before the ordering stage.

Conventions used in every prompt:

- Each prompt has three parts: **System** (role + rules), **User template** (with
  `{{placeholders}}`), and **Output** (a strict JSON schema, echoed from `docs/schema.md`).
- Models should be run with structured output / JSON mode. No prose outside the JSON.
- `{{blockIndex}}` is `block-index.json` — the diff pre-split into stable-ID'd change
  blocks by the deterministic indexer. The model references **block ids**, never raw diff
  code. This is what makes completeness checkable (see `docs/completeness.md`).
- `{{files}}` is a map of `path → full current file text` for changed files (context for
  resolving call targets). `{{comments}}` is existing PR review comments to anchor.
- The layer taxonomy (0–6) is defined once, in `_shared/layers.md`, and included by
  reference in each prompt so it stays consistent.
- Design principle: **the model classifies and narrates; it does not invent code and does
  not own completeness.** Every unit traces to real change blocks by id; a deterministic
  reconciler proves every block was placed. Unknown > guessed — prefer `layer: 5` +
  `"uncertain"` over a confident wrong layer, and place an unclassifiable block rather than
  dropping it.
