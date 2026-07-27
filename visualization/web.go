package visualization

const webPage = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>FSM Observatory</title>
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
  --success: #3f7d62;
  --danger: #b85045;
  --warning: #a06b28;
  --graph-bg: #faf9f5;
  --graph-node: #ffffff;
  --graph-line: #9b9288;
  --graph-text: #141413;
  --graph-muted: #6f6a63;
  --code: #3b3834;
  --sidebar-width: 288px;
  --topbar-height: 40px;
  --panel-alpha: 15%;
  --panel-detail-alpha: 10%;
  --panel-blur: 1.8px;
  --panel-shadow: .012;
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
  --success: #73ad8d;
  --danger: #df7c6e;
  --warning: #d5a45c;
  --graph-bg: #11110f;
  --graph-node: #262421;
  --graph-line: #777066;
  --graph-text: #faf9f5;
  --graph-muted: #aaa39a;
  --code: #e8dfd2;
}
* { box-sizing: border-box; }
html, body { width: 100%; height: 100%; }
body {
  margin: 0;
  overflow: hidden;
  background: var(--canvas);
  color: var(--ink);
  font: 14px/1.55 Inter, ui-sans-serif, system-ui, -apple-system, "Segoe UI", sans-serif;
}
button, input { font: inherit; }
.layout {
  width: 100%;
  height: 100vh;
  display: grid;
  grid-template-columns: var(--sidebar-width) 6px minmax(0, 1fr);
  transition: grid-template-columns .18s ease;
}
aside {
  min-width: 0;
  height: 100vh;
  padding: 16px 12px;
  overflow: auto;
  border-right: 1px solid var(--hairline);
  background: color-mix(in srgb, var(--surface-soft) var(--panel-alpha), transparent);
  backdrop-filter: blur(var(--panel-blur));
}
.aside-heading { display: flex; align-items: baseline; justify-content: space-between; padding: 0 8px 10px; }
.aside-title { font: 600 13px Georgia, "Times New Roman", serif; }
.count-pill { color: var(--muted); font-size: 10px; }
.sidebar-toggle {
  width: 26px;
  height: 26px;
  flex: 0 0 auto;
  display: grid;
  place-items: center;
  padding: 0;
  border: 1px solid transparent;
  border-radius: 7px;
  background: transparent;
  color: var(--muted);
  cursor: pointer;
}
.sidebar-toggle:hover { color: var(--primary); border-color: var(--hairline); background: color-mix(in srgb, var(--surface) 55%, transparent); }
.sidebar-toggle svg { width: 15px; height: 15px; fill: none; stroke: currentColor; stroke-width: 1.8; stroke-linecap: round; stroke-linejoin: round; }
.sidebar-expand-icon { display: none; }
body.sidebar-collapsed .layout { grid-template-columns: 44px 0 minmax(0, 1fr); }
body.sidebar-collapsed aside { padding: 8px 6px; overflow: hidden; }
body.sidebar-collapsed .aside-heading { justify-content: center; padding: 0; }
body.sidebar-collapsed .aside-title,
body.sidebar-collapsed .count-pill,
body.sidebar-collapsed .tree-toolbar,
body.sidebar-collapsed .fsm-list,
body.sidebar-collapsed .sidebar-collapse-icon { display: none; }
body.sidebar-collapsed #sidebarResize { visibility: hidden; pointer-events: none; }
body.sidebar-collapsed .sidebar-expand-icon { display: block; }
.tree-toolbar { padding: 0 6px 12px; }
.tree-search {
  width: 100%;
  height: 34px;
  padding: 0 11px;
  color: var(--ink);
  border: 1px solid var(--hairline);
  border-radius: 8px;
  background: color-mix(in srgb, var(--surface) 86%, transparent);
  outline: none;
}
.tree-search:focus { border-color: var(--primary); }
.fsm-list { display: grid; gap: 2px; }
.folder-node { margin: 2px 0; }
.folder-node > summary {
  min-height: 32px;
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 4px 8px;
  border-radius: 7px;
  color: var(--ink);
  cursor: pointer;
  list-style: none;
  font-size: 12px;
  font-weight: 600;
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
  padding: 8px 9px;
  color: var(--ink);
  text-align: left;
  border: 1px solid transparent;
  border-radius: 8px;
  background: transparent;
  cursor: pointer;
}
.fsm-button:hover { background: color-mix(in srgb, var(--surface) 65%, transparent); }
.fsm-button.active { background: var(--surface); border-color: color-mix(in srgb, var(--primary) 42%, var(--hairline)); }
.fsm-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 12px; font-weight: 600; }
.fsm-meta { grid-column: 1 / -1; overflow: hidden; color: var(--muted); text-overflow: ellipsis; white-space: nowrap; font-size: 10px; }
.state-mini { max-width: 82px; overflow: hidden; color: var(--success); text-overflow: ellipsis; white-space: nowrap; font-size: 10px; }
.mode-dot { display: inline-block; width: 5px; height: 5px; margin-right: 4px; border-radius: 50%; background: var(--primary); vertical-align: 1px; }
.mode-dot.pull { background: var(--warning); }
.resize-handle { position: relative; z-index: 20; cursor: col-resize; touch-action: none; }
.resize-handle::before { content: ""; position: absolute; inset: 0 auto 0 50%; width: 1px; background: var(--hairline); transform: translateX(-50%); }
.resize-handle:hover::before, .resize-handle.active::before { width: 3px; background: var(--primary); }
body.resizing { cursor: col-resize; user-select: none; }
.workspace { min-width: 0; height: 100vh; position: relative; overflow: hidden; background: var(--graph-bg); }
.topbar {
  height: var(--topbar-height);
  display: flex;
  align-items: center;
  justify-content: flex-end;
  padding: 0 8px;
  border-bottom: 1px solid color-mix(in srgb, var(--graph-line) 35%, transparent);
  background: color-mix(in srgb, var(--surface) var(--panel-alpha), transparent);
  position: relative;
  z-index: 30;
  backdrop-filter: blur(var(--panel-blur));
}
.topbar { gap: 6px; }
.opacity-control { position: relative; }
.opacity-popover {
  position: absolute;
  top: 34px;
  right: 0;
  z-index: 60;
  width: 220px;
  display: none;
  padding: 11px 12px 12px;
  border: 1px solid var(--hairline);
  border-radius: 9px;
  background: color-mix(in srgb, var(--surface) 92%, transparent);
  color: var(--ink);
  box-shadow: 0 10px 28px rgba(0,0,0,.12);
  backdrop-filter: blur(14px);
}
.opacity-control.open .opacity-popover { display: block; }
.opacity-popover-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 9px; font-size: 11px; }
.opacity-popover-head strong { font-family: Georgia, "Times New Roman", serif; }
.opacity-value { color: var(--primary); font: 600 10px ui-monospace, monospace; }
.opacity-slider { width: 100%; height: 4px; accent-color: var(--primary); cursor: pointer; }
.opacity-scale { display: flex; justify-content: space-between; margin-top: 5px; color: var(--muted); font-size: 8px; }
.icon-button {
  width: 28px;
  height: 28px;
  display: grid;
  place-items: center;
  padding: 0;
  border: 1px solid color-mix(in srgb, var(--graph-line) 60%, transparent);
  border-radius: 50%;
  background: color-mix(in srgb, var(--surface) 48%, transparent);
  color: var(--ink);
  cursor: pointer;
}
.icon-button:hover { color: var(--primary); border-color: var(--primary); }
.icon-button:focus-visible, .resize-handle:focus-visible, .fsm-button:focus-visible, summary:focus-visible, .panel-toggle:focus-visible {
  outline: 2px solid var(--primary);
  outline-offset: 2px;
}
.icon-button:disabled { cursor: not-allowed; opacity: .42; }
.icon-button svg { width: 15px; height: 15px; fill: none; stroke: currentColor; stroke-width: 1.8; stroke-linecap: round; stroke-linejoin: round; }
.theme-moon { display: none; }
:root[data-theme="dark"] .theme-sun { display: none; }
:root[data-theme="dark"] .theme-moon { display: block; }
.empty {
  position: absolute;
  inset: var(--topbar-height) 0 0;
  display: grid;
  place-items: center;
  color: var(--graph-muted);
  text-align: center;
}
.empty strong { display: block; margin-bottom: 6px; color: var(--graph-text); font: 600 22px Georgia, "Times New Roman", serif; }
.content { display: none; position: absolute; inset: var(--topbar-height) 0 0; overflow: hidden; }
.content.visible { display: block; }
.graph-surface { position: absolute; inset: 0; overflow: hidden; background: var(--graph-bg); }
.state-graph { position: absolute; inset: 0; overflow: hidden; cursor: grab; touch-action: none; }
.state-graph.dragging { cursor: grabbing; }
.graph-stage {
  position: absolute;
  left: 50%;
  top: 50%;
  transform: translate(-50%, -50%);
  transform-origin: center;
  will-change: transform;
}
.graph-stage svg { display: block; width: 100%; height: 100%; overflow: visible; }
.graph-edge { fill: none; stroke: var(--graph-line); stroke-width: 2; opacity: .7; transition: stroke .2s, opacity .2s; }
.graph-edge.available { stroke: var(--primary); opacity: .95; }
.graph-edge.recent { stroke: var(--success); stroke-width: 3; opacity: 1; }
.graph-edge.processing { stroke: var(--warning); stroke-width: 3; opacity: 1; stroke-dasharray: 10 7; animation: edge-flow .8s linear infinite; }
.graph-edge.failed { stroke: var(--danger); stroke-width: 3; opacity: 1; }
.graph-edge-label { fill: var(--graph-muted); font: 600 12px Inter, sans-serif; paint-order: stroke; stroke: var(--graph-bg); stroke-width: 5px; stroke-linejoin: round; pointer-events: none; }
.graph-edge-label.available { fill: var(--primary); }
.graph-edge-label.recent { fill: var(--success); }
.graph-edge-label.processing { fill: var(--warning); }
.graph-edge-label.failed { fill: var(--danger); }
.graph-node { cursor: help; outline: none; }
.graph-node rect { fill: var(--graph-node); stroke: var(--graph-line); stroke-width: 2; transition: stroke .15s, fill .15s; }
.graph-node text { fill: var(--graph-text); font: 650 13px Inter, sans-serif; text-anchor: middle; dominant-baseline: middle; pointer-events: none; }
.graph-node .node-status { fill: var(--success); font-size: 9px; letter-spacing: .12em; }
.graph-node:hover rect, .graph-node:focus rect { stroke: var(--graph-text); }
.graph-node.current rect { fill: color-mix(in srgb, var(--success) 14%, var(--graph-node)); stroke: var(--success); stroke-width: 3; }
.graph-node.previous rect { stroke: var(--primary); }
@keyframes edge-flow { to { stroke-dashoffset: -17; } }
.canvas-tools { position: absolute; right: 12px; top: 50%; bottom: auto; transform: translateY(-50%); z-index: 8; display: flex; gap: 7px; }
.canvas-hint {
  position: absolute;
  left: 50%;
  bottom: 12px;
  z-index: 7;
  padding: 5px 9px;
  border: 1px solid color-mix(in srgb, var(--graph-line) 45%, transparent);
  border-radius: 999px;
  background: color-mix(in srgb, var(--surface) 46%, transparent);
  color: var(--graph-muted);
  font-size: 10px;
  transform: translateX(-50%);
  pointer-events: none;
  backdrop-filter: blur(8px);
}
.node-tooltip {
  position: absolute;
  z-index: 40;
  min-width: 190px;
  max-width: 280px;
  display: none;
  padding: 11px 12px;
  border: 1px solid color-mix(in srgb, var(--primary) 55%, transparent);
  border-radius: 9px;
  background: color-mix(in srgb, var(--surface) 72%, transparent);
  color: var(--ink);
  box-shadow: 0 10px 30px rgba(0,0,0,.2);
  pointer-events: none;
  backdrop-filter: blur(12px);
}
.node-tooltip.visible { display: block; }
.node-tooltip strong { display: block; margin-bottom: 6px; font: 600 14px Georgia, "Times New Roman", serif; }
.node-tooltip-row { display: flex; justify-content: space-between; gap: 16px; color: var(--muted); font-size: 10px; }
.node-tooltip-row span:last-child { color: var(--ink); text-align: right; }
.floating-panel {
  position: absolute;
  z-index: 12;
  overflow: hidden;
  border: 1px solid color-mix(in srgb, var(--hairline) 72%, transparent);
  border-radius: 11px;
  background: color-mix(in srgb, var(--surface) var(--panel-alpha), transparent);
  color: var(--ink);
  box-shadow: 0 10px 28px rgba(0,0,0,var(--panel-shadow));
  backdrop-filter: blur(var(--panel-blur));
  transition: width .18s, height .18s, max-height .18s;
}
.payload-panel { top: 14px; right: 14px; width: min(350px, calc(100% - 28px)); max-height: calc(100% - 28px); }
.trace-panel { right: 14px; bottom: 14px; left: 14px; max-height: min(285px, 42%); }
.panel-toggle {
  width: 100%;
  min-height: 38px;
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 0 12px;
  color: inherit;
  border: 0;
  border-bottom: 1px solid color-mix(in srgb, var(--hairline) 72%, transparent);
  background: transparent;
  cursor: pointer;
  text-align: left;
}
.panel-toggle strong { font: 600 13px Georgia, "Times New Roman", serif; }
.panel-summary { margin-left: auto; color: var(--muted); font-size: 10px; }
.panel-chevron { color: var(--primary); transition: transform .15s; }
.floating-panel.collapsed { width: auto; max-height: 40px; }
.trace-panel.collapsed { left: auto; }
.floating-panel.collapsed .panel-toggle { border-bottom-color: transparent; }
.floating-panel.collapsed .panel-body { display: none; }
.floating-panel.collapsed .panel-chevron { transform: rotate(-90deg); }
.panel-body { overflow: auto; }
.payload-panel .panel-body { max-height: calc(100vh - 110px); padding: 12px; }
.trace-panel .panel-body { max-height: 245px; }
.payload-error { display: none; margin-bottom: 8px; color: var(--warning); font-size: 11px; }
pre { max-height: 270px; margin: 0; overflow: auto; color: var(--code); white-space: pre-wrap; word-break: break-word; font: 11px/1.65 ui-monospace, SFMono-Regular, Menlo, monospace; }
.meta { display: grid; grid-template-columns: 1fr 1fr; gap: 7px; margin-top: 12px; }
.meta div { padding: 8px; border: 1px solid color-mix(in srgb, var(--hairline) 72%, transparent); border-radius: 7px; background: color-mix(in srgb, var(--surface) var(--panel-detail-alpha), transparent); }
.meta label { display: block; color: var(--muted); font-size: 8px; letter-spacing: .08em; text-transform: uppercase; }
.meta span { display: block; margin-top: 2px; overflow-wrap: anywhere; font: 9px ui-monospace, monospace; }
table { width: 100%; min-width: 720px; border-collapse: collapse; }
th, td { padding: 8px 10px; border-bottom: 1px solid color-mix(in srgb, var(--hairline) 70%, transparent); text-align: left; font-size: 10px; }
th { position: sticky; top: 0; background: color-mix(in srgb, var(--surface-soft) var(--panel-detail-alpha), transparent); color: var(--muted); font-weight: 600; }
.ok { color: var(--success); }
.fail, .trace-error { color: var(--danger); }
.trace-error { max-width: 260px; white-space: normal; }
.available-panel {
  position: absolute;
  top: 14px;
  left: 14px;
  z-index: 10;
  max-width: min(520px, calc(100% - 400px));
  padding: 0;
  border: 1px solid color-mix(in srgb, var(--graph-line) 45%, transparent);
  border-radius: 9px;
  background: color-mix(in srgb, var(--surface) var(--panel-alpha), transparent);
  color: var(--ink);
  backdrop-filter: blur(var(--panel-blur));
}
.available-panel .panel-body { padding: 0 10px 10px; }
.available-head { display: flex; align-items: center; gap: 10px; margin-bottom: 6px; color: #aaa39a; font-size: 10px; }
.available-head span { margin-left: auto; }
.available-list { display: flex; flex-wrap: wrap; gap: 6px; }
.available-transition { display: flex; align-items: center; gap: 7px; padding: 5px 7px; border: 1px solid color-mix(in srgb, var(--graph-line) 55%, transparent); border-radius: 6px; background: color-mix(in srgb, var(--surface) var(--panel-detail-alpha), transparent); }
.available-transition.processing { border-color: var(--warning); }
.event-token { color: var(--primary); font: 650 10px ui-monospace, monospace; }
.available-transition.processing .event-token { color: var(--warning); }
.target-state { color: var(--ink); font-size: 10px; }
.available-empty { color: var(--muted); font-size: 11px; }
.collection-error {
  position: absolute;
  top: 14px;
  left: 50%;
  z-index: 20;
  display: none;
  max-width: 520px;
  padding: 7px 10px;
  border: 1px solid color-mix(in srgb, var(--danger) 50%, transparent);
  border-radius: 7px;
  background: color-mix(in srgb, var(--danger) 12%, var(--surface-dark));
  color: #f3a096;
  font-size: 11px;
  transform: translateX(-50%);
}
.settings-modal { position: fixed; inset: 0; z-index: 100; display: none; place-items: center; padding: 24px; background: color-mix(in srgb, var(--surface-dark) 42%, transparent); backdrop-filter: blur(8px); }
.settings-modal.open { display: grid; }
.settings-shell { width: min(1040px, 100%); height: min(720px, 100%); min-height: 520px; display: grid; grid-template-rows: 64px minmax(0, 1fr); overflow: hidden; border: 1px solid var(--hairline); border-radius: 16px; background: var(--surface); box-shadow: 0 28px 80px rgba(0,0,0,.24); }
.settings-head { display: flex; align-items: center; gap: 12px; padding: 0 20px 0 24px; border-bottom: 1px solid var(--hairline); }
.settings-head-title { display: grid; gap: 1px; }
.settings-head-title strong { font: 600 18px Georgia, "Times New Roman", serif; }
.settings-head-title span { color: var(--muted); font-size: 10px; }
.settings-close { margin-left: auto; font-size: 20px; line-height: 1; }
.settings-layout { min-height: 0; display: grid; grid-template-columns: 220px minmax(0, 1fr); }
.settings-nav { padding: 20px 12px; border-right: 1px solid var(--hairline); background: color-mix(in srgb, var(--surface-soft) 44%, var(--surface)); }
.settings-nav-label { display: block; padding: 0 10px 8px; color: var(--muted); font-size: 9px; font-weight: 700; letter-spacing: .1em; text-transform: uppercase; }
.settings-nav-item { width: 100%; display: grid; grid-template-columns: 32px minmax(0, 1fr); gap: 1px 10px; align-items: center; padding: 9px 10px; color: var(--ink); text-align: left; border: 1px solid transparent; border-radius: 9px; background: transparent; }
.settings-nav-item.active { border-color: color-mix(in srgb, var(--primary) 28%, var(--hairline)); background: var(--surface); box-shadow: 0 3px 12px rgba(0,0,0,.04); }
.settings-nav-icon { grid-row: 1 / 3; width: 32px; height: 32px; display: grid; place-items: center; border-radius: 8px; background: color-mix(in srgb, var(--primary) 12%, transparent); color: var(--primary); }
.settings-nav-icon svg { width: 17px; height: 17px; fill: none; stroke: currentColor; stroke-width: 1.8; stroke-linecap: round; stroke-linejoin: round; }
.settings-nav-item strong { font-size: 12px; }
.settings-nav-item small { overflow: hidden; color: var(--muted); text-overflow: ellipsis; white-space: nowrap; font-size: 9px; }
.settings-content { min-width: 0; overflow: auto; padding: 28px 32px 36px; }
.settings-section-head { margin-bottom: 22px; }
.settings-section-head h2 { margin: 0 0 5px; font: 600 22px Georgia, "Times New Roman", serif; }
.settings-section-head p { margin: 0; color: var(--muted); font-size: 12px; }
.settings-block { margin-top: 18px; padding: 20px; border: 1px solid var(--hairline); border-radius: 12px; background: color-mix(in srgb, var(--surface-soft) 16%, var(--surface)); }
.settings-block-head { display: flex; align-items: start; justify-content: space-between; gap: 20px; margin-bottom: 18px; }
.settings-block-head strong { display: block; margin-bottom: 3px; font-size: 13px; }
.settings-block-head span { color: var(--muted); font-size: 10px; }
.settings-block-badge { flex: 0 0 auto; padding: 3px 7px; border-radius: 999px; background: color-mix(in srgb, var(--success) 12%, transparent); color: var(--success) !important; font-size: 9px !important; font-weight: 650; }
.discovery-form { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px 16px; }
.discovery-field { min-width: 0; }
.discovery-field.wide { grid-column: 1 / -1; }
.discovery-field label { display: block; margin-bottom: 5px; color: var(--muted); font-size: 10px; font-weight: 600; }
.discovery-field input { width: 100%; height: 38px; padding: 0 11px; color: var(--ink); border: 1px solid var(--hairline); border-radius: 8px; background: var(--surface); outline: none; transition: border-color .15s, box-shadow .15s; }
.discovery-field input::placeholder { color: color-mix(in srgb, var(--muted) 62%, transparent); }
.discovery-field input:focus { border-color: var(--primary); box-shadow: 0 0 0 3px color-mix(in srgb, var(--primary) 13%, transparent); }
.discovery-form-footer { grid-column: 1 / -1; display: flex; align-items: center; gap: 12px; padding-top: 2px; }
.discovery-submit, .discovery-action { min-height: 34px; padding: 0 13px; border: 1px solid color-mix(in srgb, var(--primary) 70%, var(--hairline)); border-radius: 8px; background: var(--primary); color: #fff; font-size: 11px; font-weight: 650; cursor: pointer; }
.discovery-submit:hover { filter: brightness(.95); }
.discovery-submit:disabled { cursor: wait; opacity: .55; }
.discovery-form-status { color: var(--success); font-size: 10px; }
.discovery-form-status.error { color: var(--danger); }
.discovery-list { display: grid; gap: 10px; }
.discovery-empty { padding: 26px; border: 1px dashed var(--hairline); border-radius: 10px; color: var(--muted); text-align: center; font-size: 11px; }
.discovery-item { position: relative; padding: 15px 16px; border: 1px solid var(--hairline); border-radius: 10px; background: var(--surface); }
.discovery-item-head { min-width: 0; display: flex; align-items: center; gap: 8px; padding-right: 160px; }
.discovery-item-head strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 12px; }
.discovery-status-dot { width: 7px; height: 7px; flex: 0 0 auto; border-radius: 50%; background: var(--success); box-shadow: 0 0 0 3px color-mix(in srgb, var(--success) 12%, transparent); }
.discovery-status-dot.error { background: var(--danger); box-shadow: 0 0 0 3px color-mix(in srgb, var(--danger) 12%, transparent); }
.discovery-item-count { margin-left: auto; color: var(--success); font: 600 9px ui-monospace, monospace; white-space: nowrap; }
.discovery-item-meta { margin: 7px 0 0 15px; overflow: hidden; color: var(--muted); text-overflow: ellipsis; white-space: nowrap; font: 9px ui-monospace, monospace; }
.discovery-item-error { margin: 9px 160px 0 15px; padding: 7px 9px; border-radius: 6px; background: color-mix(in srgb, var(--danger) 9%, transparent); color: var(--danger); font-size: 9px; overflow-wrap: anywhere; }
.discovery-actions { position: absolute; top: 50%; right: 14px; display: flex; gap: 7px; transform: translateY(-50%); }
.discovery-action { min-height: 30px; padding: 0 10px; border-color: var(--hairline); background: transparent; color: var(--ink); font-size: 10px; font-weight: 600; }
.discovery-action:hover { border-color: var(--primary); color: var(--primary); }
.discovery-action.danger:hover { border-color: var(--danger); color: var(--danger); }
@media (max-width: 820px) {
  :root { --sidebar-width: 230px; }
  .payload-panel { width: min(310px, calc(100% - 24px)); }
  .available-panel { max-width: calc(100% - 24px); right: 12px; }
  .canvas-hint { display: none; }
  .settings-modal { padding: 12px; }
  .settings-shell { height: min(760px, 100%); }
  .settings-layout { grid-template-columns: 1fr; grid-template-rows: auto minmax(0, 1fr); }
  .settings-nav { padding: 10px 16px; border-right: 0; border-bottom: 1px solid var(--hairline); }
  .settings-nav-label, .settings-nav-item small { display: none; }
  .settings-nav-item { width: auto; display: inline-grid; grid-template-columns: 24px auto; padding: 6px 10px; }
  .settings-nav-icon { grid-row: auto; width: 24px; height: 24px; background: transparent; }
  .settings-content { padding: 22px 20px 28px; }
  .discovery-form { grid-template-columns: 1fr; }
  .discovery-field.wide, .discovery-form-footer { grid-column: auto; }
  .discovery-item-head { padding-right: 0; }
  .discovery-item-meta, .discovery-item-error { margin-right: 0; }
  .discovery-actions { position: static; margin: 12px 0 0 15px; transform: none; }
}
@media (max-width: 520px) {
  .settings-modal { padding: 0; }
  .settings-shell { width: 100%; height: 100%; min-height: 0; border: 0; border-radius: 0; }
  .settings-content { padding: 18px 14px 24px; }
  .settings-block { padding: 15px; }
  .discovery-form-footer { align-items: stretch; flex-direction: column; }
}
</style>
</head>
<body>
<div class="layout">
  <aside>
    <div class="aside-heading"><span class="aside-title">Machines</span><span class="count-pill"><span id="fsmCount">0</span> registered</span><button id="sidebarToggle" class="sidebar-toggle" type="button" aria-label="折叠左侧栏" title="折叠左侧栏"><svg class="sidebar-collapse-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M7 4v16M16 8l-4 4 4 4"></path></svg><svg class="sidebar-expand-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M7 4v16M12 8l4 4-4 4"></path></svg></button></div>
    <div class="tree-toolbar"><input id="fsmSearch" class="tree-search" type="search" placeholder="搜索 FSM、状态或路径" aria-label="搜索 FSM"></div>
    <div id="fsmList" class="fsm-list"></div>
  </aside>
  <div id="sidebarResize" class="resize-handle" role="separator" aria-label="调整状态机列表宽度" aria-orientation="vertical" tabindex="0"></div>
  <main class="workspace">
    <header class="topbar">
      <div id="opacityControl" class="opacity-control">
        <button id="opacityToggle" class="icon-button" type="button" aria-label="调节浮层透明度" title="调节浮层透明度" aria-expanded="false">
          <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 3s5 6 5 10a5 5 0 0 1-10 0c0-4 5-10 5-10z"></path><path d="M9 14.5c.7 1.3 1.7 2 3 2"></path></svg>
        </button>
        <div id="opacityPopover" class="opacity-popover" role="dialog" aria-label="浮层透明度">
          <div class="opacity-popover-head"><strong>浮层透明度</strong><span id="opacityValue" class="opacity-value">85%</span></div>
          <input id="opacitySlider" class="opacity-slider" type="range" min="0" max="100" step="1" value="85" aria-label="浮层透明度">
          <div class="opacity-scale"><span>不透明</span><span>全透明</span></div>
        </div>
      </div>
      <button id="themeToggle" class="icon-button" type="button" aria-label="切换主题" title="切换主题">
        <svg class="theme-sun" viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="4"></circle><path d="M12 2v2M12 20v2M4.93 4.93l1.42 1.42M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.42-1.42M17.66 6.34l1.41-1.41"></path></svg>
        <svg class="theme-moon" viewBox="0 0 24 24" aria-hidden="true"><path d="M20.5 14.2A8.5 8.5 0 0 1 9.8 3.5a8.5 8.5 0 1 0 10.7 10.7z"></path></svg>
      </button>
      <button id="discoveryToggle" class="icon-button" type="button" aria-label="打开设置" title="设置" aria-expanded="false">
        <svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="3"></circle><path d="M19.4 15a1.7 1.7 0 0 0 .34 1.88l.06.06-1.86 1.86-.06-.06A1.7 1.7 0 0 0 16 18.4a1.7 1.7 0 0 0-1 .6 1.7 1.7 0 0 0-.4 1.1V20h-2.6v-.09A1.7 1.7 0 0 0 10.9 18.4a1.7 1.7 0 0 0-1.88.34l-.06.06-1.86-1.86.06-.06A1.7 1.7 0 0 0 7.5 15a1.7 1.7 0 0 0-1.5-1H5.9v-2.6H6a1.7 1.7 0 0 0 1.5-1 1.7 1.7 0 0 0-.34-1.88l-.06-.06L8.96 6.6l.06.06A1.7 1.7 0 0 0 10.9 7a1.7 1.7 0 0 0 1-1.5v-.1h2.6v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.88-.34l.06-.06 1.86 1.86-.06.06A1.7 1.7 0 0 0 18.9 10.4a1.7 1.7 0 0 0 1.5 1h.1V14h-.1a1.7 1.7 0 0 0-1 .6z"></path></svg>
      </button>
    </header>
    <div id="empty" class="empty"><div><strong>等待 FSM 注册</strong><span>注册完成后将在此显示状态图。</span></div></div>
    <div id="content" class="content">
      <section class="graph-surface" aria-label="FSM 状态图画布">
        <div id="stateGraph" class="state-graph"></div>
        <div id="nodeTooltip" class="node-tooltip" role="tooltip"></div>
        <div id="collectionError" class="collection-error"></div>

        <section id="availablePanel" class="floating-panel available-panel collapsed">
          <button class="panel-toggle" type="button" data-panel="availablePanel" aria-expanded="false">
            <span class="panel-chevron">▾</span><strong>可接收事件</strong><span id="availableCount" class="panel-summary"></span>
          </button>
          <div class="panel-body"><div id="availableTransitions" class="available-list"></div></div>
        </section>

        <section id="payloadPanel" class="floating-panel payload-panel collapsed">
          <button class="panel-toggle" type="button" data-panel="payloadPanel" aria-expanded="false">
            <span class="panel-chevron">▾</span><strong>Payload</strong><span id="payloadPanelSummary" class="panel-summary"></span>
          </button>
          <div class="panel-body">
            <div id="payloadError" class="payload-error"></div>
            <pre id="payload"></pre>
            <div class="meta">
              <div><label>Transition ID</label><span id="transitionId"></span></div>
              <div><label>Updated</label><span id="updatedAt"></span></div>
              <div><label>Previous State</label><span id="previousState"></span></div>
              <div><label>Previous Event</label><span id="previousEvent"></span></div>
            </div>
          </div>
        </section>

        <section id="tracePanel" class="floating-panel trace-panel collapsed">
          <button class="panel-toggle" type="button" data-panel="tracePanel" aria-expanded="false">
            <span class="panel-chevron">▾</span><strong>Trace</strong><span id="traceCount" class="panel-summary"></span>
          </button>
          <div class="panel-body">
            <table><thead><tr><th>时间</th><th>事件</th><th>状态流转</th><th>结果</th><th>耗时</th><th>错误</th></tr></thead><tbody id="traceBody"></tbody></table>
          </div>
        </section>

        <div class="canvas-tools">
          <button id="exportGraph" class="icon-button" type="button" aria-label="导出状态图 PNG" title="导出状态图 PNG"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 3v12M7 10l5 5 5-5M4 19h16"></path></svg></button>
        </div>
        <div class="canvas-hint">拖动画布移动状态图 · 双击复位</div>
      </section>
    </div>
  </main>
</div>
<div id="discoveryModal" class="settings-modal" role="dialog" aria-modal="true" aria-labelledby="settingsTitle">
  <section class="settings-shell">
    <header class="settings-head">
      <div class="settings-head-title"><strong id="settingsTitle">设置</strong><span>管理 Observatory 的连接与运行选项</span></div>
      <button id="discoveryClose" class="icon-button settings-close" type="button" aria-label="关闭设置" title="关闭">×</button>
    </header>
    <div class="settings-layout">
      <nav class="settings-nav" aria-label="设置分类">
        <span class="settings-nav-label">连接</span>
        <button class="settings-nav-item active" type="button" aria-current="page">
          <span class="settings-nav-icon"><svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="6" cy="12" r="2"></circle><circle cx="18" cy="6" r="2"></circle><circle cx="18" cy="18" r="2"></circle><path d="M8 11l8-4M8 13l8 4"></path></svg></span>
          <strong>服务发现</strong><small>远程 FSM 与 Pull 配置</small>
        </button>
      </nav>
      <main class="settings-content">
        <div class="settings-section-head">
          <h2 id="discoveryTitle">服务发现</h2>
          <p>由 Server 自动发现远程 FSM，并持续拉取实时状态数据。</p>
        </div>
        <section class="settings-block" aria-labelledby="addDiscoveryTitle">
          <div class="settings-block-head"><div><strong id="addDiscoveryTitle">添加发现源</strong><span>填写注册中心提供的 HTTP 发现接口与同步周期。</span></div><span class="settings-block-badge">HTTP</span></div>
          <form id="discoveryForm" class="discovery-form">
            <div class="discovery-field"><label for="discoveryId">配置 ID</label><input id="discoveryId" name="id" required placeholder="production-services"></div>
            <div class="discovery-field"><label for="discoveryName">显示名称</label><input id="discoveryName" name="name" placeholder="Production Services"></div>
            <div class="discovery-field wide"><label for="discoveryEndpoint">发现接口地址</label><input id="discoveryEndpoint" name="endpoint" type="url" required placeholder="http://registry:8080/fsm-services"></div>
            <div class="discovery-field wide"><label for="discoveryPathPrefix">目录前缀</label><input id="discoveryPathPrefix" name="pathPrefix" placeholder="production/discovered"></div>
            <div class="discovery-field"><label for="discoveryRefresh">发现周期（秒）</label><input id="discoveryRefresh" name="refreshIntervalSeconds" type="number" min="0.1" step="0.1" value="30"></div>
            <div class="discovery-field"><label for="discoveryPull">Pull 周期（秒）</label><input id="discoveryPull" name="pullIntervalSeconds" type="number" min="0.1" step="0.1" value="5"></div>
            <div class="discovery-form-footer"><button id="discoverySubmit" class="discovery-submit" type="submit">添加发现配置</button><span id="discoveryFormStatus" class="discovery-form-status" role="status"></span></div>
          </form>
        </section>
        <section class="settings-block" aria-labelledby="configuredDiscoveryTitle">
          <div class="settings-block-head"><div><strong id="configuredDiscoveryTitle">已配置的发现源</strong><span>查看同步状态，或手动触发一次服务发现。</span></div></div>
          <div id="discoveryList" class="discovery-list"></div>
        </section>
      </main>
    </div>
  </section>
</div>
<script>
const app={fsms:[],discoveries:[],selected:"",query:"",graphOffsets:{}};
const byId=id=>document.getElementById(id);
function readPreference(key){try{return localStorage.getItem(key)||""}catch(error){return ""}}
function writePreference(key,value){try{if(value==="")localStorage.removeItem(key);else localStorage.setItem(key,value)}catch(error){}}
function applyTheme(theme,persist){const selected=theme==="light"?"light":"dark";document.documentElement.dataset.theme=selected;const button=byId("themeToggle");if(button){const target=selected==="dark"?"浅色":"深色";button.setAttribute("aria-label","切换为"+target+"主题");button.title="切换为"+target+"主题"}if(persist)writePreference("fsm-visualization-theme",selected);if(app.fsms.length)render()}
function initializeTheme(){const stored=readPreference("fsm-visualization-theme");const preferred=stored||(window.matchMedia&&window.matchMedia("(prefers-color-scheme: light)").matches?"light":"dark");applyTheme(preferred,false);byId("themeToggle").onclick=()=>applyTheme(document.documentElement.dataset.theme==="light"?"dark":"light",true)}
function setupColumnResizer(options){const handle=byId(options.handle);const container=document.querySelector(options.container);const first=document.querySelector(options.first);const root=document.documentElement;const stored=Number(readPreference(options.storage));if(Number.isFinite(stored)&&stored>0)root.style.setProperty(options.variable,stored+"px");function setSize(value,persist){const bounds=container.getBoundingClientRect();const maximum=Math.max(options.minimum,bounds.width-handle.offsetWidth-options.trailingMinimum);const size=Math.round(Math.min(maximum,Math.max(options.minimum,value)));root.style.setProperty(options.variable,size+"px");handle.setAttribute("aria-valuemin",String(options.minimum));handle.setAttribute("aria-valuemax",String(Math.round(maximum)));handle.setAttribute("aria-valuenow",String(size));if(persist)writePreference(options.storage,String(size))}handle.addEventListener("pointerdown",event=>{if(window.matchMedia("(max-width: 1100px)").matches)return;event.preventDefault();handle.classList.add("active");document.body.classList.add("resizing");const move=moveEvent=>{const bounds=container.getBoundingClientRect();setSize(moveEvent.clientX-bounds.left,false)};const stop=stopEvent=>{if(Number.isFinite(stopEvent.clientX))move(stopEvent);handle.classList.remove("active");document.body.classList.remove("resizing");window.removeEventListener("pointermove",move);window.removeEventListener("pointerup",stop);window.removeEventListener("pointercancel",stop);writePreference(options.storage,String(Math.round(first.getBoundingClientRect().width)))};window.addEventListener("pointermove",move);window.addEventListener("pointerup",stop);window.addEventListener("pointercancel",stop)});handle.addEventListener("keydown",event=>{if(event.key!=="ArrowLeft"&&event.key!=="ArrowRight")return;event.preventDefault();setSize(first.getBoundingClientRect().width+(event.key==="ArrowLeft"?-16:16),true)});handle.addEventListener("dblclick",()=>{root.style.removeProperty(options.variable);writePreference(options.storage,"");});}
function initializeResizableColumns(){setupColumnResizer({handle:"sidebarResize",container:".layout",first:".layout > aside",variable:"--sidebar-width",storage:"fsm-sidebar-width",minimum:190,trailingMinimum:360})}
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
  text("payloadPanelSummary",(item.currentState||"empty")+" · "+(item.mode||"push"));
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
function tooltipRow(label,value){
  const row=document.createElement("div");
  row.className="node-tooltip-row";
  const key=document.createElement("span");
  key.textContent=label;
  const content=document.createElement("span");
  content.textContent=value;
  row.append(key,content);
  return row;
}
function moveNodeTooltip(event){
  const tooltip=byId("nodeTooltip");
  const surface=document.querySelector(".graph-surface");
  const bounds=surface.getBoundingClientRect();
  const width=tooltip.offsetWidth||220;
  const height=tooltip.offsetHeight||120;
  let left=event.clientX-bounds.left+14;
  let top=event.clientY-bounds.top+14;
  left=Math.max(8,Math.min(bounds.width-width-8,left));
  top=Math.max(8,Math.min(bounds.height-height-8,top));
  tooltip.style.left=left+"px";
  tooltip.style.top=top+"px";
}
function showNodeTooltip(event,state,item){
  const tooltip=byId("nodeTooltip");
  const transitions=item.transitions||[];
  const outgoing=transitions.filter(edge=>String(edge.fromState)===state);
  const incoming=transitions.filter(edge=>String(edge.toState)===state);
  const title=document.createElement("strong");
  title.textContent=state||"(empty)";
  const status=state===String(item.currentState)?"当前状态":state===String(item.previousState)?"上一状态":"普通状态";
  const accepted=outgoing.length?outgoing.map(edge=>String(edge.event)+" → "+String(edge.toState)).join("，"):"无";
  tooltip.replaceChildren(
    title,
    tooltipRow("状态",status),
    tooltipRow("进入转移",String(incoming.length)),
    tooltipRow("离开转移",String(outgoing.length)),
    tooltipRow("可接收事件",accepted)
  );
  tooltip.classList.add("visible");
  moveNodeTooltip(event);
}
function hideNodeTooltip(){byId("nodeTooltip").classList.remove("visible")}
function applyGraphOffset(stage,offset){
  stage.style.transform="translate(calc(-50% + "+offset.x+"px), calc(-50% + "+offset.y+"px))";
}
function setupGraphPan(host,stage,key){
  const offset=app.graphOffsets[key]||{x:0,y:0};
  app.graphOffsets[key]=offset;
  applyGraphOffset(stage,offset);
  let dragging=false;
  let pointerID=0;
  let originX=0;
  let originY=0;
  let startX=0;
  let startY=0;
  host.onpointerdown=event=>{
    if(event.button!==0)return;
    dragging=true;
    pointerID=event.pointerId;
    originX=event.clientX;
    originY=event.clientY;
    startX=offset.x;
    startY=offset.y;
    hideNodeTooltip();
    host.classList.add("dragging");
    host.setPointerCapture(pointerID);
    event.preventDefault();
  };
  host.onpointermove=event=>{
    if(!dragging||event.pointerId!==pointerID)return;
    offset.x=startX+event.clientX-originX;
    offset.y=startY+event.clientY-originY;
    applyGraphOffset(stage,offset);
  };
  const stop=event=>{
    if(!dragging||event.pointerId!==pointerID)return;
    dragging=false;
    host.classList.remove("dragging");
    if(host.hasPointerCapture(pointerID))host.releasePointerCapture(pointerID);
  };
  host.onpointerup=stop;
  host.onpointercancel=stop;
  host.ondblclick=event=>{
    if(event.target.closest&&event.target.closest(".graph-node"))return;
    offset.x=0;
    offset.y=0;
    applyGraphOffset(stage,offset);
  };
}
function renderStateGraph(item){
  const host=byId("stateGraph");
  host.replaceChildren();
  hideNodeTooltip();
  const transitions=item.transitions||[];
  const stateValues=item.states&&item.states.length?item.states:[item.currentState,...transitions.flatMap(edge=>[edge.fromState,edge.toState])];
  const states=[...new Set(stateValues.map(String))].sort();
  if(!states.length){
    const empty=document.createElement("div");
    empty.className="available-empty";
    empty.textContent="尚未配置状态";
    host.appendChild(empty);
    return;
  }
  const radius=Math.max(155,Math.min(360,states.length*38));
  const width=Math.max(760,radius*2+240);
  const height=Math.max(560,radius*2+180);
  const centerX=width/2;
  const centerY=height/2;
  const svg=svgElement("svg",{viewBox:"0 0 "+width+" "+height,role:"img","aria-label":"FSM 状态转移图"});
  const defs=svgElement("defs");
  const themeStyle=getComputedStyle(document.documentElement);
  const themeColor=name=>themeStyle.getPropertyValue(name).trim();
  addArrowMarker(defs,"arrow-default",themeColor("--graph-line"));
  addArrowMarker(defs,"arrow-available",themeColor("--primary"));
  addArrowMarker(defs,"arrow-recent",themeColor("--success"));
  addArrowMarker(defs,"arrow-processing",themeColor("--warning"));
  addArrowMarker(defs,"arrow-failed",themeColor("--danger"));
  svg.appendChild(defs);
  const positions=new Map();
  for(let index=0;index<states.length;index++){
    if(states.length===1){
      positions.set(states[index],{x:centerX,y:centerY});
    }else{
      const angle=-Math.PI/2+index*Math.PI*2/states.length;
      positions.set(states[index],{x:centerX+Math.cos(angle)*radius,y:centerY+Math.sin(angle)*Math.min(radius,height/2-100)});
    }
  }
  const groups=new Map();
  for(const edge of transitions){
    const from=String(edge.fromState);
    const to=String(edge.toState);
    const key=JSON.stringify([from,to]);
    if(!groups.has(key))groups.set(key,{from:from,to:to,events:[]});
    groups.get(key).events.push(String(edge.event));
  }
  const latest=item.traces&&item.traces.length?item.traces[item.traces.length-1]:null;
  const edgeLayer=svgElement("g");
  const labelLayer=svgElement("g");
  for(const edgeGroup of groups.values()){
    const from=positions.get(edgeGroup.from);
    const to=positions.get(edgeGroup.to);
    if(!from||!to)continue;
    const processing=edgeGroup.from===String(item.currentState)&&edgeGroup.events.includes(String(item.processingEvent||""));
    const recent=latest&&edgeGroup.from===String(latest.fromState)&&edgeGroup.to===String(latest.toState)&&edgeGroup.events.includes(String(latest.event));
    const available=edgeGroup.from===String(item.currentState);
    const status=processing?"processing":recent?(latest.success?"recent":"failed"):available?"available":"";
    let pathData;
    let labelX;
    let labelY;
    if(edgeGroup.from===edgeGroup.to){
      pathData="M "+(from.x-25)+" "+(from.y-31)+" C "+(from.x-82)+" "+(from.y-112)+" "+(from.x+82)+" "+(from.y-112)+" "+(from.x+25)+" "+(from.y-31);
      labelX=from.x;
      labelY=from.y-100;
    }else{
      const dx=to.x-from.x;
      const dy=to.y-from.y;
      const length=Math.max(1,Math.hypot(dx,dy));
      const startX=from.x+dx/length*58;
      const startY=from.y+dy/length*38;
      const endX=to.x-dx/length*62;
      const endY=to.y-dy/length*40;
      const reverse=groups.has(JSON.stringify([edgeGroup.to,edgeGroup.from]));
      const curve=reverse?34:0;
      const controlX=(startX+endX)/2-dy/length*curve;
      const controlY=(startY+endY)/2+dx/length*curve;
      pathData=curve?"M "+startX+" "+startY+" Q "+controlX+" "+controlY+" "+endX+" "+endY:"M "+startX+" "+startY+" L "+endX+" "+endY;
      labelX=curve?(startX+2*controlX+endX)/4:(startX+endX)/2-dy/length*12;
      labelY=curve?(startY+2*controlY+endY)/4:(startY+endY)/2+dx/length*12;
    }
    const marker=status==="processing"?"arrow-processing":status==="recent"?"arrow-recent":status==="failed"?"arrow-failed":status==="available"?"arrow-available":"arrow-default";
    const path=svgElement("path",{d:pathData,class:"graph-edge"+(status?" "+status:""),"marker-end":"url(#"+marker+")"});
    path.appendChild(svgElement("title",{},edgeGroup.from+" -- "+edgeGroup.events.join(", ")+" --> "+edgeGroup.to));
    edgeLayer.appendChild(path);
    labelLayer.appendChild(svgElement("text",{x:labelX,y:labelY,class:"graph-edge-label"+(status?" "+status:""),"text-anchor":"middle"},edgeGroup.events.join(" / ")));
  }
  svg.append(edgeLayer,labelLayer);
  const nodeLayer=svgElement("g");
  for(const state of states){
    const position=positions.get(state);
    const nodeWidth=Math.max(104,Math.min(190,state.length*9+38));
    const current=state===String(item.currentState);
    const previous=state===String(item.previousState)&&!current;
    const node=svgElement("g",{class:"graph-node"+(current?" current":previous?" previous":""),tabindex:"0","aria-label":"状态 "+state});
    node.appendChild(svgElement("rect",{x:position.x-nodeWidth/2,y:position.y-31,width:nodeWidth,height:62,rx:13}));
    node.appendChild(svgElement("text",{x:position.x,y:position.y-(current?7:0)},state));
    if(current)node.appendChild(svgElement("text",{x:position.x,y:position.y+17,class:"node-status"},"CURRENT"));
    node.addEventListener("pointerenter",event=>showNodeTooltip(event,state,item));
    node.addEventListener("pointermove",moveNodeTooltip);
    node.addEventListener("pointerleave",hideNodeTooltip);
    node.addEventListener("focus",event=>{
      const bounds=node.getBoundingClientRect();
      showNodeTooltip({clientX:bounds.right,clientY:bounds.top},state,item);
    });
    node.addEventListener("blur",hideNodeTooltip);
    nodeLayer.appendChild(node);
  }
  svg.appendChild(nodeLayer);
  const stage=document.createElement("div");
  stage.className="graph-stage";
  const fitWidth=Math.max(420,host.clientWidth*.82);
  const fitHeight=Math.max(360,host.clientHeight*.82);
  const scale=Math.max(.65,Math.min(1.45,fitWidth/width,fitHeight/height));
  stage.style.width=Math.round(width*scale)+"px";
  stage.style.height=Math.round(height*scale)+"px";
  stage.appendChild(svg);
  host.appendChild(stage);
  setupGraphPan(host,stage,item.id||"fsm");
}
function renderAvailableTransitions(item){const list=byId("availableTransitions");list.replaceChildren();const available=item.availableTransitions||(item.transitions||[]).filter(edge=>edge.fromState===item.currentState);text("availableCount",available.length+" events");if(!available.length){const empty=document.createElement("div");empty.className="available-empty";empty.textContent="当前状态没有可接收的事件";list.appendChild(empty);return}for(const edge of available){const row=document.createElement("div");const processing=String(edge.event)===String(item.processingEvent||"");row.className="available-transition"+(processing?" processing":"");const event=document.createElement("span");event.className="event-token";event.textContent=edge.event;const arrow=document.createElement("span");arrow.textContent="→";arrow.style.color="var(--muted)";const target=document.createElement("span");target.className="target-state";target.textContent=edge.toState;row.append(event,arrow,target);list.appendChild(row)}}
function inlineSVGStyles(source,target){const properties=["fill","stroke","stroke-width","stroke-dasharray","stroke-dashoffset","stroke-linecap","stroke-linejoin","opacity","filter","font-family","font-size","font-weight","letter-spacing","text-anchor","dominant-baseline","paint-order"];const sourceNodes=[source,...source.querySelectorAll("*")];const targetNodes=[target,...target.querySelectorAll("*")];for(let index=0;index<sourceNodes.length;index++){const computed=getComputedStyle(sourceNodes[index]);const declarations=[];for(const property of properties){const value=computed.getPropertyValue(property);if(value)declarations.push(property+":"+value)}if(declarations.length)targetNodes[index].setAttribute("style",declarations.join(";"))}}
function exportStateGraph(){const button=byId("exportGraph");const source=byId("stateGraph").querySelector("svg");if(!source||button.disabled)return;button.disabled=true;try{const clone=source.cloneNode(true);inlineSVGStyles(source,clone);const viewBox=source.viewBox.baseVal;const width=Math.max(1,Math.ceil(viewBox.width));const height=Math.max(1,Math.ceil(viewBox.height));clone.setAttribute("xmlns","http://www.w3.org/2000/svg");clone.setAttribute("width",String(width));clone.setAttribute("height",String(height));const backgroundColor=getComputedStyle(document.documentElement).getPropertyValue("--graph-bg").trim()||"#ffffff";clone.insertBefore(svgElement("rect",{x:viewBox.x,y:viewBox.y,width:width,height:height,fill:backgroundColor}),clone.firstChild);const serialized=new XMLSerializer().serializeToString(clone);const sourceURL=URL.createObjectURL(new Blob([serialized],{type:"image/svg+xml;charset=utf-8"}));const image=new Image();image.onload=()=>{URL.revokeObjectURL(sourceURL);const scale=2;const canvas=document.createElement("canvas");canvas.width=width*scale;canvas.height=height*scale;const context=canvas.getContext("2d");context.scale(scale,scale);context.fillStyle=backgroundColor;context.fillRect(0,0,width,height);context.drawImage(image,0,0,width,height);canvas.toBlob(blob=>{button.disabled=false;if(!blob)return;const current=app.fsms.find(item=>item.id===app.selected);const raw=current?String(current.displayName||current.name||"fsm"):"fsm";const safe=raw.replace(/[^a-zA-Z0-9._-]+/g,"-")||"fsm";const downloadURL=URL.createObjectURL(blob);const link=document.createElement("a");link.href=downloadURL;link.download=safe+"-state-graph.png";document.body.appendChild(link);link.click();link.remove();setTimeout(()=>URL.revokeObjectURL(downloadURL),1000)},"image/png")};image.onerror=()=>{URL.revokeObjectURL(sourceURL);button.disabled=false;console.error("failed to render FSM graph image")};image.src=sourceURL}catch(error){button.disabled=false;console.error(error)}}
function cell(row,value,className){const element=document.createElement("td");element.textContent=value;if(className)element.className=className;row.appendChild(element)}
function renderTraces(traces){const body=byId("traceBody");body.replaceChildren();const ordered=[...traces].reverse();for(const trace of ordered){const row=document.createElement("tr");cell(row,trace.beginTime?new Date(trace.beginTime/1000000).toLocaleTimeString():"-");cell(row,trace.event);cell(row,trace.fromState+" → "+trace.finalState);cell(row,trace.success?"success":"failed",trace.success?"ok":"fail");cell(row,formatNanos(trace.duration));cell(row,trace.error||"-",trace.error?"trace-error":"");body.appendChild(row)}}
function initializeFloatingPanels(){
  for(const button of document.querySelectorAll(".panel-toggle")){
    const panel=byId(button.dataset.panel);
    const stored=readPreference("fsm-panel:"+panel.id);
    const collapsed=stored?stored==="collapsed":true;
    panel.classList.toggle("collapsed",collapsed);
    button.setAttribute("aria-expanded",String(!collapsed));
    button.onclick=()=>{
      const next=!panel.classList.contains("collapsed");
      panel.classList.toggle("collapsed",next);
      button.setAttribute("aria-expanded",String(!next));
      writePreference("fsm-panel:"+panel.id,next?"collapsed":"open");
    };
  }
}
function applySidebarCollapsed(collapsed,persist){
  document.body.classList.toggle("sidebar-collapsed",collapsed);
  const button=byId("sidebarToggle");
  const action=collapsed?"展开":"折叠";
  button.setAttribute("aria-label",action+"左侧栏");
  button.title=action+"左侧栏";
  button.setAttribute("aria-expanded",String(!collapsed));
  if(persist)writePreference("fsm-sidebar-collapsed",collapsed?"collapsed":"open");
  window.setTimeout(()=>{
    const current=app.fsms.find(item=>item.id===app.selected);
    if(current)renderStateGraph(current);
  },190);
}
function initializeSidebarToggle(){
  const collapsed=readPreference("fsm-sidebar-collapsed")==="collapsed";
  applySidebarCollapsed(collapsed,false);
  byId("sidebarToggle").onclick=()=>applySidebarCollapsed(!document.body.classList.contains("sidebar-collapsed"),true);
}
function applyOverlayTransparency(value,persist){
  const transparency=Math.max(0,Math.min(100,Number(value)||0));
  const opacity=100-transparency;
  const root=document.documentElement;
  root.style.setProperty("--panel-alpha",opacity+"%");
  root.style.setProperty("--panel-detail-alpha",Math.round(opacity*.64)+"%");
  root.style.setProperty("--panel-blur",(opacity*.12).toFixed(1)+"px");
  root.style.setProperty("--panel-shadow",(opacity*.0008).toFixed(3));
  byId("opacitySlider").value=String(transparency);
  text("opacityValue",Math.round(transparency)+"%");
  const button=byId("opacityToggle");
  button.title="浮层透明度 "+Math.round(transparency)+"%";
  button.setAttribute("aria-label",button.title);
  if(persist)writePreference("fsm-overlay-transparency",String(transparency));
}
function initializeOverlayOpacity(){
  const stored=readPreference("fsm-overlay-transparency");
  applyOverlayTransparency(stored===""?85:Number(stored),false);
  const control=byId("opacityControl");
  const toggle=byId("opacityToggle");
  const close=()=>{
    control.classList.remove("open");
    toggle.setAttribute("aria-expanded","false");
  };
  toggle.onclick=event=>{
    event.stopPropagation();
    const open=!control.classList.contains("open");
    control.classList.toggle("open",open);
    toggle.setAttribute("aria-expanded",String(open));
  };
  byId("opacitySlider").addEventListener("input",event=>applyOverlayTransparency(event.target.value,true));
  document.addEventListener("pointerdown",event=>{if(!control.contains(event.target))close()});
  document.addEventListener("keydown",event=>{if(event.key==="Escape")close()});
}
async function loadDiscoveries(){
  try{
    const response=await fetch("/api/discoveries",{cache:"no-store"});
    if(!response.ok)throw new Error(response.statusText);
    app.discoveries=await response.json();
    renderDiscoveries();
  }catch(error){
    text("discoveryFormStatus",error.message||String(error));
    byId("discoveryFormStatus").classList.add("error");
  }
}
function discoveryStatus(message,isError){
  const status=byId("discoveryFormStatus");
  status.textContent=message||"";
  status.classList.toggle("error",Boolean(isError));
}
function discoveryAction(label,className,handler){
  const button=document.createElement("button");
  button.type="button";
  button.className="discovery-action"+(className?" "+className:"");
  button.textContent=label;
  button.onclick=handler;
  return button;
}
function renderDiscoveries(){
  const list=byId("discoveryList");
  list.replaceChildren();
  if(!app.discoveries.length){
    const empty=document.createElement("div");
    empty.className="discovery-empty";
    empty.textContent="尚未配置服务发现";
    list.appendChild(empty);
    return;
  }
  for(const item of app.discoveries){
    const card=document.createElement("article");
    card.className="discovery-item";
    const head=document.createElement("div");
    head.className="discovery-item-head";
    const dot=document.createElement("span");
    dot.className="discovery-status-dot"+(item.lastError?" error":"");
    const name=document.createElement("strong");
    name.textContent=item.name||item.id;
    const count=document.createElement("span");
    count.className="discovery-item-count";
    count.textContent=String(item.registered||0)+" / "+String(item.discovered||0)+" registered";
    head.append(dot,name,count);
    const meta=document.createElement("div");
    meta.className="discovery-item-meta";
    meta.textContent=(item.endpoint||"-")+" · every "+item.refreshIntervalSeconds+"s · pull "+item.pullIntervalSeconds+"s"+(item.pathPrefix?" · /"+item.pathPrefix:"");
    card.append(head,meta);
    if(item.lastError){
      const error=document.createElement("div");
      error.className="discovery-item-error";
      error.textContent=item.lastError;
      card.appendChild(error);
    }
    const actions=document.createElement("div");
    actions.className="discovery-actions";
    actions.append(
      discoveryAction("立即发现","",()=>refreshDiscovery(item.id)),
      discoveryAction("删除","danger",()=>deleteDiscovery(item.id))
    );
    card.appendChild(actions);
    list.appendChild(card);
  }
}
async function refreshDiscovery(id){
  discoveryStatus("正在发现服务…",false);
  try{
    const response=await fetch("/api/discoveries/"+encodeURIComponent(id)+"/refresh",{method:"POST"});
    if(!response.ok){const data=await response.json().catch(()=>({}));throw new Error(data.error||response.statusText)}
    discoveryStatus("发现完成",false);
    await Promise.all([loadDiscoveries(),loadFSMs()]);
  }catch(error){discoveryStatus(error.message||String(error),true);await loadDiscoveries()}
}
async function deleteDiscovery(id){
  if(!window.confirm("删除服务发现配置 "+id+"？由它注册的 FSM 也会被移除。"))return;
  try{
    const response=await fetch("/api/discoveries/"+encodeURIComponent(id),{method:"DELETE"});
    if(!response.ok){const data=await response.json().catch(()=>({}));throw new Error(data.error||response.statusText)}
    discoveryStatus("已删除 "+id,false);
    await Promise.all([loadDiscoveries(),loadFSMs()]);
  }catch(error){discoveryStatus(error.message||String(error),true)}
}
function closeDiscoveryModal(){const modal=byId("discoveryModal");if(!modal.classList.contains("open"))return;modal.classList.remove("open");byId("discoveryToggle").setAttribute("aria-expanded","false");byId("discoveryToggle").focus()}
function initializeDiscovery(){
  const modal=byId("discoveryModal");
  byId("discoveryToggle").onclick=()=>{modal.classList.add("open");byId("discoveryToggle").setAttribute("aria-expanded","true");loadDiscoveries();window.requestAnimationFrame(()=>byId("discoveryId").focus())};
  byId("discoveryClose").onclick=closeDiscoveryModal;
  modal.addEventListener("pointerdown",event=>{if(event.target===modal)closeDiscoveryModal()});
  document.addEventListener("keydown",event=>{if(event.key==="Escape")closeDiscoveryModal()});
  byId("discoveryForm").onsubmit=async event=>{
    event.preventDefault();
    const submit=byId("discoverySubmit");
    submit.disabled=true;
    discoveryStatus("正在创建并执行首次发现…",false);
    const payload={
      id:byId("discoveryId").value.trim(),
      name:byId("discoveryName").value.trim(),
      endpoint:byId("discoveryEndpoint").value.trim(),
      pathPrefix:byId("discoveryPathPrefix").value.trim(),
      refreshIntervalSeconds:Number(byId("discoveryRefresh").value),
      pullIntervalSeconds:Number(byId("discoveryPull").value)
    };
    try{
      const response=await fetch("/api/discoveries",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(payload)});
      if(!response.ok){const data=await response.json().catch(()=>({}));throw new Error(data.error||response.statusText)}
      discoveryStatus("发现配置已启动",false);
      byId("discoveryId").value="";
      byId("discoveryName").value="";
      await Promise.all([loadDiscoveries(),loadFSMs()]);
    }catch(error){discoveryStatus(error.message||String(error),true)}
    finally{submit.disabled=false}
  };
}

initializeTheme();
initializeResizableColumns();
initializeSidebarToggle();
initializeOverlayOpacity();
initializeDiscovery();
initializeFloatingPanels();
byId("fsmSearch").addEventListener("input",event=>{app.query=event.target.value;renderList()});
byId("exportGraph").disabled=true;
byId("exportGraph").onclick=exportStateGraph;
const stream=new EventSource("/api/events");
stream.onopen=()=>{document.body.dataset.connection="online"};
stream.onerror=()=>{document.body.dataset.connection="offline"};
stream.onmessage=()=>{loadFSMs();loadDiscoveries()};
loadFSMs();
loadDiscoveries();
</script>
</body>
</html>`
