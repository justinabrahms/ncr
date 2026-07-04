"""Content-addressed cache for intermediate artifacts.

Lets you iterate on presentation (reconcile + render) without re-spending API
credits: the expensive plan step is keyed by a hash of the exact prompt, so an
unchanged PR + unchanged prompt is a cache hit and makes no API call. Ingest
(network, no credits) is cached too, keyed by repo#pr.

Cache dir: $NCR_CACHE_DIR or ./.ncr-cache. Delete it, or pass --refresh, to bust.
"""

from __future__ import annotations

import hashlib
import json
import os
import re
from pathlib import Path
from typing import Optional


def cache_dir() -> Path:
    d = Path(os.environ.get("NCR_CACHE_DIR", ".ncr-cache"))
    d.mkdir(parents=True, exist_ok=True)
    return d


def digest(*parts: str) -> str:
    """Stable short hash of the given strings (order-sensitive)."""
    h = hashlib.sha256("\0".join(parts).encode("utf-8")).hexdigest()
    return h[:16]


def _path(name: str) -> Path:
    safe = re.sub(r"[^A-Za-z0-9._#-]", "_", name)
    return cache_dir() / f"{safe}.json"


def load(name: str) -> Optional[dict]:
    p = _path(name)
    if not p.exists():
        return None
    try:
        return json.loads(p.read_text())
    except (ValueError, OSError):
        return None  # corrupt/partial cache entry — treat as a miss


def save(name: str, obj: dict) -> Path:
    p = _path(name)
    p.write_text(json.dumps(obj))
    return p
