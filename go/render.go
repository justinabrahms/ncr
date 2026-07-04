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

const sep = "\x00sep"

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

func diffHTML(blocks []Block, language, path string) template.HTML {
	var lines []string
	for i, blk := range blocks {
		if i > 0 {
			lines = append(lines, sep)
		}
		lines = append(lines, blk.ContextBefore...)
		lines = append(lines, strings.Split(blk.Text, "\n")...)
		lines = append(lines, blk.ContextAfter...)
	}
	var codeLines []string
	for _, l := range lines {
		if l != sep {
			codeLines = append(codeLines, strip(l))
		}
	}
	hl := highlightLines(strings.Join(codeLines, "\n"), language, path, len(codeLines))

	var b strings.Builder
	b.WriteString(`<pre class="chroma diff">`)
	k := 0
	for _, l := range lines {
		if l == sep {
			b.WriteString(`<span class="l sep"><span class="gutter">⋯</span></span>`)
			continue
		}
		code := hl[k]
		k++
		cls, mark := "ctx", " "
		if l != "" && l[0] == '+' {
			cls, mark = "add", "+"
		} else if l != "" && l[0] == '-' {
			cls, mark = "del", "-"
		}
		if code == "" {
			code = " "
		}
		b.WriteString(`<span class="l ` + cls + `"><span class="gutter">` + mark + `</span>` + code + `</span>`)
	}
	b.WriteString("</pre>")
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
	Diff        template.HTML
}

type chapterView struct {
	Title   string
	Summary template.HTML
	Nodes   []nodeView
	Orphan  bool
}

type pageView struct {
	Title, PRTag       string
	CovText, CovClass  string
	Overview           template.HTML
	CSS                template.CSS
	Chapters, Orphans  []chapterView
}

func nodeViewOf(u Unit, blockByID map[string]Block) nodeView {
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
		Diff:    diffHTML(blocks, u.Language, u.File),
	}
}

func BuildHTML(plan ReadingPlan, index Index) ([]byte, error) {
	unitByID := map[string]Unit{}
	for _, u := range plan.Units {
		unitByID[u.ID] = u
	}
	blockByID := map[string]Block{}
	for _, b := range index.Blocks {
		blockByID[b.BlockID] = b
	}

	var chapters []chapterView
	for _, ch := range plan.Chapters {
		var nodes []nodeView
		for _, n := range ch.Nodes {
			if u, ok := unitByID[n.Unit]; ok {
				nodes = append(nodes, nodeViewOf(u, blockByID))
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
				nodes = append(nodes, nodeViewOf(u, blockByID))
			}
		}
		name := fmt.Sprintf("L%d", grp.Layer)
		if li, ok := layers[grp.Layer]; ok {
			name = li.name
		}
		orphans = append(orphans, chapterView{
			Title:  "Orphans · " + name,
			Summary: template.HTML("Changed here but not called by anything else in this diff."),
			Nodes:  nodes, Orphan: true})
	}

	prTag := ""
	if plan.PRNumber != 0 {
		prTag = fmt.Sprintf(" · #%d", plan.PRNumber)
	}
	covText := fmt.Sprintf("%d/%d blocks placed", plan.Coverage.Counts.Placed, plan.Coverage.Counts.Indexed)
	covClass := "cov-bad"
	if plan.Coverage.OK {
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
	}

	var b strings.Builder
	if err := pageTmpl.Execute(&b, pv); err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
}
