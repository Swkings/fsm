import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api } from './api.js'
import StateGraph from './StateGraph.jsx'

function stored(key, fallback) {
  try { return localStorage.getItem(key) ?? fallback } catch { return fallback }
}
function remember(key, value) {
  try { localStorage.setItem(key, String(value)) } catch { /* storage is optional */ }
}
function formatTime(value) { return value ? new Date(value).toLocaleString() : '-' }
function formatNanos(value) {
  if (!value) return '0 ms'
  if (value < 1000) return `${value} ns`
  if (value < 1000000) return `${(value / 1000).toFixed(2)} μs`
  return `${(value / 1000000).toFixed(2)} ms`
}
function machineName(item) { return item.displayName || item.name || item.id }
function compareMachines(left, right) { return String(machineName(left)).localeCompare(String(machineName(right)), 'zh-CN') }

function buildFolderTree(items) {
  const root = { name: '', path: '', folders: new Map(), items: [] }
  items.forEach((item) => {
    let node = root
    String(item.path || '').split('/').map((part) => part.trim()).filter(Boolean).forEach((part) => {
      const path = node.path ? `${node.path}/${part}` : part
      if (!node.folders.has(part)) node.folders.set(part, { name: part, path, folders: new Map(), items: [] })
      node = node.folders.get(part)
    })
    node.items.push(item)
  })
  return root
}
function folderSize(node) { return node.items.length + [...node.folders.values()].reduce((total, child) => total + folderSize(child), 0) }

function MachineButton({ item, selected, onSelect }) {
  return (
    <button className={`fsm-button${selected ? ' active' : ''}`} type="button" onClick={() => onSelect(item.id)}>
      <span className="fsm-name">{machineName(item)}</span>
      <span className="state-mini">{item.currentState || '—'}</span>
      <span className="fsm-meta"><i className={`mode-dot ${item.mode || 'push'}`} />{item.mode || 'push'} · {item.name || item.id}</span>
    </button>
  )
}
function Folder({ node, selected, onSelect }) {
  const children = [...node.folders.values()].sort((a, b) => a.name.localeCompare(b.name, 'zh-CN'))
  return (
    <details className="folder-node" data-path={node.path} defaultOpen={stored(`fsm-folder:${node.path}`, 'open') !== 'closed'} onToggle={(event) => remember(`fsm-folder:${node.path}`, event.currentTarget.open ? 'open' : 'closed')}>
      <summary><span className="folder-chevron">›</span><span className="folder-icon">□</span><span>{node.name}</span><span className="folder-count">{folderSize(node)}</span></summary>
      <div className="folder-content">
        {[...node.items].sort(compareMachines).map((item) => <MachineButton key={item.id} item={item} selected={selected === item.id} onSelect={onSelect} />)}
        {children.map((child) => <Folder key={child.path} node={child} selected={selected} onSelect={onSelect} />)}
      </div>
    </details>
  )
}

function Sidebar({ fsms, selected, onSelect, collapsed, onToggle }) {
  const [query, setQuery] = useState('')
  const visible = useMemo(() => {
    const needle = query.trim().toLocaleLowerCase()
    if (!needle) return fsms
    return fsms.filter((item) => [item.displayName, item.name, item.id, item.path, item.currentState, item.mode].some((value) => String(value || '').toLocaleLowerCase().includes(needle)))
  }, [fsms, query])
  const tree = useMemo(() => buildFolderTree(visible), [visible])
  return (
    <aside>
      <div className="aside-heading">
        <span className="aside-title">Machines</span>
        <span className="count-pill">{fsms.length} registered</span>
        <button id="sidebarToggle" className="sidebar-toggle" type="button" aria-label={`${collapsed ? '展开' : '折叠'}左侧栏`} title={`${collapsed ? '展开' : '折叠'}左侧栏`} aria-expanded={!collapsed} onClick={onToggle}>
          <svg className="sidebar-collapse-icon" viewBox="0 0 24 24"><path d="M7 4v16M16 8l-4 4 4 4" /></svg>
          <svg className="sidebar-expand-icon" viewBox="0 0 24 24"><path d="M7 4v16M12 8l4 4-4 4" /></svg>
        </button>
      </div>
      <div className="tree-toolbar"><input className="tree-search" type="search" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索 FSM、状态或路径" aria-label="搜索 FSM" /></div>
      <div className="fsm-list">
        {[...tree.items].sort(compareMachines).map((item) => <MachineButton key={item.id} item={item} selected={selected === item.id} onSelect={onSelect} />)}
        {[...tree.folders.values()].sort((a, b) => a.name.localeCompare(b.name, 'zh-CN')).map((folder) => <Folder key={folder.path} node={folder} selected={selected} onSelect={onSelect} />)}
        {!visible.length && <div className="available-empty">{fsms.length ? '没有匹配的 FSM' : '尚未注册 FSM'}</div>}
      </div>
    </aside>
  )
}

