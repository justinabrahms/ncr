# Narrative Code Review

Reorder a pull-request diff into a **reading order that follows call paths, outside-in**,
instead of the alphabetical-by-filename order every review tool gives you today.

## The problem

GitHub (and every other review tool) presents a diff as files in alphabetical order,
each file's hunks in line order. For a 300–3,000 line change this means:

- You read `db/migrations/…` and `repository/order_store.go` *before* you know what an
  order even is or which endpoint created it.
- Related changes that tell one story (handler → service → repo) are scattered across
  the file list and interleaved with unrelated ones.
- You reconstruct the architecture in your head every time, from the bottom up, which is
  exactly backwards from how you'd want to *learn* the change.

## The idea

Treat the diff as a **narrative** and impose a reading order that mirrors how a good
author would explain the change to you:

1. **Outside-in (ports & adapters).** Start at the contract / entrypoint — the API
   payload, the route, the CLI command, the event consumer. Only then descend into the
   application logic, the domain, and finally the adapters (DB, HTTP clients, migrations).
   You never see a DB column before you understand the request that needs it.

2. **Call-path grouping.** Group the change into "chapters," where a chapter is one
   coherent story that follows a call path from an entrypoint down through the code it
   calls (`POST /orders` → `OrderService.place` → `OrderRepo.insert`).

3. **Progressive disclosure.** Each chapter opens with a plain-language summary and the
   entrypoint. You expand into callees on demand. You choose your depth; the DB
   implementation is there when you want it, not before.

The reordering + plain-language explanations of each piece are produced by an LLM pipeline
(see `prompts/`). It's an **explainer, not a reviewer** — it tells you what the change does
and how the pieces connect, and leaves the judgment to you. Static analysis can augment it
later (see `docs/design.md`).

## Usage

A single Go binary — no runtime, no deps to install. Needs the `gh` CLI (for PR ingest)
and `ANTHROPIC_API_KEY` (for the plan step).

```sh
go build -o ncr .        # or: go install github.com/justinabrahms/narrative-code-review@latest

# From a GitHub PR (uses your `gh` auth)
./ncr owner/name 812

# Local, no GitHub / no API — render a diff with a supplied reading plan
./ncr --diff tests/fixtures/sample.diff --plan tests/fixtures/sample-plan.json
```

Flags: `-o out.html`, `--no-open`, `--refresh` (bust caches), `--no-spend` (never call the
API — fail loudly on a plan cache miss), `--model <id>`.

Pipeline: **ingest (`gh`) → index (deterministic) → plan (LLM) → normalize → reconcile
(deterministic) → render → `out/review.html`** (opens in your browser). The plan is cached
by a hash of the exact prompt, so iterating on presentation re-renders for free.

## Non-negotiable: nothing gets forgotten

The reordering is LLM-driven, but the LLM is **not trusted with completeness or with the
code text.** A deterministic indexer splits the diff into stable-ID'd blocks; the model
only references those ids; a deterministic reconciler proves by set-equality that every
block was placed, and renders code verbatim from the index. So a model can't silently drop,
truncate, or alter a hunk. See `docs/completeness.md`.

## Layout

Single Go package (`package main`) at the repo root:

| file | role |
|------|------|
| `index.go` | deterministic diff → stable-ID'd change blocks (+ context) |
| `reconcile.go` | coverage guarantee: every block placed, else auto-repaired |
| `normalize.go` | coerce flexible model JSON into the canonical plan |
| `plan.go` | build the prompt + call the Anthropic Messages API |
| `ingest.go` | pull the PR via `gh` | 
| `cache.go` | content-addressed cache (ingest + plan) |
| `render.go`, `templates.go`, `md.go` | HTML (chroma highlighting + `html/template`) |
| `prompts/` | LLM prompts, embedded via `go:embed` |

Design notes live in `docs/` (`design.md`, `completeness.md`, `ingest.md`, `schema.md`) and
the language decision in `docs/adr-001-go-cli.md`. Run the tests with `go test ./...`.
