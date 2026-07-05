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
.diff.ws-hide .l.wsn{display:none}
/* --- reading affordances (issue #18): sticky TOC, mark-read, keyboard nav --- */
#toc{position:fixed;top:0;left:0;bottom:0;width:250px;overflow-y:auto;background:var(--card);border-right:1px solid var(--line);padding:18px 14px;z-index:30;transform:translateX(-100%);transition:transform .16s ease}
body.toc-open #toc{transform:none;box-shadow:2px 0 16px rgba(15,23,42,.14)}
#toc .toch{display:flex;align-items:baseline;justify-content:space-between;margin:0 0 12px}
#toc .toch b{font-size:11px;text-transform:uppercase;letter-spacing:.06em;color:var(--muted)}
#toc .tocprog{font-size:12px;color:var(--muted)}
#toc ol{list-style:none;margin:0;padding:0}
#toc li{margin:0 0 3px}
#toc a{display:block;padding:6px 8px;border-radius:6px;color:var(--fg);text-decoration:none}
#toc a:hover{background:var(--bg)}
#toc a.active{background:#eef2ff;color:#3730a3}
#toc .ctitle{display:block;font-size:13px;line-height:1.3}
#toc .cbar{display:flex;align-items:center;gap:6px;margin-top:5px}
#toc .cbar i{flex:1;height:4px;border-radius:2px;background:var(--line);overflow:hidden}
#toc .cbar i>span{display:block;height:100%;width:0;background:#22c55e;transition:width .2s}
#toc .cbar b{font-weight:400;font-size:11px;color:var(--muted);min-width:2.6em;text-align:right}
.node .mark{margin-left:6px;flex:none;font:12px/1 -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;padding:4px 9px;border:1px solid var(--line);border-radius:99px;background:var(--bg);color:var(--muted);cursor:pointer;white-space:nowrap}
.node .mark:hover{border-color:#94a3b8;color:var(--fg)}
.node.read .mark{background:#dcfce7;border-color:#86efac;color:#166534}
.node.read>summary .sym,.node.read>summary .one{opacity:.55}
.node.current{outline:2px solid #6366f1;outline-offset:1px}
.kbdhint{font-size:12px;color:var(--muted);margin-top:10px}
.kbdhint kbd{font:11px ui-monospace,SFMono-Regular,Menlo,monospace;background:var(--bg);border:1px solid var(--line);border-bottom-width:2px;border-radius:4px;padding:1px 5px;color:var(--fg)}
@media(min-width:1180px){
  #toc{transform:none;box-shadow:none}
  body.has-toc{padding-left:250px}
  #tocbtn{display:none}
}`

var pageTmpl = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}} — narrative review</title>
<style>{{.CSS}}</style></head>
<body data-ns="{{.Namespace}}">
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
<script>
/* Reading affordances (issue #18): sticky chapter TOC with per-chapter progress,
   keyboard navigation, and per-node mark-read persisted in localStorage keyed by
   repo#pr + the node's stable block-sha hash (data-readkey). Self-contained. */
(function(){
  var nodes=Array.prototype.slice.call(document.querySelectorAll('details.node'));
  if(!nodes.length) return;
  var body=document.body;
  var ns=body.getAttribute('data-ns')||location.pathname;
  var PFX='ncr:read:'+ns+':';
  function keyOf(n){return n.getAttribute('data-readkey')||n.id;}
  function isRead(n){try{return localStorage.getItem(PFX+keyOf(n))==='1';}catch(e){return false;}}
  function store(n,v){try{if(v)localStorage.setItem(PFX+keyOf(n),'1');else localStorage.removeItem(PFX+keyOf(n));}catch(e){}}
  function applyRead(n){
    var r=isRead(n);
    n.classList.toggle('read',r);
    var b=n.querySelector(':scope > summary > .mark');
    if(b){b.textContent=r?'✓ Read':'Mark read';b.setAttribute('aria-pressed',r?'true':'false');}
  }
  function setRead(n,v){store(n,v);applyRead(n);updateProgress();}

  // per-node "mark read" button, injected into each summary
  nodes.forEach(function(n){
    var s=n.querySelector(':scope > summary');if(!s)return;
    var btn=document.createElement('button');
    btn.type='button';btn.className='mark';btn.title='Toggle read (m)';
    btn.addEventListener('click',function(e){e.preventDefault();e.stopPropagation();setRead(n,!isRead(n));});
    s.appendChild(btn);
    applyRead(n);
  });

  // sticky chapter TOC with per-chapter progress
  var tocItems=[],tocProg=null,haveTOC=false;
  var sections=Array.prototype.slice.call(document.querySelectorAll('main > section.chapter'));
  if(sections.length){
    var toc=document.createElement('nav');toc.id='toc';
    var head=document.createElement('div');head.className='toch';
    var lbl=document.createElement('b');lbl.textContent='Chapters';
    tocProg=document.createElement('span');tocProg.className='tocprog';
    head.appendChild(lbl);head.appendChild(tocProg);toc.appendChild(head);
    var ol=document.createElement('ol');
    sections.forEach(function(sec,i){
      if(!sec.id)sec.id='chapter-'+i;
      var h2=sec.querySelector('h2');
      var title=h2?h2.textContent.trim():('Chapter '+(i+1));
      var secNodes=Array.prototype.slice.call(sec.querySelectorAll('details.node'));
      var a=document.createElement('a');a.href='#'+sec.id;
      var t=document.createElement('span');t.className='ctitle';t.textContent=title;
      var bar=document.createElement('span');bar.className='cbar';
      var track=document.createElement('i');var fill=document.createElement('span');track.appendChild(fill);
      var num=document.createElement('b');
      bar.appendChild(track);bar.appendChild(num);
      a.appendChild(t);a.appendChild(bar);
      a.addEventListener('click',function(){body.classList.remove('toc-open');});
      var li=document.createElement('li');li.appendChild(a);ol.appendChild(li);
      tocItems.push({sec:sec,nodes:secNodes,a:a,fill:fill,num:num});
    });
    toc.appendChild(ol);
    body.appendChild(toc);
    body.classList.add('has-toc');
    haveTOC=true;
  }

  function updateProgress(){
    var total=0,read=0;
    tocItems.forEach(function(it){
      var t=it.nodes.length,r=0;
      it.nodes.forEach(function(n){if(isRead(n))r++;});
      total+=t;read+=r;
      it.fill.style.width=t?(100*r/t)+'%':'0';
      it.num.textContent=r+'/'+t;
    });
    if(tocProg)tocProg.textContent=read+'/'+total+' read';
  }

  // extra controls: contents toggle + next-unread jump + keyboard hint
  function nextUnread(){
    var start=cur+1;
    for(var k=0;k<nodes.length;k++){
      var idx=((start+k)%nodes.length+nodes.length)%nodes.length;
      if(!isRead(nodes[idx])){setCurrent(idx,true);return;}
    }
  }
  var controls=document.querySelector('.controls');
  if(controls){
    if(haveTOC){
      var cbtn=document.createElement('button');cbtn.id='tocbtn';cbtn.type='button';cbtn.textContent='☰ Contents';
      cbtn.addEventListener('click',function(){body.classList.toggle('toc-open');});
      controls.appendChild(cbtn);
    }
    var ubtn=document.createElement('button');ubtn.type='button';ubtn.textContent='Next unread';
    ubtn.addEventListener('click',function(){nextUnread();});
    controls.appendChild(ubtn);
    var hint=document.createElement('div');hint.className='kbdhint';
    hint.innerHTML='<kbd>j</kbd>/<kbd>k</kbd> move · <kbd>o</kbd> toggle · <kbd>m</kbd> mark read · <kbd>u</kbd> next unread';
    controls.parentNode.appendChild(hint);
  }

  // keyboard navigation
  var cur=-1;
  function setCurrent(i,scroll){
    if(i<0||i>=nodes.length)return;
    if(cur>=0&&nodes[cur])nodes[cur].classList.remove('current');
    cur=i;nodes[cur].classList.add('current');
    if(scroll&&nodes[cur].scrollIntoView)nodes[cur].scrollIntoView({block:'center',behavior:'smooth'});
  }
  nodes.forEach(function(n,i){
    var s=n.querySelector(':scope > summary');
    if(s)s.addEventListener('click',function(){if(cur>=0&&nodes[cur])nodes[cur].classList.remove('current');cur=i;n.classList.add('current');});
  });
  document.addEventListener('keydown',function(e){
    if(e.ctrlKey||e.metaKey||e.altKey)return;
    var t=e.target;
    if(t&&(t.tagName==='INPUT'||t.tagName==='TEXTAREA'||t.isContentEditable))return;
    if(e.key==='Enter'&&t&&(t.tagName==='BUTTON'||t.tagName==='A'||t.tagName==='SUMMARY'))return;
    switch(e.key){
      case 'j':case 'n':e.preventDefault();setCurrent(cur<0?0:Math.min(cur+1,nodes.length-1),true);break;
      case 'k':case 'p':e.preventDefault();setCurrent(cur<0?0:Math.max(cur-1,0),true);break;
      case 'o':case 'Enter':if(cur>=0){e.preventDefault();nodes[cur].open=!nodes[cur].open;}break;
      case 'm':if(cur>=0){e.preventDefault();setRead(nodes[cur],!isRead(nodes[cur]));}break;
      case 'u':e.preventDefault();nextUnread();break;
    }
  });

  // highlight the chapter currently in view
  if(tocItems.length&&'IntersectionObserver' in window){
    var io=new IntersectionObserver(function(entries){
      entries.forEach(function(en){
        for(var i=0;i<tocItems.length;i++){if(tocItems[i].sec===en.target){tocItems[i].vis=en.isIntersecting;break;}}
      });
      var active=null;
      for(var i=0;i<tocItems.length;i++){if(tocItems[i].vis){active=tocItems[i];break;}}
      tocItems.forEach(function(x){x.a.classList.toggle('active',x===active);});
    },{rootMargin:'-8% 0px -80% 0px'});
    tocItems.forEach(function(it){io.observe(it.sec);});
  }

  // keep read-state in sync across tabs/windows
  window.addEventListener('storage',function(e){
    if(e.key&&e.key.indexOf(PFX)===0){nodes.forEach(applyRead);updateProgress();}
  });

  updateProgress();
})();
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
<details id="{{.ID}}" class="node" data-readkey="{{.ReadKey}}">
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
