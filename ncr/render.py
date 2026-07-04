"""Render a reading plan to a self-contained HTML page.

Progressive disclosure via native <details> (no JS). Code is joined from the
block index by id and shown verbatim — never from LLM output. See docs/design.md.
"""

from __future__ import annotations

import html
from typing import Optional

try:
    from pygments import highlight as _pyg_highlight
    from pygments.formatters import HtmlFormatter
    from pygments.lexers import get_lexer_by_name, guess_lexer_for_filename
    from pygments.util import ClassNotFound
    _PYG = True
    _FORMATTER = HtmlFormatter(nowrap=True, style="monokai")
    _PYG_CSS = HtmlFormatter(style="monokai").get_style_defs(".diff")
except ImportError:  # highlighting is optional; renderer degrades gracefully
    _PYG = False
    _PYG_CSS = ""

LAYERS = {
    0: ("Contract", "#6b46c1"),
    1: ("Entrypoint", "#2563eb"),
    2: ("Application", "#0891b2"),
    3: ("Domain", "#059669"),
    4: ("Adapter", "#d97706"),
    5: ("Cross-cutting", "#64748b"),
    6: ("Tests/Docs", "#94a3b8"),
}
RISK_COLOR = {"low": "#64748b", "medium": "#d97706", "high": "#dc2626"}


def _esc(s: str) -> str:
    return html.escape(s or "")


def _layer_badge(layer: Optional[int]) -> str:
    if layer is None:
        return ""
    name, color = LAYERS.get(layer, (f"L{layer}", "#64748b"))
    return f'<span class="badge" style="background:{color}">{layer} {name}</span>'


def _lexer(language: str, path: str):
    if not _PYG:
        return None
    if language:
        try:
            return get_lexer_by_name(language)
        except ClassNotFound:
            pass
    try:
        return guess_lexer_for_filename(path or "x.txt", "")
    except ClassNotFound:
        return None


def _highlight_lines(code: str, lexer) -> list[str]:
    """Return per-line highlighted HTML for `code` (no +/- prefix)."""
    if lexer is None:
        return [_esc(l) for l in code.split("\n")]
    out = _pyg_highlight(code, lexer, _FORMATTER)
    # nowrap formatter keeps newlines; a trailing newline yields a spurious last line
    lines = out.split("\n")
    if lines and lines[-1] == "":
        lines.pop()
    return lines


def _diff_html(text: str, language: str = "", path: str = "") -> str:
    lines = text.splitlines()
    prefixes = [l[:1] for l in lines]
    # highlight the block's code as a whole (so multi-line constructs resolve),
    # with the +/- marker column stripped first
    stripped = "\n".join(l[1:] if l[:1] in "+- " else l for l in lines)
    hl = _highlight_lines(stripped, _lexer(language, path))
    if len(hl) != len(lines):  # highlighter disagreed on line count; fall back safely
        hl = [_esc(l[1:] if l[:1] in "+- " else l) for l in lines]
    rows = []
    for prefix, code in zip(prefixes, hl):
        cls = {"+": "add", "-": "del"}.get(prefix, "ctx")
        mark = prefix if prefix in "+-" else " "
        rows.append(f'<span class="l {cls}"><span class="gutter">{mark}</span>{code}</span>')
    return '<pre class="diff">' + "\n".join(rows) + "</pre>"


def _node_html(unit: dict, blocks_by_id: dict, edges: list, unit_symbols: dict) -> str:
    code = "\n".join(blocks_by_id.get(b, {}).get("text", "") for b in unit.get("blocks", []))
    risk = unit.get("risk", "low")
    notes = unit.get("reviewNotes") or []
    notes_html = ""
    if notes:
        items = "".join(f"<li>{_esc(n)}</li>" for n in notes)
        notes_html = f'<div class="notes"><strong>Review carefully</strong><ul>{items}</ul></div>'
    why = f'<div class="why">{_esc(unit["why"])}</div>' if unit.get("why") else ""

    # outgoing call links
    calls = []
    for e in edges:
        if e.get("from") == unit["id"]:
            if e.get("resolved") and e.get("to") in unit_symbols:
                calls.append(f'<a href="#{e["to"]}">{_esc(unit_symbols[e["to"]])}</a>')
            elif not e.get("resolved"):
                calls.append('<span class="ext">↳ into unchanged code</span>')
    calls_html = f'<div class="calls">calls: {", ".join(calls)}</div>' if calls else ""

    diff = _diff_html(code, unit.get("language", ""), unit.get("file", ""))
    sym = _esc(unit.get("symbol") or unit.get("file", ""))
    blocks_tag = " ".join(unit.get("blocks", []))
    return f"""
<details id="{unit['id']}" class="node">
  <summary>
    {_layer_badge(unit.get('layer'))}
    <span class="risk" style="color:{RISK_COLOR.get(risk, '#64748b')}">●</span>
    <code class="sym">{sym}</code>
    <span class="one">{_esc(unit.get('summary', ''))}</span>
  </summary>
  <div class="body">
    <div class="meta">{_esc(unit.get('file',''))} · {blocks_tag} · {_esc(unit.get('layerReason',''))}</div>
    {why}
    {notes_html}
    {calls_html}
    {diff}
  </div>
</details>"""


