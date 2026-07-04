# TODO

## Single-binary Go CLI (for distribution)

Per [docs/adr-001-go-cli.md](adr-001-go-cli.md): public single-binary CLI in Go (chroma for
highlighting, GoReleaser for distribution).

- [x] render-parity skeleton (chroma + `html/template`).
- [x] **full port complete** — the tool is now Go (`package main` at repo root); the Python
      implementation is removed. Same CLI, same completeness guarantee, `go test ./...` green.
- [ ] `.goreleaser.yaml` + Homebrew tap at first release.
- [ ] `go install`-friendly binary name (`cmd/ncr/` layout) if we want `go install …@latest`
      to produce `ncr` directly (currently `go build -o ncr .`).


## Force schema via tool use (instead of free-form JSON + normalize.go)

Give the model a single forced tool `submit_reading_plan` with a strict `input_schema`
(`tool_choice: {type: "tool", name: ...}`) so the API validates arguments and retries on
mismatch — killing the structural drift class we currently patch up in `normalize.go`
(nested `changeUnits`, `label` vs `symbol`, `summary` vs `overview`, missing fields).

- Build the schema **per-PR**: set the block-id field to an `enum` of the actual block ids
  (`items: {enum: ["b001", ... ]}`) and `layer` to `enum: [0..6]`. Then hallucinated block
  ids / bad layers become structurally impossible.
- Parse `tool_use.input` directly; drop the `extractJSON` brace-scanner.
- **Keep the deterministic reconciler.** JSON Schema is per-field and cannot express "every
  block appears exactly once across all units" (a global constraint), so coverage still
  needs the reconciler. Tools reduce drift; they don't replace the completeness backstop.
- `normalize.go` shrinks to a thin fallback (keep for one release, then reassess).
- Cost: changes the prompt → invalidates the plan cache → one paid run (~$0.35 Sonnet) to
  validate. Write + unit-test the schema builder and tool-input parsing offline first.

Deferred heavier option: incremental tools (`place_block` in a loop, surfacing "N blocks
remaining") to pressure coverage *during* generation. Multi-turn, more tokens/latency —
only if single-tool drift persists.
