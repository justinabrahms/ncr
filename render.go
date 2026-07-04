package main

import (
	"fmt"
	"html/template"
	"strings"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

type layerInfo struct {
	name, color string
}

var layers = map[int]layerInfo{
	0: {"Contract", "#6b46c1"},
	1: {"Entrypoint", "#2563eb"},
	2: {"Application", "#0891b2"},
	3: {"Domain", "#059669"},
	4: {"Adapter", "#d97706"},
	5: {"Cross-cutting", "#64748b"},
	6: {"Tests/Docs", "#94a3b8"},
}

func badge(layer *int) template.HTML {
	if layer == nil {
		return ""
	}
	li, ok := layers[*layer]
	if !ok {
		li = layerInfo{fmt.Sprintf("L%d", *layer), "#64748b"}
	}
	return template.HTML(fmt.Sprintf(
		`<span class="badge" style="background:%s">%d %s</span>`, li.color, *layer, li.name))
}

// --- diff rendering (chroma twin of ncr/render.py) ---

func classOf(tt chroma.TokenType) string {
	for _, t := range []chroma.TokenType{tt, tt.SubCategory(), tt.Category()} {
		if c, ok := chroma.StandardTypes[t]; ok && c != "" {
			return c
		}
	}
	return ""
}

func pickLexer(language, path string) chroma.Lexer {
	var l chroma.Lexer
	if language != "" {
		l = lexers.Get(language)
	}
	if l == nil {
		l = lexers.Match(path)
	}
	if l == nil {
		l = lexers.Fallback
	}
	return chroma.Coalesce(l)
}

// highlightLines returns per-line HTML (token spans) for code, aligned to n lines.
func highlightLines(code, language, path string, n int) []string {
	esc := func(s string) string { return template.HTMLEscapeString(s) }
	fallback := func() []string {
		out := make([]string, 0, n)
		for _, l := range strings.Split(code, "\n") {
			out = append(out, esc(l))
		}
		return out
	}
	it, err := pickLexer(language, path).Tokenise(nil, code)
	if err != nil {
		return fallback()
	}
	lines := []string{""}
	for _, tok := range it.Tokens() {
		cls := classOf(tok.Type)
		for i, part := range strings.Split(tok.Value, "\n") {
			if i > 0 {
				lines = append(lines, "")
			}
			seg := esc(part)
			if cls != "" && seg != "" {
				seg = `<span class="` + cls + `">` + seg + `</span>`
			}
			lines[len(lines)-1] += seg
		}
	}
	if len(lines) == n+1 && lines[n] == "" { // chroma trailing newline
		lines = lines[:n]
	}
	if len(lines) != n {
		return fallback()
	}
	return lines
}

func strip(l string) string {
	if l != "" && (l[0] == '+' || l[0] == '-' || l[0] == ' ') {
		return l[1:]
	}
	return l
}

type diffItem struct {
	dl  *DiffLine
	sep bool
	wsn bool // whitespace-only noise (matches a counterpart on the other side)
}

func collapseWS(s string) string { return strings.Join(strings.Fields(s), " ") }

// wsNoise flags changed lines whose content is identical to a line on the other
// side once whitespace is collapsed — e.g. a realignment (gofmt) that rewrites a
// whole block with no real change. Returns a per-line mask.
func wsNoise(b Block) []bool {
	del, add := map[string]int{}, map[string]int{}
	for _, l := range b.Lines {
		c := collapseWS(strip(l.Text))
		switch l.Kind {
		case "del":
			del[c]++
		case "add":
			add[c]++
		}
	}
	out := make([]bool, len(b.Lines))
	for i, l := range b.Lines {
		c := collapseWS(strip(l.Text))
		out[i] = (l.Kind == "del" && add[c] > 0) || (l.Kind == "add" && del[c] > 0)
	}
	return out
}

func diffHTML(blocks []Block, language, path string) template.HTML {
	var items []diffItem
	hasNoise := false
	for bi := range blocks {
		if bi > 0 {
			items = append(items, diffItem{sep: true})
		}
		noise := wsNoise(blocks[bi])
		for li := range blocks[bi].Lines {
			items = append(items, diffItem{dl: &blocks[bi].Lines[li], wsn: noise[li]})
			if noise[li] {
				hasNoise = true
			}
		}
	}
	var codeLines []string
	for _, it := range items {
		if !it.sep {
			codeLines = append(codeLines, strip(it.dl.Text))
		}
	}
	hl := highlightLines(strings.Join(codeLines, "\n"), language, path, len(codeLines))

	var b strings.Builder
	b.WriteString(`<div class="diffwrap">`)
	if hasNoise {
		b.WriteString(`<button type="button" class="wstoggle">hide whitespace</button>`)
	}
	b.WriteString(`<pre class="chroma diff">`)
	k := 0
	for _, it := range items {
		if it.sep {
			b.WriteString(`<span class="l sep"><span class="gutter">⋯</span></span>`)
			continue
		}
		dl := it.dl
		code := hl[k]
		k++
		if code == "" {
			code = " "
		}
		cls, mark, side, lineNo := "ctx", " ", "RIGHT", dl.NewNo
		switch dl.Kind {
		case "add":
			cls, mark, side, lineNo = "add", "+", "RIGHT", dl.NewNo
		case "del":
			cls, mark, side, lineNo = "del", "-", "LEFT", dl.OldNo
		}
		if it.wsn {
			cls += " wsn"
		}
		// data-* attributes anchor a click to a GitHub review position.
		fmt.Fprintf(&b,
			`<span class="l %s" data-path="%s" data-side="%s" data-line="%d" data-text="%s"><span class="gutter">%s</span>%s</span>`,
			cls, template.HTMLEscapeString(path), side, lineNo,
			template.HTMLEscapeString(strip(dl.Text)), mark, code)
	}
	b.WriteString("</pre></div>")
	return template.HTML(b.String())
}

func chromaCSS() string {
	f := chromahtml.New(chromahtml.WithClasses(true))
	var b strings.Builder
	_ = f.WriteCSS(&b, styles.Get("monokai"))
	return b.String()
}

// --- assembly ---

type nodeView struct {
	ID          string
	Badge       template.HTML
	Sym, Teaser string
	Summary     template.HTML
	Detail      template.HTML
	Meta        string
	Calls       template.HTML
	Diff        template.HTML
}

type chapterView struct {
	Title   string
	Summary template.HTML
	Nodes   []nodeView
	Orphan  bool
}

type pageView struct {
	Title, PRTag      string
	CovText, CovClass string
	Overview          template.HTML
	CSS               template.CSS
	Chapters, Orphans []chapterView
	Interactive       bool // serve mode: include the commenting UI
}

func nodeViewOf(u Unit, blockByID map[string]Block, edges []Edge, unitSymbols map[string]string) nodeView {
	var blocks []Block
	for _, id := range u.Blocks {
		if b, ok := blockByID[id]; ok {
			blocks = append(blocks, b)
		}
	}
	sym := u.Symbol
	if sym == "" {
		sym = u.File
	}
	var detail template.HTML
	if u.Detail != "" {
		detail = template.HTML(`<div class="detail">` + string(mdRender(u.Detail)) + `</div>`)
	}
	meta := fmt.Sprintf("%s · %s · %s", u.File, strings.Join(u.Blocks, " "), u.LayerReason)
	return nodeView{
		ID:      u.ID,
		Badge:   badge(u.Layer),
		Sym:     sym,
		Teaser:  mdText(u.Summary),
		Summary: mdRender(u.Summary),
		Detail:  detail,
		Meta:    meta,
		Calls:   callsHTML(u.ID, edges, unitSymbols),
		Diff:    diffHTML(blocks, u.Language, u.File),
	}
}

func callsHTML(unitID string, edges []Edge, unitSymbols map[string]string) template.HTML {
	var parts []string
	for _, e := range edges {
		if e.From != unitID {
			continue
		}
		if e.Resolved {
			if sym, ok := unitSymbols[e.To]; ok {
				parts = append(parts, fmt.Sprintf(`<a href="#%s">%s</a>`, e.To, template.HTMLEscapeString(sym)))
			}
		} else {
			parts = append(parts, `<span class="ext">↳ into unchanged code</span>`)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return template.HTML(`<div class="calls">calls: ` + strings.Join(parts, ", ") + `</div>`)
}

func BuildHTML(plan ReadingPlan, index Index, interactive bool) ([]byte, error) {
	unitByID := map[string]Unit{}
	for _, u := range plan.Units {
		unitByID[u.ID] = u
	}
	blockByID := map[string]Block{}
	for _, b := range index.Blocks {
		blockByID[b.BlockID] = b
	}
	unitSymbols := map[string]string{}
	for _, u := range plan.Units {
		s := u.Symbol
		if s == "" {
			s = u.File
		}
		unitSymbols[u.ID] = s
	}

	var chapters []chapterView
	for _, ch := range plan.Chapters {
		var nodes []nodeView
		for _, n := range ch.Nodes {
			if u, ok := unitByID[n.Unit]; ok {
				nodes = append(nodes, nodeViewOf(u, blockByID, plan.Edges, unitSymbols))
			}
		}
		chapters = append(chapters, chapterView{
			Title: ch.Title, Summary: mdRender(ch.Summary), Nodes: nodes})
	}

	var orphans []chapterView
	for _, grp := range plan.Orphans {
		var nodes []nodeView
		for _, id := range grp.Units {
			if u, ok := unitByID[id]; ok {
				nodes = append(nodes, nodeViewOf(u, blockByID, plan.Edges, unitSymbols))
			}
		}
		name := fmt.Sprintf("L%d", grp.Layer)
		if li, ok := layers[grp.Layer]; ok {
			name = li.name
		}
		orphans = append(orphans, chapterView{
			Title:   "Orphans · " + name,
			Summary: template.HTML("Changed here but not called by anything else in this diff."),
			Nodes:   nodes, Orphan: true})
	}

	prTag := ""
	if plan.PRNumber != 0 {
		prTag = fmt.Sprintf(" · #%d", plan.PRNumber)
	}
	cov := plan.Coverage
	if cov == nil {
		cov = &Coverage{OK: true}
	}
	covText := fmt.Sprintf("%d/%d blocks placed", cov.Counts.Placed, cov.Counts.Indexed)
	covClass := "cov-bad"
	if cov.OK {
		covClass, covText = "cov-ok", covText+" ✓"
	} else {
		covText += " — see ⚠ Unplaced"
	}
	title := plan.Title
	if title == "" {
		title = "Narrative code review"
	}

	pv := pageView{
		Title: title, PRTag: prTag, CovText: covText, CovClass: covClass,
		Overview: mdRender(plan.Overview),
		CSS:      template.CSS(pageCSS + "\n" + chromaCSS()),
		Chapters: chapters, Orphans: orphans,
		Interactive: interactive,
	}

	var b strings.Builder
	if err := pageTmpl.Execute(&b, pv); err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
}
