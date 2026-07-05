# Spec: demo gallery — real PRs, before/after

Status: proposed. Supersedes the single-fixture page shipped for #15.

## Problem

The current Pages site renders the toy fixture (`tests/fixtures/sample.diff`, 6 invented
blocks). It shows *what the output looks like*, but not *why you'd want it*: the fixture is
small enough that alphabetical order doesn't hurt, and the code isn't real, so no visitor
recognizes their own pain in it.

Adoption follows recognition. The visitor needs to see **a real PR, in a language they
work in, that they can feel would be annoying to review on GitHub** — and then the same PR
as an ncr narrative that is obviously easier to follow.

## What ships

```
https://justinabrahms.github.io/ncr/
├── index.html            landing page: pitch + gallery of 3–5 example cards
├── <slug>.html           one ncr render per example (self-contained, as today)
└── (assets inlined — the whole site keeps the zero-external-requests property)
```

One example = one real, merged, public GitHub PR, rendered by ncr, with a thin demo
banner linking back to the gallery and to the PR on GitHub.

## Example selection criteria

An example earns its slot only if all of these hold:

1. **Real and public.** A merged PR in a recognizable OSS repo, permissive license
   (MIT/Apache/BSD preferred — see Licensing). Link to the original PR prominently.
2. **The right size.** ~300–1,500 changed lines across ~8–25 files. Below that,
   alphabetical order doesn't hurt and ncr looks like overhead; above that, a visitor
   won't finish reading and the page gets heavy.
3. **Cross-layer call path.** The change must descend through layers — contract/route →
   handler → service/domain → adapter/migration — because chapters are the product.
   Mechanical changes (renames, dep bumps, formatting, generated code, lockfiles) are
   disqualified: they produce flat, boring plans.
4. **Visible alphabetical pain.** On GitHub's Files tab, something low-level must sort
   *before* the entrypoint (a `db/migration`, `__tests__/`, `adapters/` file). Each
   example's card states this concretely: "GitHub shows you `002_add_orders.sql` first;
   ncr shows you `POST /orders` first."
5. **A good plan, verified by a human.** The narrative is the demo. If the model's
   chapters/units for a candidate PR read poorly, pick a different PR (or re-cut the
   plan) — never ship a mediocre plan as the sales pitch.

## Language/framework slots (5)

Cover the biggest reviewer populations. One slot each; candidates below are suggestions —
the curation step (below) validates the specific PR against the criteria:

| Slot | Audience | Candidate repos to hunt in |
|---|---|---|
| TypeScript / React or Node | largest OSS audience | excalidraw, hono, tRPC, fastify |
| Python / Django or FastAPI | web + data crowd | django, fastapi, starlette, litestar |
| Go | infra/backend crowd, dogfood-adjacent | caddy, gin, sqlc, charm projects |
| Java or Kotlin / Spring or Ktor | enterprise reviewers (most acute pain) | spring-boot, quarkus, ktor |
| Wildcard: Ruby/Rails or Rust | breadth signal | rails, mastodon*, axum, ripgrep |

\* license check required (AGPL needs care; prefer MIT/Apache).

A sixth "meta" example is cheap and charming: an ncr PR reviewed by ncr (e.g. the
chapters/concern-units PR, #3 — 104 blocks, real Go, and we already have the ingest
cached). Dogfooding builds trust.

**How to hunt:** `gh search prs --repo <repo> --merged --review-approved -- "feat"` then
filter by size (`additions`/`deletions`/`changedFiles` via `gh pr view --json`). Look for
"add endpoint/command/feature" titles; skip "refactor", "bump", "chore".

## The before/after device

No screenshots to maintain. The contrast is text, stated twice:

1. **On the gallery card:**
   - repo + PR title, language badge, `N files · +A −D`
   - one line of pain: *"Alphabetical order starts you at `db/migrations/0042_….sql` —
     before you know what an order is."*
   - one line of relief: the first 3 chapter titles from the actual plan.
   - CTA: **Read it as a story →** (ncr render) · *view on GitHub* (the real PR).
