"""Ingest a GitHub PR via the `gh` CLI (reuses the user's existing auth).

Everything here is read-only against GitHub. See docs/ingest.md.
"""

from __future__ import annotations

import base64
import json
import subprocess
from typing import Optional

MAX_FILE_BYTES = 200_000  # skip fetching huge files into context


def _gh(*args: str) -> str:
    r = subprocess.run(["gh", *args], capture_output=True, text=True)
    if r.returncode != 0:
        raise RuntimeError(f"gh {' '.join(args)} failed:\n{r.stderr.strip()}")
    return r.stdout


def _repo_slug(repo: Optional[str]) -> str:
    if repo:
        return repo
    return json.loads(_gh("repo", "view", "--json", "nameWithOwner"))["nameWithOwner"]


def get_pr_context(pr: int, repo: Optional[str] = None, fetch_files: bool = True) -> dict:
    """Return {diff, meta, files, comments} for a PR."""
    repo = _repo_slug(repo)
    repo_args = ["--repo", repo]

    diff = _gh("pr", "diff", str(pr), *repo_args)
    meta = json.loads(_gh(
        "pr", "view", str(pr), *repo_args,
        "--json", "title,body,number,headRefOid,baseRefName,headRefName,author,files",
    ))
    comments = json.loads(_gh(
        "api", f"repos/{repo}/pulls/{pr}/comments", "--paginate",
    ) or "[]")

    files: dict[str, str] = {}
    if fetch_files:
        head = meta.get("headRefOid", "")
        for f in meta.get("files", []):
            path = f.get("path")
            if not path:
                continue
            files[path] = _fetch_file(repo, path, head)

    return {"diff": diff, "meta": meta, "files": files, "comments": comments}


def _fetch_file(repo: str, path: str, ref: str) -> str:
    # ref goes in the query string: passing it via -f/-F would flip gh to POST,
    # which the contents API rejects.
    endpoint = f"repos/{repo}/contents/{path}"
    if ref:
        endpoint += f"?ref={ref}"
    try:
        payload = json.loads(_gh("api", endpoint))
    except RuntimeError:
        return ""  # deleted file or path unavailable at head
    if payload.get("encoding") != "base64":
        return ""
    raw = base64.b64decode(payload.get("content", ""))
    if len(raw) > MAX_FILE_BYTES:
        return f"(file too large: {len(raw)} bytes; omitted from context)"
    try:
        return raw.decode("utf-8")
    except UnicodeDecodeError:
        return "(binary file omitted)"


def anchor_comments(index: dict, comments: list) -> list[str]:
    """Map PR review comments to the block ids they land on (by path + new-side line)."""
    hits: list[str] = []
    for c in comments or []:
        path = c.get("path")
        line = c.get("line") or c.get("original_line")
        if not path or not line:
            continue
        for b in index.get("blocks", []):
            if b["path"] != path or b.get("newStart") is None:
                continue
            if b["newStart"] <= line < b["newStart"] + max(b["newLines"], 1):
                hits.append(b["blockId"])
                break
    return hits
