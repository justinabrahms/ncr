"""Tests for the deterministic diff indexer.

Run: python -m pytest tests/  (or: python tests/test_index.py for a quick check)
The key property: every changed line is captured in exactly one block, and ids
are stable and gap-free.
"""

import pathlib
import sys

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[1]))

from ncr.index import build_index, index_diff  # noqa: E402

FIXTURE = pathlib.Path(__file__).parent / "fixtures" / "sample.diff"


def _diff() -> str:
    return FIXTURE.read_text()


def test_ids_are_stable_and_gap_free():
    idx = build_index(_diff())
    ids = idx["blockIds"]
    assert ids == [f"b{i:03d}" for i in range(1, len(ids) + 1)]
    # deterministic: same input -> same ids
    assert build_index(_diff())["blockIds"] == ids


def test_every_changed_line_is_covered_once():
    diff = _diff()
    blocks = index_diff(diff)
    covered = "\n".join(b.text for b in blocks).splitlines()
    covered_changes = [l for l in covered if l and l[0] in "+-"]
    # every +/- line in the diff (excluding file headers ---/+++) lands in a block
    expected = [
        l for l in diff.splitlines()
        if l[:1] in "+-" and not l.startswith(("+++", "---"))
    ]
    assert sorted(covered_changes) == sorted(expected)


def test_block_coords_point_at_new_lines():
    blocks = index_diff(_diff())
    place = next(b for b in blocks if "func (h *OrderHandler) Place" in b.text)
    # Block starts at the added blank line (new-file line 22), then Place; 14 added lines.
    # The leading blank "+" line is a real added line and must be captured (completeness).
    assert place.new_start == 22
    assert place.old_lines == 0
    assert place.new_lines == 14


def test_new_file_blocks_have_no_old_side():
    blocks = index_diff(_diff())
    svc = [b for b in blocks if b.path == "internal/order/service.go"]
    assert svc and all(b.old_start is None and b.old_lines == 0 for b in svc)
    assert all(b.change_type == "added" for b in svc)


if __name__ == "__main__":
    test_ids_are_stable_and_gap_free()
    test_every_changed_line_is_covered_once()
    test_block_coords_point_at_new_lines()
    test_new_file_blocks_have_no_old_side()
    idx = build_index(_diff())
    print(f"OK — {len(idx['blockIds'])} blocks: {', '.join(idx['blockIds'])}")
    for b in idx["blocks"]:
        loc = f"new:{b['newStart']}" if b["newStart"] else f"old:{b['oldStart']}"
        print(f"  {b['blockId']}  {b['changeType']:9}  {b['path']:34} {loc}")