2. **On the example page,** a slim banner injected above ncr's own header:
   `ncr demo · <repo>#<n> on GitHub · ← more examples`, plus a one-sentence
   "what to notice" (e.g. "Note the migration lands in chapter 4, after you've met its
   consumer"). Collapsed by default; must not compete with the render itself.

Banner injection happens at site-build time (insert after `<body>` in the workflow).
If it grows fiddly, promote to an `ncr --demo-banner <file>` flag — not needed for v1.

## Frozen-artifact pipeline

CI must never call the Anthropic API or GitHub for the demos. Each example freezes its
inputs in-repo; the site rebuilds deterministically from them:

```
demos/
  <slug>/
    meta.yaml        # repo, pr number, url, language, license, card copy (pain/relief lines)
    ingest.json      # frozen PRContext (diff + file contents + PR metadata)
    plan.json        # frozen reading plan (the human-approved cut)
site/
  index.html         # hand-written landing page (no framework)
```

Build step (in `pages.yml`, replacing the current fixture render):

```
for slug in demos/*/: ncr --diff <(extracted from ingest.json) --plan demos/$slug/plan.json -o site/$slug.html
inject banner; assemble index.html cards from meta.yaml
```

Properties this buys:
- **Renderer changes propagate free.** Every push to main re-renders all examples with
  the current renderer; plans only get re-cut deliberately.
- **Deterministic + keyless CI.** Local `--diff/--plan` mode needs no `gh`, no API key.
- **Reproducible reviews.** `plan.json` is code-reviewed like anything else.

Note: `--diff` mode currently takes a diff file, not an ingest JSON. Either add a small
`--ingest demos/x/ingest.json` input mode (nicer: preserves PR title/number) or extract
`diff` from the JSON in the workflow. Title/PR number already flow from `plan.json`, so
the extraction hack works for v1.

## Curation workflow (adding or re-cutting an example)

1. Pick a candidate PR against the criteria; check the repo license.
2. `ncr owner/repo N --static` locally (~$0.30). This populates `.ncr-cache/`.
3. **Read the render critically.** Chapters must be themes/capabilities, units coarse,
   summaries non-restating. If not: try `--refresh` for a re-cut, or reject the PR.
4. Copy `.ncr-cache/ingest-…json` → `demos/<slug>/ingest.json` and the plan cache entry →
   `demos/<slug>/plan.json`. Write `meta.yaml` (including the card's pain/relief copy —
   the alphabetical-first file vs. the first chapter titles).
5. PR the new `demos/<slug>/` + regenerated `site/` snapshot; reviewer judges the
   *narrative quality*, not just the code.

Re-cut cadence: only when the prompt/schema improves enough to matter. The frozen plan is
a feature, not staleness — it's the curated cut.

## Landing page content (index.html)

Order of elements:

1. Tagline (current: "Turn a PR into a story you read outside-in…") + 2-sentence pitch.
   Second sentence is the trust differentiator: *"A deterministic reconciler proves every
   changed line is shown — the model can't drop or alter a hunk."*
2. The gallery: one card per example (see device above). Language badges are the primary
   visual sort key — a visitor should find "their" language in under 5 seconds.
3. Install one-liner (`brew install justinabrahms/tap/ncr`) + link to repo.
4. Footer: attribution per example (repo, license, PR link), MIT notice for ncr.

Keep it dependency-free, hand-written HTML/CSS matching the render's visual language
(reuse `--fg/--bg/--card` palette). Must read fine on mobile; diff pages already scroll
horizontally within `pre`.

## Licensing / attribution

Rendering a public PR redistributes source excerpts. Constraints:

- Prefer MIT/Apache/BSD repos; anything copyleft needs the license shipped alongside.
- Every example page and the footer link the source repo, PR, and license.
- If a maintainer objects, drop the example (artifact deletion = one directory).

## Acceptance criteria

- [ ] Landing page with ≥3 (target 5) real-PR examples, each meeting all five criteria.
- [ ] Each render self-contained, < ~2 MB, zero external requests.
- [ ] Card shows the alphabetical-vs-narrative contrast in text.
- [ ] CI builds the whole site from frozen artifacts with no API key.
- [ ] README's "See a live example" points at the gallery.
- [ ] Per-example attribution + license footer.

## Out of scope (v1)

Screenshot/video comparisons, analytics, search, dark mode (tracked as #19 — applies to
renders generally), interactive commenting on demo pages (serve-mode only feature).
