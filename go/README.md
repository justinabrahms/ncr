# Go render-parity skeleton

Proof-of-concept for the planned Go port (see `../docs/adr-001-go-cli.md`). It renders
`reading-plan.json` + `block-index.json` (produced by the Python pipeline) to HTML using
**chroma** (highlighting) + **html/template**, to prove those reproduce the Python renderer
before committing to the full port. **Scope: rendering only** — ingest / plan / LLM / cache
are not here.

## Run

```sh
# produce the JSON artifacts with the Python tool first (writes out/*.json)
uv run ncr open-feature/java-sdk 1985 --no-spend

# then render them with the Go skeleton (flags before positional args)
cd go && go run . -o ../out/review-go.html ../out/reading-plan.json ../out/block-index.json
```

Compare `out/review-go.html` against `out/review.html` (the Python output). Verified parity
on PR #1985: same chapters/nodes, near-identical size, same chroma/Pygments token classes,
context lines + `⋯` dividers + markdown (code/bold) all matching.

## Map to Python

| this skeleton | Python |
|---|---|
| `render.go` (chroma tokenise, diff, assembly) | `ncr/render.py` |
| `md.go` | `ncr/md.py` |
| `templates.go` (`pageCSS` copied verbatim, `html/template`) | `ncr/render.py` `_CSS` + f-strings |
| `types.go` | the JSON contract in `docs/schema.md` |

Not yet ported: the diff indexer, reconciler, normalizer, ingest, plan, cache, CLI — see
the ADR's port surface. Those come with the full port once the Python design stabilizes.
