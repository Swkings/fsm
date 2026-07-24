package visualization

const webPage = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>FSM Live Observatory</title>
<style>
:root {
  color-scheme: light;
  --canvas: #faf9f5;
  --surface: #ffffff;
  --surface-soft: #efe9de;
  --surface-dark: #181715;
  --ink: #141413;
  --muted: #6f6a63;
  --hairline: #e6dfd8;
  --primary: #cc785c;
  --primary-active: #a9583e;
  --success: #3f7d62;
  --danger: #b85045;
  --warning: #a06b28;
  --graph-bg: #181715;
  --graph-node: #292724;
  --graph-line: #746e66;
  --graph-text: #faf9f5;
  --code: #3b3834;
  --sidebar-width: 300px;
  --detail-left: 680px;
}
:root[data-theme="dark"] {
  color-scheme: dark;
  --canvas: #181715;
  --surface: #22211f;
  --surface-soft: #2d2b28;
  --ink: #f4f0e8;
  --muted: #aaa39a;
  --hairline: #3d3934;
  --primary: #dc896c;
  --primary-active: #ef9b7c;
  --success: #73ad8d;
  --danger: #df7c6e;
  --warning: #d5a45c;
  --graph-bg: #11110f;
  --graph-node: #262421;
  --graph-line: #777066;
  --graph-text: #faf9f5;
  --code: #e8dfd2;
}
* { box-sizing: border-box; }
body {
  margin: 0;
  min-height: 100vh;
  background: var(--canvas);
  color: var(--ink);
  font: 14px/1.55 Inter, ui-sans-serif, system-ui, -apple-system, "Segoe UI", sans-serif;
}
button, input { font: inherit; }
.topbar {
  height: 72px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
  padding: 0 28px;
  border-bottom: 1px solid var(--hairline);
  background: color-mix(in srgb, var(--canvas) 94%, transparent);
  position: sticky;
  top: 0;
  z-index: 10;
  backdrop-filter: blur(12px);
}
.brand { display: flex; align-items: center; gap: 13px; min-width: 0; }
.brand-mark {
  width: 38px;
  height: 38px;
  display: grid;
  place-items: center;
  color: var(--primary);
  border: 1px solid color-mix(in srgb, var(--primary) 55%, var(--hairline));
  border-radius: 50%;
  font: 22px/1 Georgia, serif;
}
.brand h1 {
  margin: 0;
  font: 600 19px/1.1 Georgia, "Times New Roman", serif;
  letter-spacing: -.015em;
}
.brand small { display: block; margin-top: 4px; color: var(--muted); font-size: 11px; letter-spacing: .04em; }
.header-actions { display: flex; align-items: center; gap: 12px; }
.connection { display: flex; align-items: center; gap: 8px; color: var(--muted); font-size: 12px; }
.dot { width: 7px; height: 7px; border-radius: 50%; background: var(--warning); }
.dot.online { background: var(--success); }
.dot.offline { background: var(--danger); }
.icon-button {
  width: 36px;
  height: 36px;
  display: grid;
  place-items: center;
  border: 1px solid var(--hairline);
  border-radius: 50%;
  background: var(--surface);
  color: var(--ink);
  cursor: pointer;
}
.icon-button:hover { color: var(--primary); border-color: var(--primary); }
.icon-button:focus-visible, .resize-handle:focus-visible, .fsm-button:focus-visible, summary:focus-visible {
  outline: 2px solid var(--primary);
  outline-offset: 2px;
}
.icon-button:disabled { cursor: not-allowed; opacity: .42; }
.icon-button svg { width: 17px; height: 17px; fill: none; stroke: currentColor; stroke-width: 1.8; stroke-linecap: round; stroke-linejoin: round; }
.theme-moon { display: none; }
:root[data-theme="dark"] .theme-sun { display: none; }
:root[data-theme="dark"] .theme-moon { display: block; }
.layout { display: grid; grid-template-columns: var(--sidebar-width) 8px minmax(0, 1fr); min-height: calc(100vh - 72px); }
aside {
  min-width: 0;
  padding: 22px 15px;
  border-right: 1px solid var(--hairline);
  background: var(--surface-soft);
  overflow: auto;
}
.aside-heading { display: flex; align-items: baseline; justify-content: space-between; padding: 0 9px 12px; }
.aside-title { font: 600 13px Georgia, "Times New Roman", serif; }
.count-pill { color: var(--muted); font-size: 11px; }
.tree-toolbar { padding: 0 7px 15px; }
.tree-search {
  width: 100%;
  height: 36px;
  padding: 0 12px;
  color: var(--ink);
  border: 1px solid var(--hairline);
  border-radius: 8px;
  background: var(--surface);
  outline: none;
}
.tree-search:focus { border-color: var(--primary); }
.fsm-list { display: grid; gap: 3px; }
.folder-node { margin: 2px 0; }
.folder-node > summary {
  display: flex;
  align-items: center;
  gap: 7px;
  min-height: 34px;
  padding: 5px 8px;
  border-radius: 7px;
  color: var(--ink);
  cursor: pointer;
  list-style: none;
  font-weight: 600;
  font-size: 12px;
}
.folder-node > summary::-webkit-details-marker { display: none; }
.folder-node > summary:hover { background: color-mix(in srgb, var(--surface) 55%, transparent); }
.folder-chevron { width: 13px; color: var(--muted); transition: transform .15s; }
.folder-node[open] > summary .folder-chevron { transform: rotate(90deg); }
.folder-icon { color: var(--primary); }
.folder-count { margin-left: auto; color: var(--muted); font-size: 10px; font-weight: 500; }
.folder-content { margin-left: 13px; padding-left: 7px; border-left: 1px solid var(--hairline); }
.fsm-button {
  width: 100%;
  min-width: 0;
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 2px 8px;
  padding: 9px 10px;
  color: var(--ink);
  text-align: left;
  border: 1px solid transparent;
  border-radius: 8px;
  background: transparent;
  cursor: pointer;
}
.fsm-button:hover { background: color-mix(in srgb, var(--surface) 65%, transparent); }
.fsm-button.active { background: var(--surface); border-color: color-mix(in srgb, var(--primary) 42%, var(--hairline)); }
.fsm-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-weight: 600; font-size: 12px; }
.fsm-meta { grid-column: 1 / -1; overflow: hidden; color: var(--muted); text-overflow: ellipsis; white-space: nowrap; font-size: 10px; }
.state-mini { max-width: 82px; overflow: hidden; color: var(--success); text-overflow: ellipsis; white-space: nowrap; font-size: 10px; }
.mode-dot { display: inline-block; width: 5px; height: 5px; margin-right: 4px; border-radius: 50%; background: var(--primary); vertical-align: 1px; }
.mode-dot.pull { background: var(--warning); }
main { padding: 30px clamp(18px, 3vw, 42px) 44px; overflow: hidden; }
.empty { min-height: 64vh; display: grid; place-items: center; color: var(--muted); text-align: center; }
.empty strong { display: block; margin-bottom: 7px; color: var(--ink); font: 600 25px Georgia, "Times New Roman", serif; }
.content { display: none; }
.content.visible { display: block; }
.eyebrow { margin-bottom: 8px; color: var(--primary); font-size: 10px; font-weight: 700; letter-spacing: .14em; text-transform: uppercase; }
.hero { display: flex; align-items: flex-start; justify-content: space-between; gap: 24px; margin-bottom: 22px; }
.hero h2 { margin: 0; font: 600 clamp(26px, 3vw, 40px)/1.08 Georgia, "Times New Roman", serif; letter-spacing: -.025em; }
.hero p { max-width: 720px; margin: 8px 0 0; color: var(--muted); }
.hero-badges { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; justify-content: flex-end; }
.state-badge, .mode-badge {
  padding: 7px 11px;
  border: 1px solid color-mix(in srgb, var(--success) 45%, var(--hairline));
  border-radius: 999px;
  color: var(--success);
  background: color-mix(in srgb, var(--success) 8%, var(--surface));
  font-size: 11px;
  font-weight: 700;
}
.mode-badge { border-color: var(--hairline); color: var(--primary); background: var(--surface); text-transform: uppercase; }
.collection-error {
  display: none;
  margin: -8px 0 18px;
  padding: 10px 12px;
  border-left: 3px solid var(--danger);
  color: var(--danger);
  background: color-mix(in srgb, var(--danger) 7%, var(--surface));
  font-size: 12px;
}
.grid { display: grid; grid-template-columns: minmax(360px, var(--detail-left)) 16px minmax(300px, 1fr); margin-bottom: 16px; }
.card { overflow: hidden; border: 1px solid var(--hairline); border-radius: 12px; background: var(--surface); }
.card-head { min-height: 50px; display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 0 17px; border-bottom: 1px solid var(--hairline); }
.card-head h3 { margin: 0; font: 600 14px Georgia, "Times New Roman", serif; }
.card-head span { color: var(--muted); font-size: 10px; }
.card-body { padding: 16px; }
.card-actions { display: flex; align-items: center; gap: 9px; }
.card-actions .icon-button { width: 30px; height: 30px; }
.graph-card { background: var(--surface-dark); color: #faf9f5; border-color: #2e2c28; }
.graph-card .card-head { border-color: #34312c; }
.graph-card .card-head span, .graph-card .available-head span, .graph-card .available-empty { color: #a8a097; }
.state-graph { min-height: 430px; overflow: auto; border: 1px solid #34312c; border-radius: 9px; background: var(--graph-bg); }
.state-graph svg { display: block; width: 100%; min-width: 560px; height: auto; }
.graph-edge { fill: none; stroke: var(--graph-line); stroke-width: 2; opacity: .7; transition: stroke .2s, opacity .2s; }
.graph-edge.available { stroke: var(--primary); opacity: .95; }
.graph-edge.recent { stroke: var(--success); stroke-width: 3; opacity: 1; }
.graph-edge.processing { stroke: var(--warning); stroke-width: 3; opacity: 1; stroke-dasharray: 10 7; animation: edge-flow .8s linear infinite; }
.graph-edge.failed { stroke: var(--danger); stroke-width: 3; opacity: 1; }
.graph-edge-label { fill: #a8a097; font: 600 12px Inter, sans-serif; paint-order: stroke; stroke: var(--graph-bg); stroke-width: 5px; stroke-linejoin: round; }
.graph-edge-label.available { fill: #edab92; }
.graph-edge-label.recent { fill: #84bd9e; }
.graph-edge-label.processing { fill: #e0b56f; }
.graph-edge-label.failed { fill: #ec8d82; }
.graph-node rect { fill: var(--graph-node); stroke: var(--graph-line); stroke-width: 2; }
.graph-node text { fill: var(--graph-text); font: 650 13px Inter, sans-serif; text-anchor: middle; dominant-baseline: middle; }
.graph-node .node-status { fill: #84bd9e; font-size: 9px; letter-spacing: .12em; }
.graph-node.current rect { fill: #25392e; stroke: #73ad8d; stroke-width: 3; }
.graph-node.previous rect { stroke: var(--primary); }
@keyframes edge-flow { to { stroke-dashoffset: -17; } }
.available-head { display: flex; align-items: center; justify-content: space-between; margin: 16px 2px 8px; }
.available-head strong { font-size: 11px; letter-spacing: .05em; }
.available-list { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 8px; }
.available-transition { display: flex; align-items: center; gap: 9px; padding: 9px 10px; border: 1px solid #45413b; border-radius: 7px; background: #211f1c; }
.available-transition.processing { border-color: var(--warning); }
.event-token { color: #edab92; font: 650 11px ui-monospace, monospace; }
.available-transition.processing .event-token { color: #e0b56f; }
.target-state { margin-left: auto; color: #faf9f5; font-size: 11px; }
.available-empty { padding: 10px 2px; color: var(--muted); }
pre { max-height: 360px; margin: 0; overflow: auto; color: var(--code); white-space: pre-wrap; word-break: break-word; font: 12px/1.7 ui-monospace, SFMono-Regular, Menlo, monospace; }
.payload-error { display: none; margin-bottom: 10px; color: var(--warning); font-size: 12px; }
.meta { display: grid; grid-template-columns: 1fr 1fr; gap: 9px; margin-top: 16px; }
.meta div { padding: 10px; border: 1px solid var(--hairline); border-radius: 7px; background: var(--canvas); }
.meta label { display: block; color: var(--muted); font-size: 9px; letter-spacing: .08em; text-transform: uppercase; }
.meta span { display: block; margin-top: 3px; overflow-wrap: anywhere; font: 10px ui-monospace, monospace; }
.table-wrap { max-height: 400px; overflow: auto; }
table { width: 100%; min-width: 780px; border-collapse: collapse; }
th, td { padding: 10px 12px; border-bottom: 1px solid var(--hairline); text-align: left; font-size: 11px; }
th { position: sticky; top: 0; background: var(--surface-soft); color: var(--muted); font-weight: 600; }
.ok { color: var(--success); }
.fail, .trace-error { color: var(--danger); }
.trace-error { max-width: 280px; white-space: normal; }
.resize-handle { position: relative; z-index: 3; cursor: col-resize; touch-action: none; }
.resize-handle::before { content: ""; position: absolute; top: 0; bottom: 0; left: 50%; width: 1px; background: var(--hairline); transform: translateX(-50%); }
.resize-handle:hover::before, .resize-handle.active::before { width: 3px; background: var(--primary); }
body.resizing { cursor: col-resize; user-select: none; }
.grid > .resize-handle { min-height: 100%; }
@media (max-width: 1100px) {
  .layout { grid-template-columns: 1fr; }
  .resize-handle { display: none; }
  aside { max-height: 340px; border-right: 0; border-bottom: 1px solid var(--hairline); }
  .grid { grid-template-columns: 1fr; gap: 16px; }
  main { padding: 22px 16px 34px; }
}
@media (max-width: 620px) {
  .topbar { height: auto; min-height: 68px; padding: 12px 16px; }
  .brand small { display: none; }
  .connection span:last-child { display: none; }
  .hero { display: grid; }
  .hero-badges { justify-content: flex-start; }
  .meta { grid-template-columns: 1fr; }
}
</style>
</head>
<body>
<header class="topbar">
  <div class="brand">
    <div class="brand-mark" aria-hidden="true">✦</div>
    <div><h1>FSM Observatory</h1><small>state, data and trace workspace</small></div>
  </div>
  <div class="header-actions">
    <div class="connection"><span id="connectionDot" class="dot"></span><span id="connectionText">正在连接</span></div>
    <button id="themeToggle" class="icon-button" type="button" aria-label="切换主题" title="切换主题">
      <svg class="theme-sun" viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="4"></circle><path d="M12 2v2M12 20v2M4.93 4.93l1.42 1.42M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.42-1.42M17.66 6.34l1.41-1.41"></path></svg>
      <svg class="theme-moon" viewBox="0 0 24 24" aria-hidden="true"><path d="M20.5 14.2A8.5 8.5 0 0 1 9.8 3.5a8.5 8.5 0 1 0 10.7 10.7z"></path></svg>
    </button>
  </div>
</header>
<div class="layout">
  <aside>
    <div class="aside-heading"><span class="aside-title">Machines</span><span class="count-pill"><span id="fsmCount">0</span> registered</span></div>
    <div class="tree-toolbar"><input id="fsmSearch" class="tree-search" type="search" placeholder="搜索 FSM、状态或路径" aria-label="搜索 FSM"></div>
    <div id="fsmList" class="fsm-list"></div>
  </aside>
  <div id="sidebarResize" class="resize-handle" role="separator" aria-label="调整状态机列表宽度" aria-orientation="vertical" tabindex="0"></div>
  <main>
    <div id="empty" class="empty"><div><strong>等待 FSM 注册</strong><span>通过 pull 或 push 接口注册后，实例会按 path 自动归档。</span></div></div>
    <div id="content" class="content">
      <div class="eyebrow" id="fsmPath">unfiled</div>
      <div class="hero">
        <div><h2 id="displayName"></h2><p id="description"></p></div>
        <div class="hero-badges"><span id="modeBadge" class="mode-badge"></span><span id="currentState" class="state-badge"></span></div>
      </div>
      <div id="collectionError" class="collection-error"></div>
      <div id="detailGrid" class="grid">
        <section id="graphCard" class="card graph-card">
          <div class="card-head"><h3>实时状态图</h3><div class="card-actions"><span id="transitionCount"></span><button id="exportGraph" class="icon-button" type="button" aria-label="导出状态图 PNG" title="导出状态图 PNG"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 3v12M7 10l5 5 5-5M4 19h16"></path></svg></button></div></div>
          <div class="card-body"><div id="stateGraph" class="state-graph"></div><div class="available-head"><strong>当前状态可接收事件</strong><span id="availableCount"></span></div><div id="availableTransitions" class="available-list"></div></div>
        </section>
        <div id="detailResize" class="resize-handle" role="separator" aria-label="调整状态图和 Payload 宽度" aria-orientation="vertical" tabindex="0"></div>
        <section id="payloadCard" class="card">
          <div class="card-head"><h3>Payload 数据</h3><span>JSON snapshot</span></div>
          <div class="card-body"><div id="payloadError" class="payload-error"></div><pre id="payload"></pre><div class="meta"><div><label>Transition ID</label><span id="transitionId"></span></div><div><label>Updated</label><span id="updatedAt"></span></div><div><label>Previous State</label><span id="previousState"></span></div><div><label>Previous Event</label><span id="previousEvent"></span></div></div></div>
        </section>
      </div>
      <section class="card"><div class="card-head"><h3>实时 Trace</h3><span id="traceCount"></span></div><div class="table-wrap"><table><thead><tr><th>时间</th><th>事件</th><th>状态流转</th><th>结果</th><th>耗时</th><th>错误</th></tr></thead><tbody id="traceBody"></tbody></table></div></section>
    </div>
  </main>
</div>
<script>
const app={fsms:[],selected:"",query:""};
const byId=id=>document.getElementById(id);
function readPreference(key){try{return localStorage.getItem(key)||""}catch(error){return ""}}
function writePreference(key,value){try{if(value==="")localStorage.removeItem(key);else localStorage.setItem(key,value)}catch(error){}}
function applyTheme(theme,persist){const selected=theme==="light"?"light":"dark";document.documentElement.dataset.theme=selected;const button=byId("themeToggle");if(button){const target=selected==="dark"?"浅色":"深色";button.setAttribute("aria-label","切换为"+target+"主题");button.title="切换为"+target+"主题"}if(persist)writePreference("fsm-visualization-theme",selected);if(app.fsms.length)render()}
function initializeTheme(){const stored=readPreference("fsm-visualization-theme");const preferred=stored||(window.matchMedia&&window.matchMedia("(prefers-color-scheme: light)").matches?"light":"dark");applyTheme(preferred,false);byId("themeToggle").onclick=()=>applyTheme(document.documentElement.dataset.theme==="light"?"dark":"light",true)}
function setupColumnResizer(options){const handle=byId(options.handle);const container=document.querySelector(options.container);const first=document.querySelector(options.first);const root=document.documentElement;const stored=Number(readPreference(options.storage));if(Number.isFinite(stored)&&stored>0)root.style.setProperty(options.variable,stored+"px");function setSize(value,persist){const bounds=container.getBoundingClientRect();const maximum=Math.max(options.minimum,bounds.width-handle.offsetWidth-options.trailingMinimum);const size=Math.round(Math.min(maximum,Math.max(options.minimum,value)));root.style.setProperty(options.variable,size+"px");handle.setAttribute("aria-valuemin",String(options.minimum));handle.setAttribute("aria-valuemax",String(Math.round(maximum)));handle.setAttribute("aria-valuenow",String(size));if(persist)writePreference(options.storage,String(size))}handle.addEventListener("pointerdown",event=>{if(window.matchMedia("(max-width: 1100px)").matches)return;event.preventDefault();handle.classList.add("active");document.body.classList.add("resizing");const move=moveEvent=>{const bounds=container.getBoundingClientRect();setSize(moveEvent.clientX-bounds.left,false)};const stop=stopEvent=>{if(Number.isFinite(stopEvent.clientX))move(stopEvent);handle.classList.remove("active");document.body.classList.remove("resizing");window.removeEventListener("pointermove",move);window.removeEventListener("pointerup",stop);window.removeEventListener("pointercancel",stop);writePreference(options.storage,String(Math.round(first.getBoundingClientRect().width)))};window.addEventListener("pointermove",move);window.addEventListener("pointerup",stop);window.addEventListener("pointercancel",stop)});handle.addEventListener("keydown",event=>{if(event.key!=="ArrowLeft"&&event.key!=="ArrowRight")return;event.preventDefault();setSize(first.getBoundingClientRect().width+(event.key==="ArrowLeft"?-16:16),true)});handle.addEventListener("dblclick",()=>{root.style.removeProperty(options.variable);writePreference(options.storage,"");});}
function initializeResizableColumns(){setupColumnResizer({handle:"sidebarResize",container:".layout",first:".layout > aside",variable:"--sidebar-width",storage:"fsm-sidebar-width",minimum:190,trailingMinimum:744});setupColumnResizer({handle:"detailResize",container:"#detailGrid",first:"#graphCard",variable:"--detail-left",storage:"fsm-graph-width",minimum:360,trailingMinimum:320})}
function text(id,value){byId(id).textContent=value===undefined||value===null?"":String(value)}
function formatTime(value){if(!value)return "-";return new Date(value).toLocaleString()}
function formatNanos(value){if(!value)return "0 ms";if(value<1000)return value+" ns";if(value<1000000)return (value/1000).toFixed(2)+" μs";return (value/1000000).toFixed(2)+" ms"}
async function loadFSMs(){try{const response=await fetch("/api/fsms",{cache:"no-store"});if(!response.ok)throw new Error(response.statusText);app.fsms=await response.json();if(!app.fsms.some(item=>item.id===app.selected))app.selected=app.fsms.length?app.fsms[0].id:"";render()}catch(error){console.error(error)}}
function render(){renderList();const current=app.fsms.find(item=>item.id===app.selected);byId("empty").style.display=current?"none":"grid";byId("content").classList.toggle("visible",Boolean(current));byId("exportGraph").disabled=!current;if(!current)return;renderDetail(current)}
function buildFolderTree(items){
  const root={name:"",path:"",folders:new Map(),items:[]};
  for(const item of items){
    let node=root;
    const segments=String(item.path||"").split("/").map(value=>value.trim()).filter(Boolean);
    for(const segment of segments){
      const childPath=node.path?node.path+"/"+segment:segment;
      if(!node.folders.has(segment))node.folders.set(segment,{name:segment,path:childPath,folders:new Map(),items:[]});
      node=node.folders.get(segment);
    }
    node.items.push(item);
  }
  return root;
}
function folderSize(node){
  let total=node.items.length;
  for(const child of node.folders.values())total+=folderSize(child);
  return total;
}
function renderMachineButton(item){
  const button=document.createElement("button");
  button.className="fsm-button"+(item.id===app.selected?" active":"");
  button.type="button";
  const name=document.createElement("span");
  name.className="fsm-name";
  name.textContent=item.displayName||item.name||item.id;
  const state=document.createElement("span");
  state.className="state-mini";
  state.textContent=item.currentState||"—";
  const meta=document.createElement("span");
  meta.className="fsm-meta";
  const mode=document.createElement("i");
  mode.className="mode-dot "+(item.mode||"push");
  meta.appendChild(mode);
  meta.appendChild(document.createTextNode((item.mode||"push")+" · "+(item.name||item.id)));
  button.append(name,state,meta);
  button.onclick=()=>{app.selected=item.id;render()};
  return button;
}
function renderFolder(parent,node){
  const details=document.createElement("details");
  details.className="folder-node";
  details.dataset.path=node.path;
  const preference=readPreference("fsm-folder:"+node.path);
  details.open=preference!=="closed";
  const summary=document.createElement("summary");
  const chevron=document.createElement("span");
  chevron.className="folder-chevron";
  chevron.textContent="›";
  const icon=document.createElement("span");
  icon.className="folder-icon";
  icon.textContent="□";
  const label=document.createElement("span");
  label.textContent=node.name;
  const count=document.createElement("span");
  count.className="folder-count";
  count.textContent=String(folderSize(node));
  summary.append(chevron,icon,label,count);
  const content=document.createElement("div");
  content.className="folder-content";
  for(const item of [...node.items].sort(compareMachines))content.appendChild(renderMachineButton(item));
  for(const child of [...node.folders.values()].sort((left,right)=>left.name.localeCompare(right.name,"zh-CN")))renderFolder(content,child);
  details.append(summary,content);
  details.addEventListener("toggle",()=>writePreference("fsm-folder:"+node.path,details.open?"open":"closed"));
  parent.appendChild(details);
}
function compareMachines(left,right){
  return String(left.displayName||left.name||left.id).localeCompare(String(right.displayName||right.name||right.id),"zh-CN");
}
function renderList(){
  const list=byId("fsmList");
  list.replaceChildren();
  text("fsmCount",app.fsms.length);
  const query=String(app.query||"").trim().toLocaleLowerCase();
  const visible=query?app.fsms.filter(item=>[item.displayName,item.name,item.id,item.path,item.currentState,item.mode].some(value=>String(value||"").toLocaleLowerCase().includes(query))):app.fsms;
  const tree=buildFolderTree(visible);
  for(const item of [...tree.items].sort(compareMachines))list.appendChild(renderMachineButton(item));
  for(const folder of [...tree.folders.values()].sort((left,right)=>left.name.localeCompare(right.name,"zh-CN")))renderFolder(list,folder);
  if(!visible.length){
    const empty=document.createElement("div");
    empty.className="available-empty";
    empty.textContent=app.fsms.length?"没有匹配的 FSM":"尚未注册 FSM";
    list.appendChild(empty);
  }
}
function renderDetail(item){
  const transitions=item.transitions||[];
  const traces=item.traces||[];
  text("fsmPath",item.path||"unfiled");
  text("displayName",item.displayName||item.name||item.id);
  text("description",item.describe||item.description||item.name+" · "+item.id);
  text("modeBadge",item.mode||"push");
  text("currentState",item.currentState||"empty");
  text("transitionCount",transitions.length+" transitions");
  text("traceCount",traces.length+" traces");
  text("transitionId",item.transitionId);
  text("updatedAt",formatTime(item.updatedAt));
  text("previousState",item.previousState||"-");
  text("previousEvent",item.previousEvent||"-");
  text("payloadError",item.payloadError);
  byId("payloadError").style.display=item.payloadError?"block":"none";
  text("collectionError",item.collectionError);
  byId("collectionError").style.display=item.collectionError?"block":"none";
  byId("payload").textContent=JSON.stringify(item.payload===undefined?null:item.payload,null,2);
  renderStateGraph(item);
  renderAvailableTransitions(item);
  renderTraces(traces);
}
function svgElement(name,attributes,content){const element=document.createElementNS("http://www.w3.org/2000/svg",name);for(const [key,value] of Object.entries(attributes||{}))element.setAttribute(key,String(value));if(content!==undefined)element.textContent=content;return element}
function addArrowMarker(defs,id,color){const marker=svgElement("marker",{id:id,viewBox:"0 0 10 10",refX:9,refY:5,markerWidth:7,markerHeight:7,orient:"auto-start-reverse"});marker.appendChild(svgElement("path",{d:"M 0 0 L 10 5 L 0 10 z",fill:color}));defs.appendChild(marker)}
function renderStateGraph(item){const host=byId("stateGraph");host.replaceChildren();const transitions=item.transitions||[];const stateValues=item.states&&item.states.length?item.states:[item.currentState,...transitions.flatMap(edge=>[edge.fromState,edge.toState])];const states=[...new Set(stateValues.map(String))].sort();if(!states.length){const empty=document.createElement("div");empty.className="available-empty";empty.textContent="尚未配置状态";host.appendChild(empty);return}const radius=Math.max(145,Math.min(330,states.length*34));const width=Math.max(760,radius*2+220);const height=Math.max(440,radius*2+150);const centerX=width/2;const centerY=height/2+10;const svg=svgElement("svg",{viewBox:"0 0 "+width+" "+height,role:"img","aria-label":"FSM 状态转移图"});const defs=svgElement("defs");const themeStyle=getComputedStyle(document.documentElement);const themeColor=name=>themeStyle.getPropertyValue(name).trim();addArrowMarker(defs,"arrow-default",themeColor("--graph-line"));addArrowMarker(defs,"arrow-available",themeColor("--primary"));addArrowMarker(defs,"arrow-recent",themeColor("--success"));addArrowMarker(defs,"arrow-processing",themeColor("--warning"));addArrowMarker(defs,"arrow-failed",themeColor("--danger"));svg.appendChild(defs);const positions=new Map();for(let index=0;index<states.length;index++){if(states.length===1){positions.set(states[index],{x:centerX,y:centerY})}else{const angle=-Math.PI/2+index*Math.PI*2/states.length;positions.set(states[index],{x:centerX+Math.cos(angle)*radius,y:centerY+Math.sin(angle)*Math.min(radius,height/2-90)})}}const groups=new Map();for(const edge of transitions){const from=String(edge.fromState);const to=String(edge.toState);const key=JSON.stringify([from,to]);if(!groups.has(key))groups.set(key,{from:from,to:to,events:[]});groups.get(key).events.push(String(edge.event))}const latest=item.traces&&item.traces.length?item.traces[item.traces.length-1]:null;const edgeLayer=svgElement("g");const labelLayer=svgElement("g");for(const group of groups.values()){const from=positions.get(group.from);const to=positions.get(group.to);if(!from||!to)continue;const processing=group.from===String(item.currentState)&&group.events.includes(String(item.processingEvent||""));const recent=latest&&group.from===String(latest.fromState)&&group.to===String(latest.toState)&&group.events.includes(String(latest.event));const available=group.from===String(item.currentState);const status=processing?"processing":recent?(latest.success?"recent":"failed"):available?"available":"";let pathData;let labelX;let labelY;if(group.from===group.to){pathData="M "+(from.x-25)+" "+(from.y-31)+" C "+(from.x-82)+" "+(from.y-112)+" "+(from.x+82)+" "+(from.y-112)+" "+(from.x+25)+" "+(from.y-31);labelX=from.x;labelY=from.y-100}else{const dx=to.x-from.x;const dy=to.y-from.y;const length=Math.max(1,Math.hypot(dx,dy));const startX=from.x+dx/length*58;const startY=from.y+dy/length*38;const endX=to.x-dx/length*62;const endY=to.y-dy/length*40;const reverse=groups.has(JSON.stringify([group.to,group.from]));const curve=reverse?34:0;const controlX=(startX+endX)/2-dy/length*curve;const controlY=(startY+endY)/2+dx/length*curve;pathData=curve?"M "+startX+" "+startY+" Q "+controlX+" "+controlY+" "+endX+" "+endY:"M "+startX+" "+startY+" L "+endX+" "+endY;labelX=curve?(startX+2*controlX+endX)/4:(startX+endX)/2-dy/length*12;labelY=curve?(startY+2*controlY+endY)/4:(startY+endY)/2+dx/length*12}const marker=status==="processing"?"arrow-processing":status==="recent"?"arrow-recent":status==="failed"?"arrow-failed":status==="available"?"arrow-available":"arrow-default";const path=svgElement("path",{d:pathData,class:"graph-edge"+(status?" "+status:""),"marker-end":"url(#"+marker+")"});const title=svgElement("title",{},group.from+" -- "+group.events.join(", ")+" --> "+group.to);path.appendChild(title);edgeLayer.appendChild(path);labelLayer.appendChild(svgElement("text",{x:labelX,y:labelY,class:"graph-edge-label"+(status?" "+status:""),"text-anchor":"middle"},group.events.join(" / ")))}svg.append(edgeLayer,labelLayer);const nodeLayer=svgElement("g");for(const state of states){const position=positions.get(state);const nodeWidth=Math.max(104,Math.min(190,state.length*9+38));const current=state===String(item.currentState);const previous=state===String(item.previousState)&&!current;const group=svgElement("g",{class:"graph-node"+(current?" current":previous?" previous":"")});group.appendChild(svgElement("rect",{x:position.x-nodeWidth/2,y:position.y-31,width:nodeWidth,height:62,rx:13}));group.appendChild(svgElement("text",{x:position.x,y:position.y-(current?7:0)},state));if(current)group.appendChild(svgElement("text",{x:position.x,y:position.y+17,class:"node-status"},"CURRENT"));nodeLayer.appendChild(group)}svg.appendChild(nodeLayer);host.appendChild(svg)}
function renderAvailableTransitions(item){const list=byId("availableTransitions");list.replaceChildren();const available=item.availableTransitions||(item.transitions||[]).filter(edge=>edge.fromState===item.currentState);text("availableCount",available.length+" events");if(!available.length){const empty=document.createElement("div");empty.className="available-empty";empty.textContent="当前状态没有可接收的事件";list.appendChild(empty);return}for(const edge of available){const row=document.createElement("div");const processing=String(edge.event)===String(item.processingEvent||"");row.className="available-transition"+(processing?" processing":"");const event=document.createElement("span");event.className="event-token";event.textContent=edge.event;const arrow=document.createElement("span");arrow.textContent="→";arrow.style.color="var(--muted)";const target=document.createElement("span");target.className="target-state";target.textContent=edge.toState;row.append(event,arrow,target);list.appendChild(row)}}
function inlineSVGStyles(source,target){const properties=["fill","stroke","stroke-width","stroke-dasharray","stroke-dashoffset","stroke-linecap","stroke-linejoin","opacity","filter","font-family","font-size","font-weight","letter-spacing","text-anchor","dominant-baseline","paint-order"];const sourceNodes=[source,...source.querySelectorAll("*")];const targetNodes=[target,...target.querySelectorAll("*")];for(let index=0;index<sourceNodes.length;index++){const computed=getComputedStyle(sourceNodes[index]);const declarations=[];for(const property of properties){const value=computed.getPropertyValue(property);if(value)declarations.push(property+":"+value)}if(declarations.length)targetNodes[index].setAttribute("style",declarations.join(";"))}}
function exportStateGraph(){const button=byId("exportGraph");const source=byId("stateGraph").querySelector("svg");if(!source||button.disabled)return;button.disabled=true;try{const clone=source.cloneNode(true);inlineSVGStyles(source,clone);const viewBox=source.viewBox.baseVal;const width=Math.max(1,Math.ceil(viewBox.width));const height=Math.max(1,Math.ceil(viewBox.height));clone.setAttribute("xmlns","http://www.w3.org/2000/svg");clone.setAttribute("width",String(width));clone.setAttribute("height",String(height));const backgroundColor=getComputedStyle(document.documentElement).getPropertyValue("--graph-bg").trim()||"#ffffff";clone.insertBefore(svgElement("rect",{x:viewBox.x,y:viewBox.y,width:width,height:height,fill:backgroundColor}),clone.firstChild);const serialized=new XMLSerializer().serializeToString(clone);const sourceURL=URL.createObjectURL(new Blob([serialized],{type:"image/svg+xml;charset=utf-8"}));const image=new Image();image.onload=()=>{URL.revokeObjectURL(sourceURL);const scale=2;const canvas=document.createElement("canvas");canvas.width=width*scale;canvas.height=height*scale;const context=canvas.getContext("2d");context.scale(scale,scale);context.fillStyle=backgroundColor;context.fillRect(0,0,width,height);context.drawImage(image,0,0,width,height);canvas.toBlob(blob=>{button.disabled=false;if(!blob)return;const current=app.fsms.find(item=>item.id===app.selected);const raw=current?String(current.displayName||current.name||"fsm"):"fsm";const safe=raw.replace(/[^a-zA-Z0-9._-]+/g,"-")||"fsm";const downloadURL=URL.createObjectURL(blob);const link=document.createElement("a");link.href=downloadURL;link.download=safe+"-state-graph.png";document.body.appendChild(link);link.click();link.remove();setTimeout(()=>URL.revokeObjectURL(downloadURL),1000)},"image/png")};image.onerror=()=>{URL.revokeObjectURL(sourceURL);button.disabled=false;console.error("failed to render FSM graph image")};image.src=sourceURL}catch(error){button.disabled=false;console.error(error)}}
function cell(row,value,className){const element=document.createElement("td");element.textContent=value;if(className)element.className=className;row.appendChild(element)}
function renderTraces(traces){const body=byId("traceBody");body.replaceChildren();const ordered=[...traces].reverse();for(const trace of ordered){const row=document.createElement("tr");cell(row,trace.beginTime?new Date(trace.beginTime/1000000).toLocaleTimeString():"-");cell(row,trace.event);cell(row,trace.fromState+" → "+trace.finalState);cell(row,trace.success?"success":"failed",trace.success?"ok":"fail");cell(row,formatNanos(trace.duration));cell(row,trace.error||"-",trace.error?"trace-error":"");body.appendChild(row)}}
initializeTheme();
initializeResizableColumns();
byId("fsmSearch").addEventListener("input",event=>{app.query=event.target.value;renderList()});
byId("exportGraph").disabled=true;
byId("exportGraph").onclick=exportStateGraph;
const stream=new EventSource("/api/events");
stream.onopen=()=>{byId("connectionDot").className="dot online";text("connectionText","实时连接")};
stream.onerror=()=>{byId("connectionDot").className="dot offline";text("connectionText","正在重连")};
stream.onmessage=()=>loadFSMs();
loadFSMs();
</script>
</body>
</html>`
