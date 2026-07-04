"""Deterministic reconciler.

Proves the LLM's reading plan covers every change block, by set-equality against
the block index. Any miss is auto-repaired into a visible "Unplaced" chapter so
nothing is ever lost, even on a bad model run. No model involved. See
docs/completeness.md.
"""

from __future__ import annotations

from collections import Counter
from typing import Optional


def _placed_block_counts(plan: dict) -> Counter:
    c: Counter = Counter()
    for unit in plan.get("units", []):
        for bid in unit.get("blocks", []):
            c[bid] += 1
    return c


def _unit_placements(plan: dict) -> Counter:
    """How many times each unit id is placed in a chapter or orphan group."""
    c: Counter = Counter()
    for ch in plan.get("chapters", []):
        for node in ch.get("nodes", []):
            c[node["unit"]] += 1
    for grp in plan.get("orphans", []):
        for uid in grp.get("units", []):
            c[uid] += 1
    return c


def reconcile(index: dict, plan: dict, comment_blocks: Optional[list[str]] = None) -> dict:
    """Verify + auto-repair coverage. Mutates and returns `plan` with a `coverage` key."""
    index_ids = list(index.get("blockIds", []))
    index_set = set(index_ids)
    by_id = {b["blockId"]: b for b in index.get("blocks", [])}

    placed = _placed_block_counts(plan)
    placed_set = set(placed)

    missing = [bid for bid in index_ids if bid not in placed_set]  # index order
    duplicated = sorted(b for b, n in placed.items() if n > 1)
    dangling = sorted(placed_set - index_set)

    unit_ids = {u["id"] for u in plan.get("units", [])}
    placements = _unit_placements(plan)
    unplaced_units = sorted(uid for uid in unit_ids if placements[uid] == 0)

    comment_blocks = comment_blocks or []
    comment_gaps = [b for b in comment_blocks if b not in placed_set]

    if missing:
        _auto_repair(plan, missing, by_id)
        # missing blocks are now placed; recompute the derived sets
        placed = _placed_block_counts(plan)
        placed_set = set(placed)

    plan["coverage"] = {
        "ok": not (missing or duplicated or dangling or unplaced_units or comment_gaps),
        "counts": {"indexed": len(index_ids), "placed": len(placed_set & index_set)},
        "missing": missing,
        "duplicated": duplicated,
        "unplacedUnits": unplaced_units,
        "danglingRefs": dangling,
        "commentGaps": comment_gaps,
        "repaired": bool(missing),
    }
    return plan


def _auto_repair(plan: dict, missing: list[str], by_id: dict) -> None:
    """Wrap missing blocks in synthetic units and a visible Unplaced chapter."""
    units = plan.setdefault("units", [])
    existing = {u["id"] for u in units}
    nodes = []
    n = 0
    for bid in missing:
        n += 1
        uid = f"u-unplaced-{n}"
        while uid in existing:
            n += 1
            uid = f"u-unplaced-{n}"
        existing.add(uid)
        blk = by_id.get(bid, {})
        units.append({
            "id": uid,
            "file": blk.get("path", "?"),
            "symbol": "",
            "kind": "other",
            "changeType": blk.get("changeType", "modified"),
            "blocks": [bid],
            "layer": 5,
            "layerReason": "auto-repair: organizer did not place this block",
            "uncertain": True,
            "summary": f"Block {bid} the organizer left unplaced — review directly.",
            "reviewNotes": [],
            "risk": "medium",
        })
        nodes.append({"unit": uid, "depth": 0})

    plan.setdefault("chapters", []).append({
        "id": "c-unplaced",
        "title": "⚠ Unplaced changes",
        "summary": (f"{len(missing)} change block(s) the organizer could not place. "
                    "Shown verbatim so nothing is lost."),
        "rootUnit": nodes[0]["unit"] if nodes else None,
        "layerSpan": [5, 5],
        "nodes": nodes,
    })
