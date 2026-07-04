package main

import "html/template"

// pageCSS is copied verbatim from ncr/render.py _CSS to guarantee visual parity.
// chromaCSS() (monokai token colors) is appended at render time.
const pageCSS = `:root{--fg:#1e293b;--muted:#64748b;--bg:#f8fafc;--card:#fff;--line:#e2e8f0}
*{box-sizing:border-box}
body{margin:0;font:15px/1.5 -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;color:var(--fg);background:var(--bg)}
header{padding:24px 32px;background:var(--card);border-bottom:1px solid var(--line)}
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
.sym{font-size:13px;background:var(--bg);padding:1px 6px;border-radius:4px}
.one{color:#334155;flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.body{padding:12px 14px}
.sumfull{color:var(--fg);margin-bottom:8px}
.sumfull code,.detail code,.chsum code,.overview code{font:12.5px/1.4 ui-monospace,SFMono-Regular,Menlo,monospace;background:#eef2f7;color:#0f172a;padding:1px 5px;border-radius:4px}
.sumfull p,.detail p,.overview p{margin:0 0 8px}
.meta{font-size:12px;color:var(--muted);margin-bottom:10px}
.detail{color:#475569;margin-bottom:8px}
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
.diff .ctx{opacity:.62}
.diff .sep{color:#75715e;user-select:none}
.diffwrap{position:relative}
.wstoggle{position:absolute;top:6px;right:10px;z-index:1;font:11px ui-monospace,monospace;padding:2px 8px;border-radius:6px;border:1px solid #3a3f36;background:#1e2019;color:#c9d1d9;cursor:pointer;opacity:.55}
.wstoggle:hover{opacity:1}
.diff.ws-hide .l.wsn{display:none}`

var pageTmpl = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}} — narrative review</title>
<style>{{.CSS}}</style></head>
<body>
<header>
  <div class="titlebar">
    <h1>{{.Title}}<span class="prtag">{{.PRTag}}</span></h1>
    <span class="cov {{.CovClass}}">{{.CovText}}</span>
  </div>
  <p class="overview">{{.Overview}}</p>
  <div class="controls">
    <button id="xall">Expand all</button>
    <button id="call">Collapse all</button>
  </div>
</header>
<main>
{{range .Chapters}}{{template "chapter" .}}{{end}}
{{range .Orphans}}{{template "chapter" .}}{{end}}
</main>
<script>
document.getElementById('xall').onclick=function(){document.querySelectorAll('details').forEach(function(d){d.open=true})};
document.getElementById('call').onclick=function(){document.querySelectorAll('details').forEach(function(d){d.open=false})};
document.querySelectorAll('.wstoggle').forEach(function(btn){btn.onclick=function(){
  var pre=btn.parentElement.querySelector('pre.diff');
  var hidden=pre.classList.toggle('ws-hide');
  btn.textContent=hidden?'show whitespace':'hide whitespace';
}});
</script>
{{if .Interactive}}<link rel="stylesheet" href="/review.css"><script src="/review.js" defer></script>{{end}}
</body></html>

{{define "chapter"}}
<section class="chapter{{if .Orphan}} orphan{{end}}">
  <h2>{{.Title}}</h2>
  <p class="chsum">{{.Summary}}</p>
  {{range .Nodes}}{{template "node" .}}{{end}}
</section>{{end}}

{{define "node"}}
<details id="{{.ID}}" class="node">
  <summary>
    {{.Badge}}
    <code class="sym">{{.Sym}}</code>
    <span class="one">{{.Teaser}}</span>
  </summary>
  <div class="body">
    <div class="sumfull">{{.Summary}}</div>
    {{.Detail}}
    <div class="meta">{{.Meta}}</div>
    {{.Calls}}
    {{.Diff}}
  </div>
</details>{{end}}`))
