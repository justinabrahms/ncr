"""Tests for the deterministic reconciler — the completeness guarantee.

The load-bearing test: drop a block from the plan and confirm it is rescued into
a visible Unplaced chapter rather than silently lost.
"""

import copy
import json
import pathlib
import sys

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[1]))

from ncr.index import build_index  # noqa: E402
from ncr.reconcile import reconcile  # noqa: E402

FIX = pathlib.Path(__file__).parent / "fixtures"


def _index():
    return build_index((FIX / "sample.diff").read_text())


def _plan():
    return json.loads((FIX / "sample-plan.json").read_text())


def test_complete_plan_passes_clean():
    rep = reconcile(_index(), _plan())["coverage"]
    assert rep["ok"] is True
    assert rep["counts"] == {"indexed": 4, "placed": 4}
    assert rep["missing"] == [] and rep["duplicated"] == [] and not rep["repaired"]


def test_dropped_block_is_rescued_not_lost():
    plan = _plan()
    # simulate the model forgetting the migration block b004
    plan["units"] = [u for u in plan["units"] if u["id"] != "u4"]
    plan["orphans"] = []
    out = reconcile(_index(), plan)
    rep = out["coverage"]
    assert "b004" in rep["missing"]
    assert rep["repaired"] is True
    # b004 now lives in a visible Unplaced chapter, still covered
    unplaced = next(c for c in out["chapters"] if c["id"] == "c-unplaced")
    rescued_units = {n["unit"] for n in unplaced["nodes"]}
    rescued_blocks = {b for u in out["units"] if u["id"] in rescued_units for b in u["blocks"]}
    assert "b004" in rescued_blocks
    # after repair, every indexed block is placed exactly once
    placed = [b for u in out["units"] for b in u["blocks"]]
    assert sorted(placed) == ["b001", "b002", "b003", "b004"]


def test_duplicate_block_is_flagged():
    plan = _plan()
    plan["units"][0]["blocks"].append("b002")  # b002 now in two units
    rep = reconcile(_index(), plan)["coverage"]
    assert "b002" in rep["duplicated"]
    assert rep["ok"] is False


def test_comment_anchor_gap_flagged():
    rep = reconcile(_index(), _plan(), comment_blocks=["b999"])["coverage"]
    assert rep["commentGaps"] == ["b999"]


if __name__ == "__main__":
    for name, fn in sorted(globals().items()):
        if name.startswith("test_"):
            fn()
            print(f"ok  {name}")
    print("all reconciler tests passed")