function PanelClose({ label, onClick }) {
  return <button className="panel-close" type="button" aria-label={`关闭 ${label}`} title={`关闭 ${label}`} onClick={onClick}><svg viewBox="0 0 24 24"><path d="M6 6l12 12M18 6L6 18" /></svg></button>
}
function ActiveEventPanel({ item, open, onClose }) {
  if (!open) return null
  const available = item.availableTransitions || (item.transitions || []).filter((edge) => edge.fromState === item.currentState)
  return (
    <section className="monitor-panel available-panel" aria-labelledby="activeEventTitle">
      <div className="monitor-panel-head"><strong id="activeEventTitle">Active Event</strong><span className="panel-summary">{available.length} events</span><PanelClose label="Active Event" onClick={onClose} /></div>
      <div className="panel-body"><div className="available-list">
        {!available.length && <div className="available-empty">当前状态没有可接收的事件</div>}
        {available.map((edge, index) => <div key={`${edge.event}-${edge.toState}-${index}`} className={`available-transition${String(edge.event) === String(item.processingEvent || '') ? ' processing' : ''}`}><span className="event-token">{edge.event}</span><span style={{ color: 'var(--muted)' }}>→</span><span className="target-state">{edge.toState}</span></div>)}
      </div></div>
    </section>
  )
}
function PayloadPanel({ item, open, onClose }) {
  if (!open) return null
  const summary = `${item.currentState || 'empty'} · ${item.mode || 'push'}`
  return (
    <section className="monitor-panel payload-panel" aria-labelledby="payloadTitle">
      <div className="monitor-panel-head"><strong id="payloadTitle">Payload</strong><span className="panel-summary">{summary}</span><PanelClose label="Payload" onClick={onClose} /></div>
      <div className="panel-body">
        {item.payloadError && <div className="payload-error" style={{ display: 'block' }}>{String(item.payloadError)}</div>}
        <pre>{JSON.stringify(item.payload === undefined ? null : item.payload, null, 2)}</pre>
        <div className="meta">
          <div><label>Transition ID</label><span>{item.transitionId}</span></div>
          <div><label>Updated</label><span>{formatTime(item.updatedAt)}</span></div>
          <div><label>Previous State</label><span>{item.previousState || '-'}</span></div>
          <div><label>Previous Event</label><span>{item.previousEvent || '-'}</span></div>
        </div>
      </div>
    </section>
  )
}
function TracePanel({ traces }) {
  const [expanded, setExpanded] = useState(false)
  const ordered = useMemo(() => [...(traces || [])].reverse(), [traces])
  const latest = ordered[0]
  return (
    <section className={`monitor-panel trace-panel${expanded ? ' expanded' : ''}`} aria-labelledby="traceTitle">
      <div className="trace-status-bar">
        <div className="trace-title"><strong id="traceTitle">Trace</strong><span className="panel-summary">{ordered.length} traces</span></div>
        <div className="trace-latest">
          {!latest ? <span className="trace-latest-empty">暂无 Trace</span> : <><span>{latest.beginTime ? new Date(latest.beginTime / 1000000).toLocaleTimeString() : '-'}</span><span className="trace-latest-event">{latest.event || '-'}</span><span>{latest.fromState || '-'} → {latest.finalState || '-'}</span><span className={`trace-latest-result ${latest.success ? 'ok' : 'fail'}`} title={latest.error || ''}>{latest.success ? 'success' : 'failed'}</span><span>{formatNanos(latest.duration)}</span></>}
        </div>
        <button className="trace-toggle" type="button" aria-label={expanded ? '折叠 Trace 状态栏' : '展开完整 Trace'} title={expanded ? '折叠 Trace 状态栏' : '展开完整 Trace'} aria-expanded={expanded} onClick={() => setExpanded((value) => !value)}>
          <svg className="trace-expand-icon" viewBox="0 0 24 24"><path d="M6 15l6-6 6 6" /></svg><svg className="trace-collapse-icon" viewBox="0 0 24 24"><path d="M6 9l6 6 6-6" /></svg>
        </button>
      </div>
      <div className="trace-history"><table><thead><tr><th>时间</th><th>事件</th><th>状态流转</th><th>结果</th><th>耗时</th><th>错误</th></tr></thead><tbody>{ordered.map((trace, index) => <tr key={trace.transitionId || index}><td>{trace.beginTime ? new Date(trace.beginTime / 1000000).toLocaleTimeString() : '-'}</td><td>{trace.event}</td><td>{trace.fromState} → {trace.finalState}</td><td className={trace.success ? 'ok' : 'fail'}>{trace.success ? 'success' : 'failed'}</td><td>{formatNanos(trace.duration)}</td><td className={trace.error ? 'trace-error' : ''}>{trace.error || '-'}</td></tr>)}</tbody></table></div>
    </section>
  )
}

