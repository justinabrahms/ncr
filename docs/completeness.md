# Completeness & determinism

**Requirement:** the system must never silently forget a hunk, a changed line, or an
existing review comment. Reordering is allowed to be non-deterministic; *coverage is not.*

The LLM is a reordering and narration engine. It is **not trusted** with completeness and
**not trusted** with the code text itself. Both are owned by deterministic code.

## The three guarantees

1. **Every changed line is placed exactly once.** Checked at line granularity: a unit may
   reference a whole block (`"b07"`) or a sub-range of it (`"b07:1-20"`) so the narrative can
   split a big block, but the reconciler proves the segments tile every changed line of every
   block with no gaps and no overlaps. A gap → surfaced (auto-repaired into "Unplaced"), an
   overlap → flagged. ("Never split a function" is a prompt instruction; this line accounting
   is what makes splitting safe regardless.)
2. **Displayed code is verbatim.** The UI renders each hunk from the deterministic index,
   keyed by block id — never from LLM output. The model cannot alter, truncate, or
   invent a line of the diff, because it never emits diff lines.
3. **Every discussion anchor survives.** Existing PR review comments are mapped to their
   block; the reconciler asserts each anchored block is present and each comment is
   attached to the node covering it.

## Mechanism

### 1. Deterministic diff indexer (pre-LLM, no model)

Parse the unified diff into **change blocks**: a change block is a maximal run of added/
removed lines (context lines split blocks). Each gets a **stable id** and canonical coords.

```jsonc
// block-index.json — the source of truth
{
  "blocks": [
    {
      "blockId": "b07",                 // stable: index in file order, per PR
      "path": "internal/order/handler.go",
      "changeType": "modified",
      "oldStart": 40, "oldLines": 0,    // null side for pure adds/deletes
      "newStart": 40, "newLines": 12,
      "text": "@@ ... @@\n+func (h *OrderHandler) Place(...) {\n...",
      "sha": "sha256:…"                 // hash of `text`; detects any drift
    }
  ],
  "blockIds": ["b01","b02", "..."]       // the complete set the reconciler checks against
}
```

`blockId` is assigned deterministically (stable sort by path, then hunk order, then
position) so the same diff always yields the same ids. `sha` lets us assert the text the UI
renders is byte-identical to what was indexed.

### 2. LLM references ids, never copies code

The prompt is handed `block-index.json` and works in terms of `blockId`s. A `ChangeUnit`
carries `blocks: ["b07","b08"]` instead of raw hunk text. The model's contract:

> Every `blockId` in the index must appear in exactly one unit's `blocks`. If you cannot
> classify a block, put it in a unit anyway and set `layer: 5, uncertain: true` — do not
> omit it.

Because the model only emits short ids, "forgetting a hunk" degrades from *silent data
loss* to *a missing id in a set* — which the next step catches deterministically.

### 3. Deterministic reconciler (post-LLM, no model)

Pure function over `block-index.json` + the LLM's `reading-plan.json`:

```
placed      = ∪ unit.blocks  for unit in plan.units
missing     = index.blockIds \ placed          # forgotten by the model
duplicated  = ids appearing in >1 unit
unplacedUnits = units in neither a chapter nor orphans
danglingRefs  = unit.blocks ids not in the index   # model invented an id
commentGaps   = PR-comment anchor blocks not in `placed`
```

Report:

```jsonc
{
  "ok": false,
  "counts": { "indexed": 142, "placed": 139 },
  "missing": ["b73","b118","b119"],
  "duplicated": [], "unplacedUnits": [], "danglingRefs": [], "commentGaps": []
}
```

**On any failure we do not proceed silently.** Two responses, in order:

- **Auto-repair (always):** synthesize an `Unplaced` chapter containing the `missing`
  blocks verbatim from the index, grouped by file, flagged in the UI. Nothing is ever
  lost even on a bad model run — worst case the reader sees "3 blocks the organizer
  couldn't place," with the real code.
- **Targeted re-ask (optional):** re-invoke the model with *only* the missing blocks plus
  the current chapter list, asking where they belong. Merge. Re-run the reconciler. Cap
  the retries; if still missing, keep them in `Unplaced`.

The UI always shows a coverage badge: `142/142 blocks placed ✓` or `139/142 — 3 unplaced`.

## Why this makes single-shot safe

The scary property of "LLM-only single-shot" is unaccountable omission. We remove exactly
that: completeness is a set-equality assertion against a canonical index, and code fidelity
is a hash check. The model is free to be non-deterministic about *grouping and prose* — the
parts where variation is harmless — while the parts where variation is dangerous (did every
line get shown? is the shown line correct?) are deterministic and verified.

## Determinism knobs (secondary)

For run-to-run stability of the *narrative* (nice-to-have, not a guarantee):
temperature 0, a fixed `blockId` ordering, and a stable-sort tie-break everywhere the
prompt says "order by …". These reduce churn; the reconciler is what actually protects you.
