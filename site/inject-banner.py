#!/usr/bin/env python3
"""Insert the demo banner into a rendered ncr page.

Usage: inject-banner.py <render.html> <meta.yaml>

Runs at site-build time (pages.yml) so the renderer itself stays demo-agnostic.
The banner links back to the gallery and to the real PR, plus the meta.yaml
`notice` line collapsed behind a <details> so it never competes with the render.
Stdlib only: the meta.yaml files are flat `key: value` / `key: >-` block
scalars, so a tiny hand parser beats a yaml dependency.
"""

import html
import sys


def parse_meta(path):
    """Flat YAML subset: `key: value`, quoted strings, and `>-` folded blocks."""
    meta, key = {}, None
    for line in open(path, encoding="utf-8"):
        if line.startswith((" ", "\t")):  # continuation of a folded block
            if key:
                meta[key] = (meta[key] + " " + line.strip()).strip()
            continue
        if ":" not in line:
            key = None
            continue
        key, _, val = line.partition(":")
        key, val = key.strip(), val.strip()
        if val == ">-" or val == ">":
            meta[key] = ""
        else:
            if len(val) >= 2 and val[0] == val[-1] and val[0] in "\"'":
                val = val[1:-1]
            meta[key] = val
            key = None
    return meta


def main():
    render_path, meta_path = sys.argv[1], sys.argv[2]
    meta = parse_meta(meta_path)
    repo, pr, url = meta["repo"], meta["pr"], meta["url"]
    notice = meta.get("notice", "")

    # Inline <style> (not style="") so the banner can follow the page's
    # light/dark scheme; the render's own CSS variables are already present.
    banner = (
        '<style>'
        '.ncr-demo-banner{font:13px/1.5 -apple-system,BlinkMacSystemFont,'
        "'Segoe UI',sans-serif;background:var(--card);color:var(--muted);"
        'border-bottom:1px solid var(--line);padding:7px 32px}'
        '.ncr-demo-banner a{color:inherit;text-decoration:underline}'
        '.ncr-demo-banner details{display:inline}'
        '.ncr-demo-banner summary{display:inline;cursor:pointer}'
        '</style>'
        '<div class="ncr-demo-banner">ncr demo · '
        f'<a href="{html.escape(url, quote=True)}">{html.escape(repo)}#{html.escape(pr)}</a>'
        ' on GitHub · <a href="index.html">&larr; all examples</a>'
    )
    if notice:
        banner += (
            ' · <details><summary>what to notice</summary> '
            f'{html.escape(notice)}</details>'
        )
    banner += '</div>'

    page = open(render_path, encoding="utf-8").read()
    if "ncr-demo-banner" in page:
        sys.exit(f"{render_path}: banner already present")
    # The render's body tag carries attributes (<body data-ns="#N">).
    end = page.find(">", page.find("<body"))
    if end < 0:
        sys.exit(f"{render_path}: no <body> tag to inject after")
    page = page[: end + 1] + banner + page[end + 1 :]
    open(render_path, "w", encoding="utf-8").write(page)


if __name__ == "__main__":
    main()