function DiscoverySettings({ open, onClose, discoveries, reloadDiscoveries, reloadFSMs }) {
  const [status, setStatus] = useState({ message: '', error: false })
  const [submitting, setSubmitting] = useState(false)
  const idRef = useRef(null)
  useEffect(() => {
    if (!open) return undefined
    reloadDiscoveries()
    requestAnimationFrame(() => idRef.current?.focus())
    const escape = (event) => { if (event.key === 'Escape') onClose() }
    document.addEventListener('keydown', escape)
    return () => document.removeEventListener('keydown', escape)
  }, [open, onClose, reloadDiscoveries])
  if (!open) return null

  const refresh = async (id) => {
    setStatus({ message: '正在发现服务…', error: false })
    try { await api.refreshDiscovery(id); setStatus({ message: '发现完成', error: false }); await Promise.all([reloadDiscoveries(), reloadFSMs()]) }
    catch (error) { setStatus({ message: error.message, error: true }); await reloadDiscoveries() }
  }
  const remove = async (id) => {
    if (!window.confirm(`删除服务发现配置 ${id}？由它注册的 FSM 也会被移除。`)) return
    try { await api.deleteDiscovery(id); setStatus({ message: `已删除 ${id}`, error: false }); await Promise.all([reloadDiscoveries(), reloadFSMs()]) }
    catch (error) { setStatus({ message: error.message, error: true }) }
  }
  const submit = async (event) => {
    event.preventDefault()
    setSubmitting(true)
    setStatus({ message: '正在创建并执行首次发现…', error: false })
    const data = new FormData(event.currentTarget)
    try {
      await api.createDiscovery({
        id: String(data.get('id')).trim(), name: String(data.get('name')).trim(), endpoint: String(data.get('endpoint')).trim(), pathPrefix: String(data.get('pathPrefix')).trim(),
        refreshIntervalSeconds: Number(data.get('refreshIntervalSeconds')), pullIntervalSeconds: Number(data.get('pullIntervalSeconds')),
      })
      event.currentTarget.reset()
      setStatus({ message: '发现配置已启动', error: false })
      await Promise.all([reloadDiscoveries(), reloadFSMs()])
    } catch (error) { setStatus({ message: error.message, error: true }) }
    finally { setSubmitting(false) }
  }
  return (
    <div className="settings-modal open" role="dialog" aria-modal="true" aria-labelledby="settingsTitle" onPointerDown={(event) => { if (event.target === event.currentTarget) onClose() }}>
      <section className="settings-shell">
        <header className="settings-head"><div className="settings-head-title"><strong id="settingsTitle">设置</strong><span>管理 Observatory 的连接与运行选项</span></div><button className="icon-button settings-close" type="button" aria-label="关闭设置" title="关闭" onClick={onClose}>×</button></header>
        <div className="settings-layout">
          <nav className="settings-nav" aria-label="设置分类"><span className="settings-nav-label">连接</span><button className="settings-nav-item active" type="button" aria-current="page"><span className="settings-nav-icon"><svg viewBox="0 0 24 24"><circle cx="6" cy="12" r="2"/><circle cx="18" cy="6" r="2"/><circle cx="18" cy="18" r="2"/><path d="M8 11l8-4M8 13l8 4"/></svg></span><strong>服务发现</strong><small>Client Server 与 Pull 配置</small></button></nav>
          <main className="settings-content">
            <div className="settings-section-head"><h2>服务发现</h2><p>由独立 Server 发现业务进程的可视化 Client Server，并持续拉取其中注册的 FSM。</p></div>
            <section className="settings-block"><div className="settings-block-head"><div><strong>添加发现源</strong><span>可填写注册中心发现接口，也可直接填写单个 Client Server 地址。</span></div><span className="settings-block-badge">HTTP</span></div>
              <form className="discovery-form" onSubmit={submit}>
                <div className="discovery-field"><label htmlFor="discoveryId">配置 ID</label><input ref={idRef} id="discoveryId" name="id" required placeholder="production-services" /></div>
                <div className="discovery-field"><label htmlFor="discoveryName">显示名称</label><input id="discoveryName" name="name" placeholder="Production Services" /></div>
                <div className="discovery-field wide"><label htmlFor="discoveryEndpoint">发现接口 / Client Server 地址</label><input id="discoveryEndpoint" name="endpoint" type="url" required placeholder="http://service-a:5005" /></div>
                <div className="discovery-field wide"><label htmlFor="discoveryPathPrefix">目录前缀</label><input id="discoveryPathPrefix" name="pathPrefix" placeholder="production/discovered" /></div>
                <div className="discovery-field"><label htmlFor="discoveryRefresh">发现周期（秒）</label><input id="discoveryRefresh" name="refreshIntervalSeconds" type="number" min="0.1" step="0.1" defaultValue="30" /></div>
                <div className="discovery-field"><label htmlFor="discoveryPull">Pull 周期（秒）</label><input id="discoveryPull" name="pullIntervalSeconds" type="number" min="0.1" step="0.1" defaultValue="5" /></div>
                <div className="discovery-form-footer"><button className="discovery-submit" type="submit" disabled={submitting}>添加发现配置</button><span className={`discovery-form-status${status.error ? ' error' : ''}`} role="status">{status.message}</span></div>
              </form>
            </section>
            <section className="settings-block"><div className="settings-block-head"><div><strong>已配置的发现源</strong><span>查看同步状态，或手动触发一次服务发现。</span></div></div>
              <div className="discovery-list">{!discoveries.length ? <div className="discovery-empty">尚未配置服务发现</div> : discoveries.map((item) => <article key={item.id} className="discovery-item"><div className="discovery-item-head"><span className={`discovery-status-dot${item.lastError ? ' error' : ''}`} /><strong>{item.name || item.id}</strong><span className="discovery-item-count">{item.registered || 0} / {item.discovered || 0} registered</span></div><div className="discovery-item-meta">{item.endpoint || '-'} · every {item.refreshIntervalSeconds}s · pull {item.pullIntervalSeconds}s{item.pathPrefix ? ` · /${item.pathPrefix}` : ''}</div>{item.lastError && <div className="discovery-item-error">{item.lastError}</div>}<div className="discovery-actions"><button className="discovery-action" type="button" onClick={() => refresh(item.id)}>立即发现</button><button className="discovery-action danger" type="button" onClick={() => remove(item.id)}>删除</button></div></article>)}</div>
            </section>
          </main>
        </div>
      </section>
    </div>
  )
}

