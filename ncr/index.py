"""Deterministic diff indexer.

Parses a unified diff into stable-ID'd *change blocks* — the completeness source
of truth. A change block is a maximal run of added/removed lines (context lines
break blocks). Every block gets a stable id and a content hash, so the reconciler
can prove coverage by set-equality and the renderer can show verbatim code that
the LLM never touched. See docs/completeness.md.
"""

from __future__ import annotations

import hashlib
import re
from dataclasses import dataclass, field, asdict
from typing import Optional

_HUNK_RE = re.compile(r"^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(.*)$")


@dataclass
class Block:
    block_id: str
    path: str
    change_type: str  # added | deleted | modified | renamed
    old_start: Optional[int]
    old_lines: int
    new_start: Optional[int]
    new_lines: int
    header: str  # the @@ ... @@ line this block came from
    text: str  # verbatim prefixed diff lines ("+"/"-"), newline-joined
    sha: str

    def to_dict(self) -> dict:
        # camelCase for the JSON contract in docs/schema.md
        return {
            "blockId": self.block_id,
            "path": self.path,
            "changeType": self.change_type,
            "oldStart": self.old_start,
            "oldLines": self.old_lines,
            "newStart": self.new_start,
            "newLines": self.new_lines,
            "header": self.header,
            "text": self.text,
            "sha": self.sha,
        }


@dataclass
class _FileDiff:
    path: str
    change_type: str
    body: list[str] = field(default_factory=list)


def _sha(text: str) -> str:
    return "sha256:" + hashlib.sha256(text.encode("utf-8")).hexdigest()


def _clean_path(new_path: Optional[str], old_path: Optional[str]) -> str:
    p = new_path if new_path and new_path != "/dev/null" else old_path
    p = p or "?"
    for prefix in ("a/", "b/"):
        if p.startswith(prefix):
            return p[len(prefix):]
    return p


def _split_files(diff: str) -> list[_FileDiff]:
    """Split a unified diff into per-file sections."""
    files: list[_FileDiff] = []
    cur: Optional[_FileDiff] = None
    old_path = None
    change_type = "modified"
    for line in diff.splitlines():
        if line.startswith("diff --git"):
            cur = None
            old_path = None
            change_type = "modified"
        elif line.startswith("new file mode"):
            change_type = "added"
        elif line.startswith("deleted file mode"):
            change_type = "deleted"
        elif line.startswith("rename from") or line.startswith("rename to"):
            change_type = "renamed"
        elif line.startswith("--- "):
            old_path = line[4:].strip()
        elif line.startswith("+++ "):
            cur = _FileDiff(path=_clean_path(line[4:].strip(), old_path),
                            change_type=change_type)
            files.append(cur)
        elif cur is not None:
            cur.body.append(line)
    return files


def index_diff(diff: str) -> list[Block]:
    """Parse a unified diff into an ordered list of change blocks with stable ids."""
    blocks: list[Block] = []
    counter = 0
    for fd in _split_files(diff):
        old_no = new_no = 0
        header = ""
        run: list[tuple[str, str]] = []  # (prefix, raw_line) for the current block
        run_old_start = run_new_start = 0

        def flush() -> None:
            nonlocal counter, run
            if not run:
                return
            counter += 1
            removed = sum(1 for t, _ in run if t == "-")
            added = sum(1 for t, _ in run if t == "+")
            text = "\n".join(raw for _, raw in run)
            blocks.append(
                Block(
                    block_id=f"b{counter:03d}",
                    path=fd.path,
                    change_type=fd.change_type,
                    old_start=run_old_start if removed else None,
                    old_lines=removed,
                    new_start=run_new_start if added else None,
                    new_lines=added,
                    header=header,
                    text=text,
                    sha=_sha(text),
                )
            )
            run = []

        for line in fd.body:
            m = _HUNK_RE.match(line)
            if m:
                flush()
                old_no = int(m.group(1))
                new_no = int(m.group(3))
                header = line
                continue

            tag = line[0] if line else " "
            if tag in ("+", "-"):
                if not run:
                    run_old_start, run_new_start = old_no, new_no
                run.append((tag, line))
                if tag == "+":
                    new_no += 1
                else:
                    old_no += 1
            elif tag == "\\":
                # "\ No newline at end of file" — attach, no line advance
                if run:
                    run.append((" ", line))
            else:  # context (space prefix) or blank line inside a hunk
                flush()
                old_no += 1
                new_no += 1
        flush()
    return blocks


def build_index(diff: str) -> dict:
    """Produce block-index.json (see docs/schema.md)."""
    blocks = index_diff(diff)
    return {
        "blocks": [b.to_dict() for b in blocks],
        "blockIds": [b.block_id for b in blocks],
    }
