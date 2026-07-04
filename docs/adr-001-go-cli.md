# ADR 001 — Distribute as a single-binary Go CLI

Status: accepted 2026-07-04 — implemented (Go port complete; Python removed)

## Context

`narrative-code-review` is currently a Python CLI (`uv run ncr …`). We want to distribute
it as a **public single-binary CLI** — `brew install`, `curl | sh`, or download-one-file,
with no runtime to install. Python is our weakest axis for exactly that: there's no clean
static-binary story (PyInstaller/shiv/pex are large, clunky, platform-specific), and even
with `uv` the ask is "have uv/Python," not "have a binary."

The tool is **I/O-bound**: it shells out to `gh`, makes one HTTP call to the Anthropic API,
and templates HTML. It spends ~all its time blocked on the network.

## Decision

Rewrite the CLI in **Go** for the distributed build. Keep Python as the fast iteration
harness until the prompts / HTML-UX / JSON schema stabilize, then port.

### Why Go over Rust

- **I/O-bound → Rust's perf/no-GC edge buys nothing here.** Not worth the borrow-checker tax.
- **chroma** (Go) is a direct Pygments port (same lexers, Monokai style) → ~1:1 highlighting
  parity with the current renderer.
- **GoReleaser** is the best single-binary distribution pipeline available: cross-compiled
  binaries per OS/arch, Homebrew tap, GitHub Releases with archives + checksums + SBOM,
  `curl | sh`. This *is* the stated goal, essentially for free.
- Faster, simpler port; `html/template` and `os/exec` are stdlib; the maintainer already
  works in Go (repo lives under `~/src/github.com/…`).

### The one thing that would flip us to Rust

If the **static-analysis roadmap** (tree-sitter call graphs, `docs/design.md` v2/v3) becomes
the *actual* product rather than a maybe. tree-sitter is Rust-native (reference bindings are
Rust). Not a reason to rewrite today for a speculative future.

## Port surface

**Carries over verbatim (language-agnostic — the real value):**
- `prompts/*.md`, `docs/schema.md` (the JSON contract), the HTML/CSS template (`go:embed`),
  and the completeness design.

**Ports — ~700 lines of pure, already-tested logic (Python tests are the executable spec):**

| Python | Go |
|--------|----|
| `index.py` (diff → blocks + context) | pure logic |
| `reconcile.py`, `normalize.py`, `md.py` | pure logic |
| `render.py` | `html/template` + `chroma` |
| `ingest.py` | `os/exec` on `gh` |
| `plan.py` | build prompt + one `net/http` POST to `/v1/messages` (no SDK — API is just JSON; caching is a `cache_control` field) |
| `cache.py` | content-addressed files |
| `__main__.py` | `cobra` or stdlib `flag` |

## Consequences

- **Sequencing:** do NOT port now — we're iterating hard on prompts/UX, and porting a moving
  target means re-porting every change. Lock the design in Python first; the port is then a
  mechanical translation for the release build, not a rewrite.
- **De-risk early (optional):** a thin Go skeleton that renders `reading-plan.json` → HTML
  would prove chroma + `html/template` parity before we commit the full port.
- **Completeness guarantee is preserved** — it's an algorithm, not a language feature.
- Distribution: add a `.goreleaser.yaml` and a Homebrew tap when we cut the first release.
