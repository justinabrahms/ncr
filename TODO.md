# TODO

## Print Claude cost per reviewed PR

Whenever the plan step calls the API, report the estimated spend for that PR — e.g.
`plan: 57.9k in / 11.2k out (cache: 1.1k write, 0 read) — ~$0.34 (claude-sonnet-4-6)`.

- Capture `usage` from the Messages response (`input_tokens`, `output_tokens`,
  `cache_creation_input_tokens`, `cache_read_input_tokens`) in `runModel`/`parseModelResponse`.
- Small per-model price table ($/Mtok in, out; cache write ×1.25, cache read ×0.1). Mark it
  approximate and easy to update.
- On a **cache hit** (no API call), print `$0.00 (cached)`.
- Show it in the CLI log at minimum; consider surfacing it in the served page header too.
- Consider a session/running total if a run makes multiple calls (not today, but leave room).

## Single-binary Go CLI (for distribution)

Per [docs/adr-001-go-cli.md](adr-001-go-cli.md): public single-binary CLI in Go (chroma for
highlighting, GoReleaser for distribution).

- [x] render-parity skeleton (chroma + `html/template`).
- [x] **full port complete** — the tool is now Go (`package main` at repo root); the Python
      implementation is removed. Same CLI, same completeness guarantee, `go test ./...` green.
- [ ] `.goreleaser.yaml` + Homebrew tap at first release.
- [ ] `go install`-friendly binary name (`cmd/ncr/` layout) if we want `go install …@latest`
      to produce `ncr` directly (currently `go build -o ncr .`).


## Force schema via tool use — DONE

The model now calls a forced `submit_reading_plan` tool (`tool_choice: {type:tool}`) whose
`input_schema` carries a per-PR block-id `enum` and a `layer` `enum: [0..6]`. The response
is structured JSON (the tool input), so there's no prose to scrape — this fixed the
`invalid character 'a'` failure where an incidental `{ code snippet }` in prose fooled the
old first-brace extractor. See `planTool`/`parseModelResponse` in `plan.go`.

Kept as defense in depth:
- `extractJSON` remains as a fallback for text responses, but now returns the *largest valid*
  balanced object (preferring a fenced block), not the first brace group.
- `normalize.go` still coerces the model's nested shape into the canonical plan.
- The **reconciler stays** — JSON Schema can't express "every block exactly once across all
  units" (a global constraint), so coverage is still enforced deterministically.
- The raw model response is cached (not the post-extraction result), so a future parser fix
  can never be defeated by a poisoned cache.

Deferred heavier option: incremental tools (`place_block` in a loop, surfacing "N blocks
remaining") to pressure coverage *during* generation. Only if single-tool drift persists.
