package main

import (
	"html"
	"html/template"
	"regexp"
	"strings"
)

// Tiny, safe markdown — the Go twin of ncr/md.py. Escape-first; only `code` and
// **bold** (no single-* italics, since identifiers like getLong* have stray *).

var (
	reCode    = regexp.MustCompile("`([^`]+)`")
	reBold    = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reMarkers = regexp.MustCompile("[`*]+")
)

func emph(s string) string {
	return reBold.ReplaceAllString(s, "<strong>$1</strong>")
}

func mdInline(raw string) template.HTML {
	esc := html.EscapeString(raw)
	var b strings.Builder
	last := 0
	for _, m := range reCode.FindAllStringSubmatchIndex(esc, -1) {
		b.WriteString(emph(esc[last:m[0]]))
		b.WriteString("<code>" + esc[m[2]:m[3]] + "</code>")
		last = m[1]
	}
	b.WriteString(emph(esc[last:]))
	return template.HTML(b.String())
}

func mdRender(raw string) template.HTML {
	paras := regexp.MustCompile(`\n\s*\n`).Split(strings.TrimSpace(raw), -1)
	if len(paras) <= 1 {
		return mdInline(raw)
	}
	var b strings.Builder
	for _, p := range paras {
		if strings.TrimSpace(p) == "" {
			continue
		}
		b.WriteString("<p>" + string(mdInline(p)) + "</p>")
	}
	return template.HTML(b.String())
}

// mdText strips markdown markers for the one-line teaser; html/template escapes it.
func mdText(raw string) string {
	return reMarkers.ReplaceAllString(raw, "")
}