def build_html(plan: dict, index: dict) -> str:
    blocks_by_id = {b["blockId"]: b for b in index.get("blocks", [])}
    units_by_id = {u["id"]: u for u in plan.get("units", [])}
    unit_symbols = {uid: (u.get("symbol") or u.get("file", "")) for uid, u in units_by_id.items()}
    edges = plan.get("edges", [])

    cov = plan.get("coverage") or {}
    counts = cov.get("counts", {})
    ok = cov.get("ok", True)
    badge_cls = "cov-ok" if ok else "cov-bad"
    cov_text = (f'{counts.get("placed", "?")}/{counts.get("indexed", "?")} blocks placed'
                + (" ✓" if ok else " — see ⚠ Unplaced"))

    sections = []
    for ch in plan.get("chapters", []):
        nodes = "".join(
            _node_html(units_by_id[n["unit"]], blocks_by_id, edges, unit_symbols)
            for n in ch.get("nodes", []) if n["unit"] in units_by_id
        )
        sections.append(f"""
<section class="chapter">
  <h2>{_esc(ch.get('title',''))}</h2>
  <p class="chsum">{_esc(ch.get('summary',''))}</p>
  {nodes}
</section>""")

    orphan_sections = []
    for grp in plan.get("orphans", []):
        layer = grp.get("layer")
        nodes = "".join(
            _node_html(units_by_id[uid], blocks_by_id, edges, unit_symbols)
            for uid in grp.get("units", []) if uid in units_by_id
        )
        name = LAYERS.get(layer, (f"L{layer}", ""))[0]
        orphan_sections.append(
            f'<section class="chapter orphan"><h2>Orphans · {_esc(name)}</h2>'
            f'<p class="chsum">Changed here but not called by anything else in this diff.</p>{nodes}</section>'
        )

    title = _esc(plan.get("title", "Narrative code review"))
    pr = plan.get("prNumber")
    pr_tag = f" · #{pr}" if pr else ""

    return f"""<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{title} — narrative review</title>
<style>{_CSS}
{_PYG_CSS}</style></head>
<body>
<header>
  <div class="titlebar">
    <h1>{title}<span class="prtag">{pr_tag}</span></h1>
    <span class="cov {badge_cls}">{cov_text}</span>
  </div>
  <p class="overview">{_esc(plan.get('overview',''))}</p>
  <div class="controls">
    <button onclick="document.querySelectorAll('details').forEach(d=>d.open=true)">Expand all</button>
    <button onclick="document.querySelectorAll('details').forEach(d=>d.open=false)">Collapse all</button>
  </div>
</header>
<main>
{''.join(sections)}
{''.join(orphan_sections)}
</main>
</body></html>"""


_CSS = """
:root{--fg:#1e293b;--muted:#64748b;--bg:#f8fafc;--card:#fff;--line:#e2e8f0}
*{box-sizing:border-box}
body{margin:0;font:15px/1.5 -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;color:var(--fg);background:var(--bg)}
header{padding:24px 32px;background:var(--card);border-bottom:1px solid var(--line);position:sticky;top:0;z-index:2}
.titlebar{display:flex;align-items:center;gap:16px;justify-content:space-between}
h1{font-size:20px;margin:0}
.prtag{color:var(--muted);font-weight:400}
.overview{max-width:70ch;color:#334155;margin:10px 0 0}
.controls{margin-top:12px}
.controls button{font-size:13px;padding:4px 10px;margin-right:8px;border:1px solid var(--line);background:var(--bg);border-radius:6px;cursor:pointer}
.cov{font-size:13px;padding:4px 10px;border-radius:99px;white-space:nowrap}
.cov-ok{background:#dcfce7;color:#166534}
.cov-bad{background:#fee2e2;color:#991b1b}
main{max-width:960px;margin:0 auto;padding:24px 32px}
.chapter{margin:0 0 28px}
.chapter h2{font-size:16px;margin:0 0 4px;padding-bottom:6px;border-bottom:2px solid var(--line)}
.orphan h2{color:var(--muted)}
.chsum{color:var(--muted);margin:0 0 12px}
.node{background:var(--card);border:1px solid var(--line);border-radius:8px;margin:8px 0;overflow:hidden}
.node summary{padding:10px 14px;cursor:pointer;display:flex;align-items:center;gap:10px;list-style:none}
.node summary::-webkit-details-marker{display:none}
.node[open] summary{border-bottom:1px solid var(--line);background:#fbfcfe}
.badge{color:#fff;font-size:11px;font-weight:600;padding:2px 8px;border-radius:99px;white-space:nowrap}
.risk{font-size:12px}
.sym{font-size:13px;background:var(--bg);padding:1px 6px;border-radius:4px}
.one{color:#334155;flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.body{padding:12px 14px}
.meta{font-size:12px;color:var(--muted);margin-bottom:8px}
.why{font-style:italic;color:#475569;margin-bottom:8px}
.notes{background:#fffbeb;border:1px solid #fde68a;border-radius:6px;padding:8px 12px;margin-bottom:10px}
.notes ul{margin:4px 0 0;padding-left:18px}
.calls{font-size:13px;color:var(--muted);margin-bottom:8px}
.calls a{color:#2563eb;text-decoration:none}
.ext{color:#94a3b8}
.diff{margin:0;padding:8px 0;background:#272822;border-radius:6px;overflow-x:auto;color:#f8f8f2;font:12.5px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace}
.diff .l{display:block;white-space:pre;padding-right:10px}
.diff .gutter{display:inline-block;width:1.6em;text-align:center;color:#75715e;user-select:none}
.diff .add{background:rgba(74,222,128,.14)}
.diff .add .gutter{color:#4ade80}
.diff .del{background:rgba(248,113,113,.14)}
.diff .del .gutter{color:#f87171}
"""
