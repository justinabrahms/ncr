# Demo gallery: vetted PR candidates

Input to the curation workflow in [demo-gallery-spec.md](demo-gallery-spec.md). Every
candidate below was verified against the hard criteria via the GitHub API (size window,
file count, cross-layer shape, license, generated-code noise) — **but none has been
rendered with ncr yet**; criterion 5 (a good plan, verified by a human) is the remaining
gate. Ranked best-first within each slot.

## TypeScript / JavaScript

### 1. supabase/supabase#47266 — feat(studio): add feature to rewrite queries
- https://github.com/supabase/supabase/pull/47266 | merged 2026-06-29 | Apache-2.0
- 15 files, +986 −229
- Pain: GitHub shows a test file first; the Logs Explorer page entrypoint is **#14 of 15**, the API route #13.
- Path: `pages/project/[ref]/logs/explorer/index.tsx` → `pages/api/ai/code/complete.ts` → AI prompt layer → state/data hooks → domain logic → UI panels.
- Why: 90k-star repo, real product feature (AI-assisted SQL rewriting), the two files a reader wants first sort dead last.
- Risks: one chunky UI file mid-narrative; ~21% tests.

### 2. triggerdotdev/trigger.dev#4110 — feat(cli,core): dev-only telnet log streaming
- https://github.com/triggerdotdev/trigger.dev/pull/4110 | merged 2026-07-02 | Apache-2.0
- 14 files, +519 −1
- Pain: four meta files (changeset, .env.example, server-changes notes) before any code; the implementation appears **after its own test file** (#13 vs #12).
- Path: supervisor/webapp entry wiring → env/config contract → logging layer → new `telnetLogServer.ts` adapter.
- Risks: half the diff is one 262-line file; infra-flavored feature; ~31% tests.

### 3. novuhq/novu#5541 — feat: set channels during trigger
- https://github.com/novuhq/novu/pull/5541 | merged 2024-05-12 | MIT for all touched files (repo shows "Other" due to dual MIT + enterprise `ee/` license — no `ee/` files touched; flag in attribution)
- 23 files, +397 −115
- Pain: GitHub shows an **IDE config file** first, then a 161-line e2e test; controller is #4, core usecases buried at #18–20.
- Path: controller + DTO → callers → shared types → generic usecases package → worker wiring.
- Risks: license nuance; ~33% tests; oldest (2024); NestJS ceremony.

## Python

### 1. zulip/zulip#35641 — Add E2EE test push notification support
- https://github.com/zulip/zulip/pull/35641 | merged 2025-08-13 | Apache-2.0
- 14 files, +509 −15
- Pain: GitHub shows `api_docs/changelog.md` first; route registration `zproject/urls.py` is **#14 of 14**, the view #13.
- Path: `zproject/urls.py` → `zerver/views/push_notifications.py` → `zerver/lib/push_notifications.py` → models/exceptions/middleware → OpenAPI + api_docs contract.
- Why: textbook URL → view → service → model → contract descent; Zulip's directory naming inverts the read order exactly.
- Risks: no DB migration; one +203 test file.

### 2. zulip/zulip#37087 — Backend support for admins updating user settings
- https://github.com/zulip/zulip/pull/37087 | merged 2026-01-02 | Apache-2.0
- 15 files, +489 −81
- Pain: the endpoint (`zerver/views/user_settings.py`, +100) is **#15 of 15 — literally last**, behind five test files.
- Path: views → `zerver/actions/user_settings.py` service layer → lib/domain + cache invalidation → OpenAPI contract.
- Risks: no schema change; 6 of 15 files are tests.

### 3. saleor/saleor#19100 — Add channel stock events
- https://github.com/saleor/saleor/pull/19100 | merged 2026-04-16 | BSD-3-Clause
- 15 files, +1064 −14
- Pain: GitHub opens on a **200-line generated `schema.graphql` dump**; the event-emission entrypoint is #11 of 15.
- Path: warehouse webhook emitter → plugin dispatch → event contract/transport → GraphQL subscription types → generated schema.
- Risks: event-plumbing shape (less canonical than request→DB); ~47% tests; generated schema must demote cleanly.

## Go

### 1. go-gitea/gitea#37500 — feat(actions): add job summaries (GITHUB_STEP_SUMMARY)
- https://github.com/go-gitea/gitea/pull/37500 | merged 2026-06-08 | MIT
- 18 files, +683 −23
- Pain: GitHub shows the 207-line storage model first, then two DB migrations and a locale file, all before anything that uses them; the runner-upload endpoint is #6 of 18.
- Path: `routers/api/actions/job_summary.go` (runner upload) → model + migration (persist) → web handler → template → Vue component (render).
- Why: genuinely full-stack (runner API → DB → web → UI) in a repo every Go dev knows.
- Risks: ~105 lines of Vue/TS dilutes "pure Go"; two entry directions (upload vs. view) — chaptering must pick a spine.

### 2. miniflux/v2#4372 — feat(api): unread/starred entry-ID list endpoints
- https://github.com/miniflux/v2/pull/4372 | merged 2026-06-12 | Apache-2.0
- 9 files, +578 −4
- Pain: GitHub shows the **Go client SDK first** (consumer before the endpoints exist); route registration #4, handlers #7 of 9.
- Path: `internal/api/api.go` (routes) → handlers → model → storage query builder → client SDK as epilogue.
- Why: textbook route → handler → storage with zero generated code.
- Risks: 9 files (small end); ~425 of 582 lines are tests.

### 3. go-gitea/gitea#37995 — feat(api): token introspection + self-deletion endpoint
- https://github.com/go-gitea/gitea/pull/37995 | merged 2026-06-14 | MIT
- 12 files, +437 −69
- Pain: low-level token model (+test) first; route registration #4, new handler #6 of 12.
- Path: route registration → token handlers → auth services → access-token model → API contract structs.
- Risks: ~38% of changed lines are generated swagger templates — the demo relies on ncr demoting them well (arguably a feature to show off).

## Java / Kotlin

### 1. keycloak/keycloak#48668 — [OID4VCI] credentials CRUD (DB + admin REST)
- https://github.com/keycloak/keycloak/pull/48668 | merged 2026-05-05 | Apache-2.0
- 18 files, +843 −0 (all additions)
- Pain: GitHub shows a DTO first; the admin REST endpoint is **#14 of 18**, while the JPA entity, Liquibase migration, and persistence.xml land at #5–8 — the reader gets the database bottom-up before seeing the API.
- Path: REST resources → SPI contracts + model↔representation mapping → JPA provider + entity + migration → Infinispan cache + storage adapters → tests.
- Why: textbook enterprise descent scattered across five Maven modules; alphabetical order shuffles layers near-randomly.
- Risks: chapter grouping must key off module prefixes; client and server classes share a name (narrative gold or a grouping trap — verify the plan).

### 2. opensearch-project/OpenSearch#14735 — Add Delete QueryGroup API logic
- https://github.com/opensearch-project/OpenSearch/pull/14735 | merged 2024-08-22 | Apache-2.0
- 17 files, +636 −15
- Pain: CHANGELOG first; REST handler #8 of 17; the API contract JSON dangles at #15.
- Path: rest-api-spec contract → RestAction → Action/Request → TransportAction → persistence service → cluster metadata; plugin wiring.
- Risks: ~40% tests; Rest/Transport idiom reads less universally than Spring's controller→service→repo.

### 3. keycloak/keycloak#49872 — Add sort parameters to Client Admin API v2
- https://github.com/keycloak/keycloak/pull/49872 | merged 2026-06-25 | Apache-2.0
- 14 files, +641 −21
- Pain: GitHub opens on a **generated `openapi.json` dump**; the JAX-RS interface is #6, impl #9, service #10.
- Path: API interface + types → default impl → service (sort/slice logic) → OpenAPI doc filter → tests.
- Risks: ~48% tests; no DB layer — weakest descent of the three.

## Rust

### 1. meilisearch/meilisearch#6193 — Add `POST /tasks/compact`
- https://github.com/meilisearch/meilisearch/pull/6193 | merged 2026-03-19 | MIT (repo is MIT AND BUSL-1.1; none of this PR's files carry the EE marker — verified by header inspection)
- 16 files, +321 −5
- Pain: crate-name sorting puts `index-scheduler` (LMDB compaction internals) first; the new route handler in the `meilisearch` server crate is **#9 of 16**.
- Path: route + handler → API-key action (auth) → experimental-feature gate → index-scheduler compaction → tests + workload file.
- Why: endpoint → permission → feature-flag → storage-engine descent that alphabetical order literally inverts.
- Risks: 321 lines is the floor of the band; ~40% of files are tests.

### 2. typst/typst#6357 — Add bleed support to page layout
- https://github.com/typst/typst/pull/6357 | merged 2026-05-26 | Apache-2.0
- 22 files, +394 −125
- Pain: trivial plumbing diffs (`typst-bundle`, `typst-cli`) and layout-engine internals sort before `typst-library/src/layout/page.rs` (+155, defines the user-facing `bleed` parameter) at **#7 of 22**.
- Path: library contract (page.rs + types) → layout engine (run/finalize/document) → three parallel export backends (pdf, render, svg) → CLI plumbing → test refs.
- Why: one concept fanning from a single definition through an engine into three adapters — "contract first, adapters last" in its purest form.
- Risks: 2 binary ref PNGs + 2 hashes.txt (generated test refs — must demote to an appendix); backends are parallel siblings rather than a strict descent.

### 3. astral-sh/uv#19605 — Add `uv check` to run `ty` from uv
- https://github.com/astral-sh/uv/pull/19605 | merged 2026-05-29 | Apache-2.0/MIT
- 13 files, +1,154 −3
- Pain: GitHub shows `uv-bin-install` (the bottom-layer binary-fetch adapter, +155) **on page one**, before the reader learns a new `uv check` command exists; the command implementation is #6, dispatch #8.
- Path: CLI surface (+192) → dispatch + settings → `commands/project/check.rs` core (+347) → bin-install adapter → integration tests + snapshots.
- Risks: the CLI contract itself sorts #2 (pain rests on the adapter-first inversion, not a buried entrypoint); ~280 lines of snapshot tests.

## Next step

Per the spec's curation workflow: render the chosen candidate per slot locally
(`ncr owner/repo N --static`, ~$0.30 each), read the plan critically, then freeze
`demos/<slug>/{ingest.json,plan.json,meta.yaml}`.
