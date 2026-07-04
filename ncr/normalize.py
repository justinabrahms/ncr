"""Normalize model output into the canonical reading-plan schema.

Models don't reliably emit our exact JSON shape — they nest change units inside
chapters, rename `symbol`->`label`, use `overview`->`summary`, omit `file`, etc.
This coerces those common variations into the canonical form (flat `units[]` +
chapters with `nodes[{unit, depth}]`) so the reconciler and renderer work, and
fills each unit's `file`/`symbol` from the block index when the model left them
out. Deterministic; runs before reconcile.
"""

from __future__ import annotations


def _first(d: dict, *keys, default=None):
    for k in keys:
        v = d.get(k)
        if v not in (None, "", []):
            return v
    return default


def normalize_plan(plan: dict, index: dict) -> dict:
    blocks_by_id = {b["blockId"]: b for b in index.get("blocks", [])}

    if not plan.get("overview"):
        plan["overview"] = _first(plan, "overview", "summary", default="")

    units: list[dict] = list(plan.get("units") or [])
    by_id = {u.get("id"): u for u in units if u.get("id")}

    def ingest(raw: dict, fallback_id: str) -> str:
        uid = raw.get("id") or fallback_id
        if uid in by_id:  # already a flat unit; just fill gaps
            _fill(by_id[uid], raw, blocks_by_id)
            return uid
        u = _to_unit(raw, uid, blocks_by_id)
        units.append(u)
        by_id[uid] = u
        return uid

    for ci, ch in enumerate(plan.get("chapters") or []):
        nodes = ch.get("nodes")
        # canonical nodes already reference unit ids -> leave alone
        if nodes and all(isinstance(n, dict) and "unit" in n for n in nodes):
            for n in nodes:
                if n["unit"] in by_id:
                    _fill(by_id[n["unit"]], by_id[n["unit"]], blocks_by_id)
            continue
        inline = _first(ch, "changeUnits", "units", "nodes", default=[])
        ch["nodes"] = [
            {"unit": ingest(cu, f"u-c{ci}-{j}"), "depth": cu.get("depth", 0)}
            for j, cu in enumerate(inline)
        ]
        ch.pop("changeUnits", None)

    # orphans may also carry inline units instead of id references
    norm_orphans = []
    for grp in plan.get("orphans") or []:
        ids = []
        for item in grp.get("units", []):
            if isinstance(item, dict):
                ids.append(ingest(item, f"u-orphan-{len(units)}"))
            else:
                ids.append(item)
        norm_orphans.append({"layer": grp.get("layer"), "units": ids})
    plan["orphans"] = norm_orphans

    # ensure every already-flat unit has file/symbol filled from its blocks
    for u in units:
        _fill(u, u, blocks_by_id)

    plan["units"] = units
    plan.setdefault("edges", [])
    return plan


def _to_unit(raw: dict, uid: str, blocks_by_id: dict) -> dict:
    u = {
        "id": uid,
        "blocks": raw.get("blocks", []),
        "symbol": _first(raw, "symbol", "label", "name", default=""),
        "summary": raw.get("summary", ""),
        "layer": raw.get("layer"),
        "layerReason": _first(raw, "layerReason", "layer_reason", default=""),
        "references": raw.get("references", []),
    }
    if raw.get("detail"):
        u["detail"] = raw["detail"]
    _fill(u, raw, blocks_by_id)
    return u


def _fill(u: dict, raw: dict, blocks_by_id: dict) -> None:
    """Fill file / language / kind from the model or, failing that, the blocks."""
    blk = next((blocks_by_id[b] for b in u.get("blocks", []) if b in blocks_by_id), None)
    if not u.get("file"):
        u["file"] = _first(raw, "file", "path", default=blk["path"] if blk else "")
    if not u.get("language") and raw.get("language"):
        u["language"] = raw["language"]
    if not u.get("symbol"):
        u["symbol"] = _first(raw, "symbol", "label", "name", default="")
