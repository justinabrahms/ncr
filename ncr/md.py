"""Tiny, safe markdown for the model's prose fields.

The model writes `inline code`, **bold**, and paragraphs. We render just that
subset — escape-first so raw HTML can't inject, and deliberately no single-`*`
italics (identifiers like `getLong*` contain stray asterisks). Diffs are
highlighted separately and never pass through here.
"""

from __future__ import annotations

import html
import re

_CODE = re.compile(r"`([^`]+)`")
_BOLD = re.compile(r"\*\*(.+?)\*\*", re.S)
_MARKERS = re.compile(r"[`*]+")


def _emph(s: str) -> str:
    return _BOLD.sub(r"<strong>\1</strong>", s)


def render_inline(raw: str) -> str:
    """Escape, then apply `code` and **bold** (code spans shield their contents)."""
    esc = html.escape(raw or "")
    out, pos = [], 0
    for m in _CODE.finditer(esc):
        out.append(_emph(esc[pos:m.start()]))
        out.append("<code>" + m.group(1) + "</code>")
        pos = m.end()
    out.append(_emph(esc[pos:]))
    return "".join(out)


def render(raw: str) -> str:
    """Inline rendering, splitting blank-line-separated paragraphs into <p>."""
    paras = re.split(r"\n\s*\n", (raw or "").strip())
    if len(paras) <= 1:
        return render_inline(raw)
    return "".join(f"<p>{render_inline(p)}</p>" for p in paras if p.strip())


def to_text(raw: str) -> str:
    """Plain, escaped text with markdown markers removed — for the one-line teaser."""
    return html.escape(_MARKERS.sub("", raw or ""))