function App() {
  const [fsms, setFSMs] = useState([])
  const [discoveries, setDiscoveries] = useState([])
  const [selected, setSelected] = useState('')
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [theme, setTheme] = useState(() => stored('fsm-visualization-theme', matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark'))
  const [transparency, setTransparency] = useState(() => Number(stored('fsm-overlay-transparency', '85')))
  const [opacityOpen, setOpacityOpen] = useState(false)
  const [sidebarCollapsed, setSidebarCollapsed] = useState(() => stored('fsm-sidebar-collapsed', 'open') === 'collapsed')
  const [activeOpen, setActiveOpen] = useState(() => stored('fsm-monitor-panel:availablePanel', 'open') !== 'closed')
  const [payloadOpen, setPayloadOpen] = useState(() => stored('fsm-monitor-panel:payloadPanel', 'open') !== 'closed')
  const layoutRef = useRef(null)
  const opacityRef = useRef(null)

  const loadFSMs = useCallback(async () => { try { setFSMs(await api.fsms()) } catch (error) { console.error(error) } }, [])
  const loadDiscoveries = useCallback(async () => { try { setDiscoveries(await api.discoveries()) } catch (error) { console.error(error) } }, [])
  const closeSettings = useCallback(() => setSettingsOpen(false), [])
  useEffect(() => {
    loadFSMs(); loadDiscoveries()
    const stream = new EventSource('/api/events')
    stream.onopen = () => { document.body.dataset.connection = 'online' }
    stream.onerror = () => { document.body.dataset.connection = 'offline' }
    stream.onmessage = () => { loadFSMs(); loadDiscoveries() }
    return () => stream.close()
  }, [loadFSMs, loadDiscoveries])
  useEffect(() => { if (!fsms.some((item) => item.id === selected)) setSelected(fsms[0]?.id || '') }, [fsms, selected])
  useEffect(() => { document.documentElement.dataset.theme = theme; remember('fsm-visualization-theme', theme) }, [theme])
  useEffect(() => {
    document.body.classList.toggle('sidebar-collapsed', sidebarCollapsed)
    remember('fsm-sidebar-collapsed', sidebarCollapsed ? 'collapsed' : 'open')
  }, [sidebarCollapsed])
  useEffect(() => {
    const value = Math.max(0, Math.min(100, Number(transparency) || 0))
    const opacity = 100 - value
    const root = document.documentElement
    root.style.setProperty('--panel-alpha', `${opacity}%`)
    root.style.setProperty('--panel-detail-alpha', `${Math.round(opacity * 0.64)}%`)
    root.style.setProperty('--panel-blur', `${(opacity * 0.12).toFixed(1)}px`)
    root.style.setProperty('--panel-shadow', (opacity * 0.0008).toFixed(3))
    remember('fsm-overlay-transparency', value)
  }, [transparency])
  useEffect(() => {
    if (!opacityOpen) return undefined
    const close = (event) => { if (!opacityRef.current?.contains(event.target)) setOpacityOpen(false) }
    document.addEventListener('pointerdown', close)
    return () => document.removeEventListener('pointerdown', close)
  }, [opacityOpen])

  const setPanel = (panel, value) => {
    remember(`fsm-monitor-panel:${panel}`, value ? 'open' : 'closed')
    if (panel === 'availablePanel') setActiveOpen(value); else setPayloadOpen(value)
  }
  const current = fsms.find((item) => item.id === selected)
  const resizeStart = (event) => {
    if (matchMedia('(max-width: 1100px)').matches || sidebarCollapsed) return
    event.preventDefault()
    const layout = layoutRef.current
    const move = (moveEvent) => {
      const bounds = layout.getBoundingClientRect()
      const width = Math.round(Math.max(190, Math.min(bounds.width - 366, moveEvent.clientX - bounds.left)))
      document.documentElement.style.setProperty('--sidebar-width', `${width}px`)
      remember('fsm-sidebar-width', width)
    }
    const stop = () => { window.removeEventListener('pointermove', move); window.removeEventListener('pointerup', stop); window.removeEventListener('pointercancel', stop) }
    window.addEventListener('pointermove', move); window.addEventListener('pointerup', stop); window.addEventListener('pointercancel', stop)
  }
  useEffect(() => {
    const width = Number(stored('fsm-sidebar-width', '0'))
    if (width > 0) document.documentElement.style.setProperty('--sidebar-width', `${width}px`)
  }, [])

  return (
    <>
      <div ref={layoutRef} className="layout">
        <Sidebar fsms={fsms} selected={selected} onSelect={setSelected} collapsed={sidebarCollapsed} onToggle={() => setSidebarCollapsed((value) => !value)} />
        <div id="sidebarResize" className="resize-handle" role="separator" aria-label="调整状态机列表宽度" aria-orientation="vertical" tabIndex="0" onPointerDown={resizeStart} onKeyDown={(event) => { if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return; event.preventDefault(); const currentWidth = layoutRef.current?.querySelector("aside")?.getBoundingClientRect().width || 288; const width = Math.max(190, currentWidth + (event.key === "ArrowLeft" ? -16 : 16)); document.documentElement.style.setProperty("--sidebar-width", `${width}px`); remember("fsm-sidebar-width", width) }} onDoubleClick={() => { document.documentElement.style.removeProperty('--sidebar-width'); remember('fsm-sidebar-width', '') }} />
        <main className="workspace">
          <header className="topbar">
            <div ref={opacityRef} className={`opacity-control${opacityOpen ? ' open' : ''}`}>
              <button className="icon-button" type="button" aria-label={`浮层透明度 ${Math.round(transparency)}%`} title={`浮层透明度 ${Math.round(transparency)}%`} aria-expanded={opacityOpen} onClick={() => setOpacityOpen((value) => !value)}><svg viewBox="0 0 24 24"><path d="M12 3s5 6 5 10a5 5 0 0 1-10 0c0-4 5-10 5-10z"/><path d="M9 14.5c.7 1.3 1.7 2 3 2"/></svg></button>
              <div className="opacity-popover" role="dialog" aria-label="浮层透明度"><div className="opacity-popover-head"><strong>浮层透明度</strong><span className="opacity-value">{Math.round(transparency)}%</span></div><input className="opacity-slider" type="range" min="0" max="100" step="1" value={transparency} onChange={(event) => setTransparency(Number(event.target.value))} aria-label="浮层透明度"/><div className="opacity-scale"><span>不透明</span><span>全透明</span></div></div>
            </div>
            <button id="themeToggle" className="icon-button" type="button" aria-label={`切换为${theme === 'dark' ? '浅色' : '深色'}主题`} title={`切换为${theme === 'dark' ? '浅色' : '深色'}主题`} onClick={() => setTheme((value) => value === 'light' ? 'dark' : 'light')}><svg className="theme-sun" viewBox="0 0 24 24"><circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.93 4.93l1.42 1.42M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.42-1.42M17.66 6.34l1.41-1.41"/></svg><svg className="theme-moon" viewBox="0 0 24 24"><path d="M20.5 14.2A8.5 8.5 0 0 1 9.8 3.5a8.5 8.5 0 1 0 10.7 10.7z"/></svg></button>
            <button id="discoveryToggle" className="icon-button" type="button" aria-label="打开设置" title="设置" aria-expanded={settingsOpen} onClick={() => setSettingsOpen(true)}><svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .34 1.88l.06.06-1.86 1.86-.06-.06A1.7 1.7 0 0 0 16 18.4a1.7 1.7 0 0 0-1 .6 1.7 1.7 0 0 0-.4 1.1V20h-2.6v-.09A1.7 1.7 0 0 0 10.9 18.4a1.7 1.7 0 0 0-1.88.34l-.06.06-1.86-1.86.06-.06A1.7 1.7 0 0 0 7.5 15a1.7 1.7 0 0 0-1.5-1H5.9v-2.6H6a1.7 1.7 0 0 0 1.5-1 1.7 1.7 0 0 0-.34-1.88l-.06-.06L8.96 6.6l.06.06A1.7 1.7 0 0 0 10.9 7a1.7 1.7 0 0 0 1-1.5v-.1h2.6v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.88-.34l.06-.06 1.86 1.86-.06.06A1.7 1.7 0 0 0 18.9 10.4a1.7 1.7 0 0 0 1.5 1h.1V14h-.1a1.7 1.7 0 0 0-1 .6z"/></svg></button>
          </header>
          {!current ? <div className="empty"><div><strong>等待 FSM 注册</strong><span>注册完成后将在此显示状态图。</span></div></div> : <div className="content visible"><section className="graph-surface" aria-label="FSM 状态图画布">
            <StateGraph item={current} />
            {current.collectionError && <div className="collection-error" style={{ display: 'block' }}>{current.collectionError}</div>}
            <nav className="detail-nav" aria-label="FSM 详情面板">
              <button className={`detail-tab${activeOpen ? ' active' : ''}`} type="button" aria-controls="availablePanel" aria-expanded={activeOpen} onClick={() => setPanel('availablePanel', !activeOpen)}><strong>Active Event</strong><span className="panel-summary">{(current.availableTransitions || []).length}</span></button>
              <button className={`detail-tab${payloadOpen ? ' active' : ''}`} type="button" aria-controls="payloadPanel" aria-expanded={payloadOpen} onClick={() => setPanel('payloadPanel', !payloadOpen)}><strong>Payload</strong><span className="panel-summary">{current.currentState || 'empty'} · {current.mode || 'push'}</span></button>
            </nav>
            <ActiveEventPanel item={current} open={activeOpen} onClose={() => setPanel('availablePanel', false)} />
            <PayloadPanel item={current} open={payloadOpen} onClose={() => setPanel('payloadPanel', false)} />
            <TracePanel traces={current.traces} />
          </section></div>}
        </main>
      </div>
      <DiscoverySettings open={settingsOpen} onClose={closeSettings} discoveries={discoveries} reloadDiscoveries={loadDiscoveries} reloadFSMs={loadFSMs} />
    </>
  )
}

export default App
