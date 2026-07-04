# TODO

## Port to a single-binary Go CLI (for distribution)

Decided in [docs/adr-001-go-cli.md](adr-001-go-cli.md): distribute as a public single-binary
CLI, written in Go (chroma for highlighting, GoReleaser for distribution). **Sequencing: do
NOT port yet** — keep iterating on prompts/HTML-UX/schema in Python (the fast harness), then
port once they stabilize. Port surface + rationale are in the ADR.

- [x] (optional, de-risk) thin Go skeleton: `reading-plan.json` → HTML, proving chroma +
      `html/template` parity with the Python renderer. Done — see `go/` (verified on #1985:
      same chapters/nodes, matching token classes, context + markdown all render).
- [ ] full port once prompts/UX freeze; carry `prompts/`, `docs/schema.md`, HTML template
      (`go:embed`) verbatim; port the ~700 lines of tested logic + their tests.
- [ ] `.goreleaser.yaml` + Homebrew tap at first release.


## Force schema via tool use (instead of free-form JSON + normalize.py)

Give the model a single forced tool `submit_reading_plan` with a strict `input_schema`
(`tool_choice: {type: "tool", name: ...}`) so the API validates arguments and retries on
mismatch — killing the structural drift class we currently patch up in `normalize.py`
(nested `changeUnits`, `label` vs `symbol`, `summary` vs `overview`, missing fields).

- Build the schema **per-PR**: set the block-id field to an `enum` of the actual block ids
  (`items: {enum: ["b001", ... ]}`) and `layer` to `enum: [0..6]`. Then hallucinated block
  ids / bad layers become structurally impossible.
- Parse `tool_use.input` directly; drop the `extract_json` brace-scanner.
- **Keep the deterministic reconciler.** JSON Schema is per-field and cannot express "every
  block appears exactly once across all units" (a global constraint), so coverage still
  needs the reconciler. Tools reduce drift; they don't replace the completeness backstop.
- `normalize.py` shrinks to a thin fallback (keep for one release, then reassess).
- Cost: changes the prompt → invalidates the plan cache → one paid run (~$0.35 Sonnet) to
  validate. Write + unit-test the schema builder and tool-input parsing offline first.

Deferred heavier option: incremental tools (`place_block` in a loop, surfacing "N blocks
remaining") to pressure coverage *during* generation. Multi-turn, more tokens/latency —
only if single-tool drift persists.
