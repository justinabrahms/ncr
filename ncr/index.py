"""Deterministic diff indexer.

Parses a unified diff into stable-ID'd *change blocks* — the completeness source
of truth. A change block is a maximal run of added/removed lines (context lines
break blocks). Every block gets a stable id and a content hash, so the reconciler
can prove coverage by set-equality and the renderer can show verbatim code that
the LLM never touched. See docs/completeness.md.

Each block also carries up to CONTEXT surrounding unchanged lines (contextBefore/
contextAfter) purely for display — they are NOT part of `text`/`sha`, so coverage
accounting stays exactly the changed lines.
"""

from __future__ import annotations

import hashlib
import re
from dataclasses import dataclass, field, asdict
from typing import Optional

_HUNK_RE = re.compile(r"^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(.*)$")
CONTEXT = 3  # unchanged lines to show on each side of a block


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
    context_before: list = field(default_factory=list)  # raw " " lines, display only
    context_after: list = field(default_factory=list)

    def to_dict(self) -> dict:
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
            "contextBefore": self.context_before,
            "contextAfter": self.context_after,
        }


@dataclass
class _FileDiff:
    path: str
    change_type: str
    body: list = field(default_factory=list)


@dataclass
class _Rec:
    kind: str  # ctx | add | del
    raw: str
    old_no: int
    new_no: int


def _sha(text: str) -> str:
    return "sha256:" + hashlib.sha256(text.encode("utf-8")).hexdigest()


def _clean_path(new_path: Optional[str], old_path: Optional[str]) -> str:
    p = new_path if new_path and new_path != "/dev/null" else old_path
    p = p or "?"
    for prefix in ("a/", "b/"):
        if p.startswith(prefix):
            return p[len(prefix):]
    return p


def _split_files(diff: str) -> list:
    """Split a unified diff into per-file sections."""
    files: list = []
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


def _records(body: list) -> list:
    """Flatten a file body's hunks into typed line records with file line numbers."""
    recs: list = []
    old_no = new_no = 0
    header = ""
    for line in body:
        m = _HUNK_RE.match(line)
        if m:
            old_no, new_no = int(m.group(1)), int(m.group(3))
            header = line
            recs.append(_Rec("hunk", header, 0, 0))
            continue
        tag = line[0] if line else " "
        if tag == "+":
            recs.append(_Rec("add", line, 0, new_no)); new_no += 1
        elif tag == "-":
            recs.append(_Rec("del", line, old_no, 0)); old_no += 1
        elif tag == "\\":
            continue  # "\ No newline at end of file" — not a code line
        else:
            recs.append(_Rec("ctx", line, old_no, new_no)); old_no += 1; new_no += 1
    return recs


def index_diff(diff: str) -> list:
    """Parse a unified diff into an ordered list of change blocks with stable ids."""
    blocks: list = []
    counter = 0
    for fd in _split_files(diff):
        recs = _records(fd.body)
        # find the header governing each record (nearest preceding "hunk")
        header = ""
        i = 0
        while i < len(recs):
            r = recs[i]
            if r.kind == "hunk":
                header = r.raw
                i += 1
                continue
            if r.kind == "ctx":
                i += 1
                continue
            # start of a changed run
            j = i
            while j < len(recs) and recs[j].kind in ("add", "del"):
                j += 1
            run = recs[i:j]
            counter += 1
            removed = [x for x in run if x.kind == "del"]
            added = [x for x in run if x.kind == "add"]
            text = "\n".join(x.raw for x in run)
            before = [x.raw for x in _neighbors(recs, i - 1, -1)]
            after = [x.raw for x in _neighbors(recs, j, 1)]
            blocks.append(Block(
                block_id=f"b{counter:03d}",
                path=fd.path,
                change_type=fd.change_type,
                old_start=removed[0].old_no if removed else None,
                old_lines=len(removed),
                new_start=added[0].new_no if added else None,
                new_lines=len(added),
                header=header,
                text=text,
                sha=_sha(text),
                context_before=before,
                context_after=after,
            ))
            i = j
    return blocks


def _neighbors(recs: list, start: int, step: int) -> list:
    """Up to CONTEXT consecutive ctx records from `start` going `step` direction."""
    out: list = []
    k = start
    while 0 <= k < len(recs) and recs[k].kind == "ctx" and len(out) < CONTEXT:
        out.append(recs[k])
        k += step
    return out[::-1] if step < 0 else out


def build_index(diff: str) -> dict:
    """Produce block-index.json (see docs/schema.md)."""
    blocks = index_diff(diff)
    return {
        "blocks": [b.to_dict() for b in blocks],
        "blockIds": [b.block_id for b in blocks],
    }
