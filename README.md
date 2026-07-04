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

A single Go binary. It needs the [`gh`](https://cli.github.com/) CLI (for PR ingest) and
`ANTHROPIC_API_KEY` (for the reordering step).

```sh
ncr owner/name 812
```

It writes `out/review.html` and opens it. Build instructions, flags, the pipeline, and the
package layout are in [CONTRIBUTING.md](CONTRIBUTING.md).

## Non-negotiable: nothing gets forgotten

The reordering is LLM-driven, but the LLM is **not trusted with completeness or with the
code text.** A deterministic indexer splits the diff into stable-ID'd blocks; the model
only references those ids; a deterministic reconciler proves by set-equality that every
block was placed, and renders code verbatim from the index. So a model can't silently drop,
truncate, or alter a hunk. See `docs/completeness.md`.

## More

Building, running, the pipeline, and how the code is laid out:
[CONTRIBUTING.md](CONTRIBUTING.md). Design notes live in `docs/`
(`design.md`, `completeness.md`, `ingest.md`, `schema.md`), with the language choice in
`docs/adr-001-go-cli.md`.
