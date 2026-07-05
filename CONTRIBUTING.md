# Contributing

`ncr` is a single Go program (`package main` at the repo root). No runtime, no external
services beyond the `gh` CLI and the Anthropic API.

## Build & run

```sh
go build -o ncr .        # or: go install github.com/justinabrahms/ncr@latest
go test ./...            # index + reconcile specs
```

Requirements at runtime: the [`gh`](https://cli.github.com/) CLI (authenticated — used for
PR ingest) and `ANTHROPIC_API_KEY` (the plan step). Neither is needed for `--diff`/`--plan`
local mode.

```sh
./ncr owner/name 812                                    # serve the review on localhost
./ncr owner/name 812 --static                           # write the HTML and exit
./ncr --diff tests/fixtures/sample.diff --plan tests/fixtures/sample-plan.json
```

By default `ncr` serves the review over localhost (the home for inline commenting, being
built per `docs/design-review-comments.md`). `--static` writes the HTML file instead;
`--diff` local mode implies `--static`.

Flags: `--static`, `-o out.html`, `--no-open`, `--refresh` (bust caches), `--no-spend`
(never call the API — fail loudly on a plan cache miss), `--model <id>`,
`--max-tokens <n>` (model output ceiling; overrides `NCR_MAX_TOKENS`, default 32000),
`--version`.

## Pipeline

```
ingest (gh) → index → plan (LLM) → normalize → reconcile → render → out/review.html
```

- **ingest** (`ingest.go`) — pull diff, metadata, review comments, and full changed-file
  contents via `gh`. Cached by `repo#pr`.
- **index** (`index.go`) — deterministic: split the diff into stable-ID'd change blocks
  (+ up to 3 context lines each). The completeness source of truth.
- **plan** (`plan.go`) — build the prompt (embedded in `prompts/`) and call the Anthropic
  Messages API. Cached by a hash of the exact prompt, so re-rendering is free.
- **normalize** (`normalize.go`) — coerce the model's flexible JSON into the canonical plan.
- **reconcile** (`reconcile.go`) — prove every block is placed; auto-repair misses into a
  visible "Unplaced" chapter. The completeness guarantee (see `docs/completeness.md`).
- **render** (`render.go`, `templates.go`, `md.go`) — HTML via chroma (syntax highlighting)
  and `html/template`. Code is joined from the block index, never from model output.

## Files

| file | role |
|------|------|
| `index.go` | diff → stable-ID'd change blocks (+ context) |
| `reconcile.go` | coverage guarantee: every block placed, else auto-repaired |
| `normalize.go` | coerce flexible model JSON into the canonical plan |
| `plan.go` | build the prompt + call the Anthropic Messages API |
| `ingest.go` | pull the PR via `gh` |
| `cache.go` | content-addressed cache (ingest + plan), under `$NCR_CACHE_DIR` or `./.ncr-cache` |
| `render.go`, `templates.go`, `md.go` | HTML rendering |
| `types.go` | the JSON contract (see `docs/schema.md`) |
| `prompts/` | LLM prompts, embedded via `go:embed` |

## Design docs

- `docs/design.md` — the outside-in reading model and roadmap
- `docs/completeness.md` — the "nothing gets forgotten" guarantee
- `docs/schema.md` — JSON shapes; `docs/ingest.md` — the `gh` calls
- `docs/adr-001-go-cli.md` — why Go; `TODO.md` — what's next
