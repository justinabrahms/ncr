# Data schemas

The inter-stage contract. `reading-plan.json` (the UI input) is the union of everything.
`block-index.json` (deterministic, pre-LLM) is the completeness source of truth — see
`docs/completeness.md`.

## Block (output of the deterministic indexer — the unit of completeness accounting)

```jsonc
{
  "blockId": "b07",                 // stable per PR; the id the LLM references
  "path": "internal/order/handler.go",
  "changeType": "modified",         // added|modified|deleted|renamed
  "oldStart": 40, "oldLines": 0,    // null side for pure adds/deletes
  "newStart": 40, "newLines": 12,
  "text": "+ ...",                   // verbatim changed (+/-) lines; what coverage counts
  "sha": "sha256:…",                // hash of text; asserts nothing drifted
  "contextBefore": [" ...", " ..."], // up to 3 surrounding unchanged lines, display only
  "contextAfter":  [" ...", " ..."]  // NOT part of text/sha, so coverage == changed lines
}
```

## ChangeUnit (output of Segment, enriched by later stages)

```jsonc
{
  "id": "u12",                       // stable id, referenced by edges
  "file": "internal/order/handler.go",
  "language": "go",
  "symbol": "OrderHandler.Place",    // fully-qualified where possible; "" for file-level
  "kind": "method",                  // function|method|class|type|const|config|migration|test|other
  "changeType": "modified",          // added|modified|deleted|renamed
  "blocks": ["b07", "b08"],          // block ids this unit covers (blocks are function-aligned;
                                     // group a symbol's blocks together) — the completeness link
  "startLine": 40, "endLine": 88,    // in the NEW file (post-diff); null for pure deletes
  "signature": "func (h *OrderHandler) Place(w, r) ",
  "references": ["OrderService.place", "decodeOrderRequest"], // names it calls/uses
  "imports": ["internal/order/service"],

  // added by Layer:
  "layer": 1,
  "layerReason": "HTTP handler registered on the /orders route",

  // added by Annotate (explanatory only — no critique/risk):
  "summary": "Adds a POST /orders handler that decodes the payload and calls the service.",
  "detail": "Entry point for the order flow; hands the decoded request to Service.Place."  // optional
}
```

## Edge (output of Graph)

```jsonc
{ "from": "u12", "to": "u34", "kind": "calls", "resolved": true }
// kind: calls|constructs|implements|reads|writes|references
// resolved=false means the callee is not in the diff (points into unchanged code)
```

## Chapter / ReadingPlan (output of Order + Annotate; the UI input)

```jsonc
{
  "prNumber": 812,
  "title": "Order placement endpoint",
  "overview": "Two new endpoints and a migration...",   // whole-PR narrative
  "chapters": [
    {
      "id": "c1",
      "title": "POST /orders — place an order",
      "summary": "Entry payload → validation → service → repo insert.",
      "rootUnit": "u12",
      "layerSpan": [1, 4],
      "nodes": [                     // ordered; depth = call distance from root
        { "unit": "u12", "depth": 0 },
        { "unit": "u34", "depth": 1 },
        { "unit": "u56", "depth": 2 }
      ]
    }
  ],
  "orphans": [                       // changed units with no in-diff caller, grouped by layer
    { "layer": 6, "units": ["u90", "u91"] }
  ],
  "units": [ /* ChangeUnit[] */ ],
  "edges": [ /* Edge[] */ ],
  "coverage": { /* CoverageReport, filled by the reconciler */ }
}
```

Note: `ChangeUnit` has no `hunk` field — the UI joins `unit.blocks` → `block-index.json`
`text` and renders that. LLM output never contains diff code.

## CoverageReport (output of the deterministic reconciler)

```jsonc
{
  "ok": false,
  "counts": { "indexed": 142, "placed": 139 },
  "missing": ["b73","b118","b119"],  // in the index, absent from the plan → auto-repaired into Unplaced
  "duplicated": [],                  // a block claimed by >1 unit
  "unplacedUnits": [],               // units in neither a chapter nor orphans
  "danglingRefs": [],                // unit.blocks ids not present in the index (model invented)
  "commentGaps": []                  // PR-comment anchor blocks missing from the plan
}
```
