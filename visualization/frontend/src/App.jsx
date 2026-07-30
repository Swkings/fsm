import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
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

function Sidebar({ fsms, selected, onSelect, collapsed, onToggle, title = 'Machines', countLabel, searchPlaceholder = '搜索 FSM、状态或路径', emptyLabel = '尚未注册 FSM' }) {
  const [query, setQuery] = useState('')
  const visible = useMemo(() => {
    const needle = query.trim().toLocaleLowerCase()
    if (!needle) return fsms
    return fsms.filter((item) => [item.displayName, item.name, item.id, item.path, item.currentState, item.mode].some((value) => String(value || '').toLocaleLowerCase().includes(needle)))
  }, [fsms, query])
  const tree = useMemo(() => buildFolderTree(visible), [visible])
  return (
    <aside className="left-sidebar">
      <div className="aside-heading">
        <span className="aside-title">{title}</span>
        <span className="count-pill">{countLabel || `${fsms.length} registered`}</span>
        <button id="sidebarToggle" className="sidebar-toggle" type="button" aria-label={`${collapsed ? '展开' : '折叠'}左侧栏`} title={`${collapsed ? '展开' : '折叠'}左侧栏`} aria-expanded={!collapsed} onClick={onToggle}>
          <svg className="left-sidebar-open-icon" viewBox="0 0 16 16"><rect x="1.5" y="2.5" width="13" height="11" rx="1.5"/><rect className="sidebar-pane-fill" x="2.5" y="3.5" width="3.5" height="9" rx=".75"/><path d="M6.5 3v10"/></svg>
          <svg className="left-sidebar-closed-icon" viewBox="0 0 16 16"><rect x="1.5" y="2.5" width="13" height="11" rx="1.5"/><path d="M6.5 3v10"/></svg>
        </button>
      </div>
      <div className="tree-toolbar"><input className="tree-search" type="search" value={query} onChange={(event) => setQuery(event.target.value)} placeholder={searchPlaceholder} aria-label="搜索 FSM" /></div>
      <div className="fsm-list">
        {[...tree.items].sort(compareMachines).map((item) => <MachineButton key={item.id} item={item} selected={selected === item.id} onSelect={onSelect} />)}
        {[...tree.folders.values()].sort((a, b) => a.name.localeCompare(b.name, 'zh-CN')).map((folder) => <Folder key={folder.path} node={folder} selected={selected} onSelect={onSelect} />)}
        {!visible.length && <div className="available-empty">{fsms.length ? '没有匹配的 FSM' : emptyLabel}</div>}
      </div>
    </aside>
  )
}

function ActiveEventPanel({ item, open, onToggle }) {
  const available = [...(item.availableTransitions || (item.transitions || []).filter((edge) => edge.fromState === item.currentState))]
    .sort((left, right) => String(left.event).localeCompare(String(right.event)) || String(left.toState).localeCompare(String(right.toState)))
  const currentState = String(item.currentState || 'empty')
  return (
    <section className={"monitor-panel available-panel" + (open ? "" : " collapsed")} aria-labelledby="activeEventTitle">
      <div className="monitor-panel-head"><strong id="activeEventTitle">Active Event</strong><span className="panel-summary">{available.length} events</span><button className="active-event-toggle" type="button" aria-label={open ? "折叠 Active Event" : "展开 Active Event"} title={open ? "折叠 Active Event" : "展开 Active Event"} aria-expanded={open} aria-controls="activeEventBody" onClick={() => onToggle(!open)}><svg className="active-event-collapse-icon" viewBox="0 0 24 24"><path d="M6 15l6-6 6 6" /></svg><svg className="active-event-expand-icon" viewBox="0 0 24 24"><path d="M6 9l6 6 6-6" /></svg></button></div>
      <div id="activeEventBody" className="panel-body"><div className="active-event-tree" role="tree" aria-label={`${currentState} 可接收事件`}>
        <div className="active-tree-root" role="treeitem" aria-expanded={available.length > 0}>
          <span className="active-tree-root-dot" aria-hidden="true" />
          <strong>{currentState}</strong>
          <span className="active-tree-current">CURRENT</span>
        </div>
        {!available.length && <div className="available-empty">当前状态没有可接收的事件</div>}
        {!!available.length && <div className="active-tree-branches" role="group">
          {available.map((edge, index) => {
            const processing = String(edge.event) === String(item.processingEvent || '')
            return (
              <div key={`${edge.event}-${edge.toState}-${index}`} className={`active-tree-branch${processing ? ' processing' : ''}`} role="treeitem">
                <span className="active-tree-prefix" aria-hidden="true">{index === available.length - 1 ? '└──' : '├──'}</span>
                <span className="event-token">{edge.event}</span>
                <span className="active-tree-arrow" aria-hidden="true">→</span>
                <span className="target-state">{edge.toState}</span>
                {processing && <span className="active-tree-processing">PROCESSING</span>}
              </div>
            )
          })}
        </div>}
      </div></div>
    </section>
  )
}
function PayloadContent({ item }) {
  return (
    <div id="payloadPanel" className="payload-content" aria-label="Payload">
      <div className="meta payload-meta">
        <div><label>Transition ID</label><span>{item.transitionId}</span></div>
        <div><label>Updated</label><span>{formatTime(item.updatedAt)}</span></div>
        <div><label>Previous State</label><span>{item.previousState || "-"}</span></div>
        <div><label>Previous Event</label><span>{item.previousEvent || "-"}</span></div>
      </div>
      {item.payloadError && <div className="payload-error" style={{ display: "block" }}>{String(item.payloadError)}</div>}
      <pre className="payload-code"><code>{JSON.stringify(item.payload === undefined ? null : item.payload, null, 2)}</code></pre>
    </div>
  )
}
function RightSidebar({ item, payloadOpen, onPayloadToggle }) {
  const defaultWidth = window.innerWidth <= 520 ? 220 : window.innerWidth <= 820 ? 280 : window.innerWidth <= 1100 ? 320 : 400
  const minimumWidth = 220
  const [width, setWidth] = useState(() => {
    const value = Number(stored('fsm-right-sidebar-width', String(defaultWidth)))
    return Math.round(Math.max(minimumWidth, Math.min(window.innerWidth * .6, Number.isFinite(value) ? value : defaultWidth)))
  })
  const [resizing, setResizing] = useState(false)
  const resizeRef = useRef(null)
  const maximumWidth = () => Math.max(minimumWidth, Math.min(720, window.innerWidth * .6))
  const setClampedWidth = (value, persist = false) => {
    const next = Math.round(Math.max(minimumWidth, Math.min(maximumWidth(), value)))
    setWidth(next)
    if (resizeRef.current) resizeRef.current.width = next
    if (persist) remember('fsm-right-sidebar-width', next)
    return next
  }
  const startResize = (event) => {
    if (event.button !== 0) return
    event.preventDefault()
    event.currentTarget.setPointerCapture(event.pointerId)
    resizeRef.current = { pointerId: event.pointerId, startX: event.clientX, startWidth: width, width }
    setResizing(true)
  }
  const moveResize = (event) => {
    const resize = resizeRef.current
    if (!resize || resize.pointerId !== event.pointerId) return
    setClampedWidth(resize.startWidth + resize.startX - event.clientX)
  }
  const stopResize = (event) => {
    const resize = resizeRef.current
    if (!resize || (event.pointerId !== undefined && resize.pointerId !== event.pointerId)) return
    resizeRef.current = null
    setResizing(false)
    remember('fsm-right-sidebar-width', resize.width)
  }
  const resizeWithKeyboard = (event) => {
    if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight' && event.key !== 'Home') return
    event.preventDefault()
    setClampedWidth(event.key === 'Home' ? defaultWidth : width + (event.key === 'ArrowLeft' ? 20 : -20), true)
  }
  useEffect(() => { document.documentElement.style.setProperty('--right-sidebar-width', `${width}px`) }, [width])
  useEffect(() => {
    const fit = () => setClampedWidth(width)
    window.addEventListener('resize', fit)
    return () => window.removeEventListener('resize', fit)
  }, [width])
  useEffect(() => {
    document.body.classList.toggle('right-sidebar-resizing', resizing)
    return () => document.body.classList.remove('right-sidebar-resizing')
  }, [resizing])
  return (
    <aside className={`right-sidebar${resizing ? ' resizing' : ''}`} aria-label="FSM Inspector">
      <div className="right-sidebar-resize" role="separator" aria-label="调整右侧边栏宽度" aria-orientation="vertical" aria-valuemin={minimumWidth} aria-valuemax={Math.round(maximumWidth())} aria-valuenow={width} tabIndex="0" title="拖动调整宽度 · 双击恢复默认" onPointerDown={startResize} onPointerMove={moveResize} onPointerUp={stopResize} onPointerCancel={stopResize} onLostPointerCapture={stopResize} onKeyDown={resizeWithKeyboard} onDoubleClick={() => setClampedWidth(defaultWidth, true)} />
      <nav className="right-sidebar-nav" aria-label="FSM Inspector 面板">
        <button className={["inspector-tab", payloadOpen ? "active" : ""].filter(Boolean).join(" ")} type="button" disabled={!item} aria-controls="payloadPanel" aria-expanded={payloadOpen} onClick={() => onPayloadToggle(!payloadOpen)}>
          <strong>Payload</strong>
        </button>
      </nav>
      <div className="right-sidebar-content">
        {!item ? <div className="right-sidebar-empty">选择 FSM 后查看 Payload</div> : payloadOpen ? <PayloadContent item={item} /> : <div className="right-sidebar-empty">点击 Payload 查看当前数据</div>}
      </div>
    </aside>
  )
}

function traceTimeMillis(trace) {
  const nanos = Number(trace?.beginTime)
  return Number.isFinite(nanos) && nanos > 0 ? nanos / 1000000 : 0
}
function formatTraceTime(trace) {
  const millis = traceTimeMillis(trace)
  return millis ? new Date(millis).toLocaleString() : '-'
}
function TraceContextMenu({ menu, onView }) {
  if (!menu) return null
  return createPortal(
    <div
      className="trace-context-menu"
      style={{ top: menu.top, left: menu.left }}
      role="menu"
      aria-label="Trace 操作"
      onContextMenu={(event) => event.preventDefault()}
    >
      <button type="button" role="menuitem" onClick={() => onView(menu.trace)}>
        <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M2.5 12s3.4-6 9.5-6 9.5 6 9.5 6-3.4 6-9.5 6-9.5-6-9.5-6Z"/><circle cx="12" cy="12" r="2.7"/></svg>
        <span><strong>查看 FSM 数据</strong><small>进入和退出快照</small></span>
      </button>
    </div>,
    document.body,
  )
}
function TraceMetadataDialog({ trace, onClose }) {
  if (!trace) return null
  const entry = trace.entryMetadata
  const exit = trace.exitMetadata
  return createPortal(
    <div className="trace-metadata-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose() }}>
      <section className="trace-metadata-dialog" role="dialog" aria-modal="true" aria-labelledby="traceMetadataTitle">
        <header>
          <div><strong id="traceMetadataTitle">FSM 数据快照</strong><span>{trace.event || '-'} · {trace.fromState || '-'} → {trace.finalState || '-'}</span></div>
          <button type="button" aria-label="关闭 FSM 数据" title="关闭" onClick={onClose}><svg viewBox="0 0 24 24"><path d="M6 6l12 12M18 6 6 18"/></svg></button>
        </header>
      <div className="trace-metadata-grid">
        <section><strong>进入 FSM</strong><pre>{JSON.stringify(entry === undefined ? null : entry, null, 2)}</pre></section>
        <section><strong>退出 FSM</strong><pre>{JSON.stringify(exit === undefined ? null : exit, null, 2)}</pre></section>
      </div>
      </section>
    </div>,
    document.body,
  )
}

function TracePanel({ traces, selectedId = '', onSelect }) {
  const defaultHeight = 440
  const minimumHeight = 180
  const [expanded, setExpanded] = useState(false)
  const [query, setQuery] = useState('')
  const [startTime, setStartTime] = useState('')
  const [endTime, setEndTime] = useState('')
  const [sortBy, setSortBy] = useState('newest')
  const [contextMenu, setContextMenu] = useState(null)
  const [metadataTrace, setMetadataTrace] = useState(null)
  const [traceHeight, setTraceHeight] = useState(() => {
    const value = Number(stored('fsm-trace-height', String(defaultHeight)))
    return Number.isFinite(value) && value >= minimumHeight ? value : defaultHeight
  })
  const [resizing, setResizing] = useState(false)
  const panelRef = useRef(null)
  const resizeRef = useRef(null)
  const allTraces = useMemo(() => [...(traces || [])], [traces])
  const latest = useMemo(() => allTraces.reduce((current, trace) => !current || traceTimeMillis(trace) > traceTimeMillis(current) ? trace : current, null), [allTraces])
  const visible = useMemo(() => {
    const needle = query.trim().toLocaleLowerCase()
    const startMillis = startTime ? new Date(startTime).getTime() : null
    const endMillis = endTime ? new Date(endTime).getTime() : null
    const filtered = allTraces.filter((trace) => {
      const millis = traceTimeMillis(trace)
      if (startMillis !== null && Number.isFinite(startMillis) && millis < startMillis) return false
      if (endMillis !== null && Number.isFinite(endMillis) && millis > endMillis) return false
      if (!needle) return true
      const date = millis ? new Date(millis) : null
      return [
        trace.event,
        trace.fromState,
        trace.toState,
        trace.finalState,
        trace.success ? 'success' : 'failed',
        trace.error,
        trace.transitionId,
        trace.beginTime,
        trace.endTime,
        date?.toLocaleString(),
        date?.toLocaleTimeString(),
        date?.toISOString(),
      ].some((value) => String(value ?? '').toLocaleLowerCase().includes(needle))
    })
    return filtered.sort((left, right) => {
      if (sortBy === 'oldest') return traceTimeMillis(left) - traceTimeMillis(right)
      if (sortBy === 'duration-desc') return Number(right.duration || 0) - Number(left.duration || 0)
      if (sortBy === 'duration-asc') return Number(left.duration || 0) - Number(right.duration || 0)
      return traceTimeMillis(right) - traceTimeMillis(left)
    })
  }, [allTraces, endTime, query, sortBy, startTime])
  useEffect(() => {
    if (!contextMenu && !metadataTrace) return undefined
    const onKeyDown = (event) => {
      if (event.key !== 'Escape') return
      setContextMenu(null)
      setMetadataTrace(null)
    }
    const onPointerDown = (event) => {
      if (contextMenu && !event.target.closest('.trace-context-menu')) setContextMenu(null)
    }
    document.addEventListener('keydown', onKeyDown)
    document.addEventListener('pointerdown', onPointerDown)
    return () => {
      document.removeEventListener('keydown', onKeyDown)
      document.removeEventListener('pointerdown', onPointerDown)
    }
  }, [contextMenu, metadataTrace])
  const filtered = query.trim() || startTime || endTime
  const clearFilters = () => { setQuery(''); setStartTime(''); setEndTime('') }
  const openTraceMenu = (event, trace) => {
    event.preventDefault()
    const rect = event.currentTarget.getBoundingClientRect()
    const pointerX = event.clientX || rect.left + 24
    const pointerY = event.clientY || rect.top + rect.height / 2
    setMetadataTrace(null)
    setContextMenu({
      trace,
      left: Math.max(8, Math.min(pointerX, window.innerWidth - 220)),
      top: Math.max(8, Math.min(pointerY, window.innerHeight - 62)),
    })
  }
  const viewMetadata = (trace) => {
    setContextMenu(null)
    setMetadataTrace(trace)
  }
  const maximumHeight = () => Math.max(minimumHeight, (panelRef.current?.parentElement?.getBoundingClientRect().height || window.innerHeight) - 28)
  const setClampedHeight = (value, persist = false) => {
    const height = Math.round(Math.max(minimumHeight, Math.min(maximumHeight(), value)))
    setTraceHeight(height)
    if (persist) remember('fsm-trace-height', height)
    return height
  }
  const startTraceResize = (event) => {
    if (!expanded || event.button !== 0) return
    event.preventDefault()
    event.currentTarget.setPointerCapture(event.pointerId)
    resizeRef.current = { pointerId: event.pointerId, startY: event.clientY, startHeight: panelRef.current?.getBoundingClientRect().height || traceHeight, height: traceHeight }
    setResizing(true)
  }
  const moveTraceResize = (event) => {
    const resize = resizeRef.current
    if (!resize || resize.pointerId !== event.pointerId) return
    resize.height = setClampedHeight(resize.startHeight + resize.startY - event.clientY)
  }
  const stopTraceResize = (event) => {
    const resize = resizeRef.current
    if (!resize || (event.pointerId !== undefined && resize.pointerId !== event.pointerId)) return
    resizeRef.current = null
    setResizing(false)
    remember('fsm-trace-height', resize.height)
  }
  const resizeTraceWithKeyboard = (event) => {
    if (event.key !== 'ArrowUp' && event.key !== 'ArrowDown' && event.key !== 'Home') return
    event.preventDefault()
    setClampedHeight(event.key === 'Home' ? defaultHeight : traceHeight + (event.key === 'ArrowUp' ? 24 : -24), true)
  }
  useEffect(() => {
    document.body.classList.toggle('trace-resizing', resizing)
    return () => document.body.classList.remove('trace-resizing')
  }, [resizing])
  return (
    <section ref={panelRef} className={`monitor-panel trace-panel${expanded ? ' expanded' : ''}${resizing ? ' resizing' : ''}`} style={expanded ? { height: `min(${traceHeight}px, calc(100% - 28px))` } : undefined} aria-labelledby="traceTitle">
      <div className="trace-resize-handle" role="separator" aria-label="调整 Trace 面板高度" aria-orientation="horizontal" aria-valuemin={minimumHeight} aria-valuenow={Math.round(traceHeight)} tabIndex={expanded ? 0 : -1} title="拖动调整高度 · 双击恢复默认" onPointerDown={startTraceResize} onPointerMove={moveTraceResize} onPointerUp={stopTraceResize} onPointerCancel={stopTraceResize} onLostPointerCapture={stopTraceResize} onKeyDown={resizeTraceWithKeyboard} onDoubleClick={() => setClampedHeight(defaultHeight, true)} />
      <div className="trace-status-bar">
        <div className="trace-title"><strong id="traceTitle">Trace</strong><span className="panel-summary">{allTraces.length} traces</span></div>
        <div className="trace-latest">
          {!latest ? <span className="trace-latest-empty">暂无 Trace</span> : <><span>{latest.beginTime ? new Date(latest.beginTime / 1000000).toLocaleTimeString() : '-'}</span><span className="trace-latest-event">{latest.event || '-'}</span><span>{latest.fromState || '-'} → {latest.finalState || '-'}</span><span className={`trace-latest-result ${latest.success ? 'ok' : 'fail'}`} title={latest.error || ''}>{latest.success ? 'success' : 'failed'}</span><span>{formatNanos(latest.duration)}</span></>}
        </div>
        <button className="trace-toggle" type="button" aria-label={expanded ? '折叠 Trace 状态栏' : '展开完整 Trace'} title={expanded ? '折叠 Trace 状态栏' : '展开完整 Trace'} aria-expanded={expanded} onClick={() => setExpanded((value) => !value)}>
          <svg className="trace-expand-icon" viewBox="0 0 24 24"><path d="M6 15l6-6 6 6" /></svg><svg className="trace-collapse-icon" viewBox="0 0 24 24"><path d="M6 9l6 6 6-6" /></svg>
        </button>
      </div>
      <div className="trace-history">
        <div className="trace-toolbar">
          <input className="trace-search" type="search" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索事件、状态、结果、错误或时间" aria-label="搜索 Trace" />
          <label><span>开始</span><input type="datetime-local" step="1" value={startTime} max={endTime || undefined} onChange={(event) => setStartTime(event.target.value)} /></label>
          <label><span>结束</span><input type="datetime-local" step="1" value={endTime} min={startTime || undefined} onChange={(event) => setEndTime(event.target.value)} /></label>
          <label><span>排序</span><select value={sortBy} onChange={(event) => setSortBy(event.target.value)}><option value="newest">最新优先</option><option value="oldest">最早优先</option><option value="duration-desc">耗时从长到短</option><option value="duration-asc">耗时从短到长</option></select></label>
          <span className="trace-filter-count">{visible.length} / {allTraces.length}</span>
          <button className="trace-clear" type="button" disabled={!filtered} onClick={clearFilters}>清空</button>
        </div>
        <div className="trace-table-wrap">
          <table><thead><tr><th>时间</th><th>事件</th><th>状态流转</th><th>结果</th><th>耗时</th><th>错误</th></tr></thead><tbody>
            {visible.map((trace, index) => {
              const traceId = trace.replayRecordId || trace.transitionId || String(index)
              return <tr className={`trace-row${selectedId === traceId ? ' selected' : ''}${onSelect ? ' selectable' : ''}`} key={traceId} tabIndex={0} title="右键查看 FSM 数据" aria-current={selectedId === traceId ? 'true' : undefined} onClick={() => onSelect?.(trace)} onKeyDown={(event) => { if (onSelect && (event.key === 'Enter' || event.key === ' ')) { event.preventDefault(); onSelect(trace) } }} onContextMenu={(event) => openTraceMenu(event, trace)}><td>{formatTraceTime(trace)}</td><td>{trace.event}</td><td>{trace.fromState} → {trace.finalState}</td><td className={trace.success ? "ok" : "fail"}>{trace.success ? "success" : "failed"}</td><td>{formatNanos(trace.duration)}</td><td className={trace.error ? "trace-error" : ""}>{trace.error || "-"}</td></tr>
            })}
            {!visible.length && <tr><td className="trace-empty" colSpan="6">没有匹配的 Trace</td></tr>}
          </tbody></table>
        </div>
      </div>
      <TraceContextMenu menu={contextMenu} onView={viewMetadata} />
      <TraceMetadataDialog trace={metadataTrace} onClose={() => setMetadataTrace(null)} />
    </section>
  )
}

function ReplaySidebar({ sources, machinesBySource, selectedSource, selectedFSM, onSelect, onRefresh, collapsed, onToggle }) {
  const [query, setQuery] = useState('')
  const visibleSources = useMemo(() => {
    const needle = query.trim().toLocaleLowerCase()
    if (!needle) return sources.map((source) => ({ source, machines: machinesBySource[source.id] || [] }))
    return sources.map((source) => {
      const machines = machinesBySource[source.id] || []
      const sourceMatch = [source.id, source.name, source.driver, source.location].some((value) => String(value || '').toLocaleLowerCase().includes(needle))
      const visibleMachines = sourceMatch ? machines : machines.filter((item) => [item.id, item.name, item.displayName, item.path, item.currentState].some((value) => String(value || '').toLocaleLowerCase().includes(needle)))
      return { source, machines: visibleMachines }
    }).filter((item) => item.machines.length || [item.source.id, item.source.name, item.source.driver, item.source.location].some((value) => String(value || '').toLocaleLowerCase().includes(needle)))
  }, [machinesBySource, query, sources])
  const machineCount = Object.values(machinesBySource).reduce((total, machines) => total + machines.length, 0)
  return (
    <aside className="left-sidebar replay-sidebar">
      <div className="aside-heading">
        <span className="aside-title">Replay Sources</span><span className="count-pill">{sources.length} / {machineCount}</span>
        <button className="sidebar-tool" type="button" aria-label="刷新 Replay 数据源" title="刷新" onClick={onRefresh}><svg viewBox="0 0 24 24"><path d="M20 7v5h-5M4 17v-5h5"/><path d="M6.1 8.2A7 7 0 0 1 18.8 9M17.9 15.8A7 7 0 0 1 5.2 15"/></svg></button>
        <button className="sidebar-toggle" type="button" aria-label={`${collapsed ? '展开' : '折叠'}左侧栏`} title={`${collapsed ? '展开' : '折叠'}左侧栏`} aria-expanded={!collapsed} onClick={onToggle}><svg className="left-sidebar-open-icon" viewBox="0 0 16 16"><rect x="1.5" y="2.5" width="13" height="11" rx="1.5"/><rect className="sidebar-pane-fill" x="2.5" y="3.5" width="3.5" height="9" rx=".75"/><path d="M6.5 3v10"/></svg><svg className="left-sidebar-closed-icon" viewBox="0 0 16 16"><rect x="1.5" y="2.5" width="13" height="11" rx="1.5"/><path d="M6.5 3v10"/></svg></button>
      </div>
      <div className="tree-toolbar"><input className="tree-search" type="search" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索数据源、FSM 或状态" aria-label="搜索 Replay 数据源和 FSM" /></div>
      <div className="fsm-list">
        {visibleSources.map(({ source, machines }) => <details className="folder-node replay-source-node" key={source.id} defaultOpen={stored(`replay-source:${source.id}`, 'open') !== 'closed'} onToggle={(event) => remember(`replay-source:${source.id}`, event.currentTarget.open ? 'open' : 'closed')}>
          <summary title={source.location || source.name}><span className="folder-chevron">›</span><span className={`source-status${source.lastError ? ' error' : ''}`} /><span>{source.name || source.id}</span><span className="source-driver">{source.driver}</span><span className="folder-count">{machines.length}</span></summary>
          <div className="folder-content">
            {machines.map((item) => <MachineButton key={item.id} item={item} selected={selectedSource === source.id && selectedFSM === item.id} onSelect={(id) => onSelect(source.id, id)} />)}
            {!machines.length && <div className="source-empty">未发现 FSM</div>}
          </div>
        </details>)}
        {!visibleSources.length && <div className="available-empty">没有匹配的数据源或 FSM</div>}
      </div>
    </aside>
  )
}

function ReplayPage({ sidebarCollapsed, onSidebarToggle, payloadOpen, onPayloadToggle }) {
  const [sources, setSources] = useState([])
  const [source, setSource] = useState('')
  const [recordsBySource, setRecordsBySource] = useState({})
  const [selectedFSM, setSelectedFSM] = useState('')
  const [selected, setSelected] = useState(null)
  const [playing, setPlaying] = useState(false)

  const load = useCallback(async () => {
    const values = await api.replaySources()
    setSources(values)
    const entries = await Promise.all(values.map(async (item) => {
      try { return [item.id, await api.replayRecords(item.id)] }
      catch (error) { console.error(error); return [item.id, []] }
    }))
    setRecordsBySource(Object.fromEntries(entries))
  }, [])
  useEffect(() => { setPlaying(false); setSelected(null); load().catch(console.error) }, [load])

  const machinesBySource = useMemo(() => Object.fromEntries(sources.map((sourceItem) => {
    const grouped = new Map()
    ;(recordsBySource[sourceItem.id] || []).forEach((record) => {
      if (!record.fsmId) return
      const current = grouped.get(record.fsmId)
      if (current) { current.recordCount++; return }
      grouped.set(record.fsmId, { id: record.fsmId, name: record.fsmId, displayName: record.displayName || record.fsmId, path: record.path || '', currentState: record.state || '', mode: 'replay', recordCount: 1 })
    })
    return [sourceItem.id, [...grouped.values()].sort(compareMachines)]
  })), [recordsBySource, sources])
  useEffect(() => {
    if ((machinesBySource[source] || []).some((item) => item.id === selectedFSM)) return
    const nextSource = sources.find((item) => (machinesBySource[item.id] || []).length) || sources[0]
    setSource(nextSource?.id || '')
    setSelectedFSM(nextSource ? machinesBySource[nextSource.id]?.[0]?.id || '' : '')
  }, [machinesBySource, selectedFSM, source, sources])
  const records = useMemo(() => (recordsBySource[source] || []).filter((item) => item.fsmId === selectedFSM), [recordsBySource, selectedFSM, source])
  const traces = useMemo(() => records.map((record) => ({
    ...(record.trace || {}),
    replayRecordId: record.id,
    transitionId: record.trace?.transitionId || record.transitionId || record.id,
    event: record.trace?.event || record.event || '',
    finalState: record.trace?.finalState || record.state || '',
    success: record.trace?.transitionId || record.trace?.event ? record.trace.success : record.success,
    beginTime: record.trace?.beginTime || new Date(record.recordedAt).getTime() * 1000000,
  })), [records])
  useEffect(() => {
    let cancelled = false
    if (!source || !selectedFSM || !records.length) { setSelected(null); return () => { cancelled = true } }
    if (selected?.sourceId === source && selected?.fsmId === selectedFSM && records.some((item) => item.id === selected.id)) return () => { cancelled = true }
    api.replayRecord(source, records[0].id).then((record) => { if (!cancelled) setSelected(record) }).catch(console.error)
    return () => { cancelled = true }
  }, [records, selected?.fsmId, selected?.id, selected?.sourceId, selectedFSM, source])
  useEffect(() => {
    if (!playing || records.length < 2) return undefined
    const ordered = [...records].reverse()
    let index = Math.max(0, ordered.findIndex((item) => item.id === selected?.id))
    const timer = setInterval(async () => {
      index = (index + 1) % ordered.length
      setSelected(await api.replayRecord(source, ordered[index].id))
    }, 1000)
    return () => clearInterval(timer)
  }, [playing, records, selected?.id, source])
  const chooseRecord = async (record) => {
    setPlaying(false)
    setSelected(await api.replayRecord(source, record.id))
  }
  const removeSelected = async () => {
    if (!selected || !window.confirm('删除当前回放记录？')) return
    setPlaying(false)
    await api.deleteReplayRecord(source, selected.id)
    setSelected(null)
    await load()
  }
  const selectMachine = (sourceID, fsmID) => { setPlaying(false); setSelected(null); setSource(sourceID); setSelectedFSM(fsmID) }
  const selectTrace = (trace) => {
    const record = records.find((item) => item.id === trace.replayRecordId)
    if (record) chooseRecord(record).catch(console.error)
  }

  return <>
    <ReplaySidebar sources={sources} machinesBySource={machinesBySource} selectedSource={source} selectedFSM={selectedFSM} onSelect={selectMachine} onRefresh={() => load().catch(console.error)} collapsed={sidebarCollapsed} onToggle={onSidebarToggle} />
    <main className="replay-page">
      <section className="replay-stage">{selected ? <><div className="replay-controls"><button type="button" disabled={records.length < 2} onClick={() => setPlaying((value) => !value)}>{playing ? '暂停' : '播放'}</button><span>{selected.snapshot.previousState || '-'} -- {selected.snapshot.previousEvent || '-'} → {selected.snapshot.currentState}</span><button type="button" onClick={() => load().catch(console.error)}>刷新</button><button className="replay-delete" type="button" onClick={removeSelected}>删除</button></div><div className="replay-graph"><StateGraph item={selected.snapshot}/></div></> : <div className="replay-empty">请选择数据源中的 FSM</div>}</section>
      <TracePanel traces={traces} selectedId={selected?.id || ''} onSelect={selectTrace} />
    </main>
    <RightSidebar item={selected?.snapshot} payloadOpen={payloadOpen} onPayloadToggle={onPayloadToggle} />
  </>
}

function ReplaySourceSettings() {
  const formRef = useRef(null)
  const [sources, setSources] = useState([])
  const [driver, setDriver] = useState('jsonl')
  const [jsonlMode, setJSONLMode] = useState('local')
  const [directory, setDirectory] = useState('')
  const [filename, setFilename] = useState('')
  const [directoryBrowser, setDirectoryBrowser] = useState(null)
  const [remoteBrowser, setRemoteBrowser] = useState(null)
  const [sshBrowsing, setSSHBrowsing] = useState(false)
  const [status, setStatus] = useState({ message: '', error: false })
  const [submitting, setSubmitting] = useState(false)
  const reload = useCallback(() => api.replaySources().then(setSources), [])
  useEffect(() => { reload().catch(console.error) }, [reload])
  const browseDirectory = async (path = directory) => {
    try { setDirectoryBrowser(await api.replayDirectories(path)); setStatus({ message: '', error: false }) }
    catch (error) { setStatus({ message: error.message, error: true }) }
  }
  const openChildDirectory = (name) => {
    const separator = directoryBrowser.path.includes('\\') ? '\\' : '/'
    browseDirectory(`${directoryBrowser.path.replace(/[\\/]$/, '')}${separator}${name}`)
  }
  const sshConfig = () => {
    const data = new FormData(formRef.current)
    return {
      address: String(data.get('sshAddress') || '').trim(), username: String(data.get('sshUsername') || '').trim(), password: String(data.get('sshPassword') || ''),
      privateKeyFile: String(data.get('privateKeyFile') || '').trim(), privateKeyPassphrase: String(data.get('privateKeyPassphrase') || ''), knownHostsFile: String(data.get('knownHostsFile') || '').trim(),
      insecureIgnoreHostKey: data.get('insecureIgnoreHostKey') === 'on', timeoutSeconds: Number(data.get('timeoutSeconds') || 10),
    }
  }
  const browseRemoteDirectory = async (path = directory) => {
    const ssh = sshConfig()
    if (!ssh.address || !ssh.username) {
      setStatus({ message: '请先填写 SSH 地址和用户名', error: true })
      return
    }
    if (!ssh.password && !ssh.privateKeyFile) {
      setStatus({ message: '请先填写 SSH 密码或私钥文件', error: true })
      return
    }
    setSSHBrowsing(true)
    setStatus({ message: '正在连接 SSH…', error: false })
    try {
      const view = await api.replaySSHDirectories({ directory: String(path || '.').trim() || '.', ssh })
      setRemoteBrowser(view)
      setDirectory(view.path)
      setStatus({ message: 'SSH 已连接，请选择远程目录或 JSONL 文件', error: false })
    } catch (error) { setStatus({ message: error.message, error: true }) }
    finally { setSSHBrowsing(false) }
  }
  const chooseRemoteEntry = (entry) => {
    if (entry.directory) {
      browseRemoteDirectory(entry.path)
      return
    }
    setDirectory(remoteBrowser.path)
    setFilename(entry.name)
    setRemoteBrowser(null)
    setStatus({ message: `已选择 ${entry.path}`, error: false })
  }
  const submit = async (event) => {
    event.preventDefault()
    setSubmitting(true)
    setStatus({ message: '正在连接数据源…', error: false })
    const form = event.currentTarget
    const data = new FormData(form)
    try {
      await api.createReplaySource({
        id: String(data.get('id') || '').trim(), name: String(data.get('name') || '').trim(), driver,
        jsonl: driver === 'jsonl' ? {
          directory: directory.trim(), filename: filename.trim(),
          ssh: jsonlMode === 'ssh' ? sshConfig() : undefined,
        } : undefined,
        mysql: driver === 'mysql' ? { dsn: String(data.get('dsn') || '').trim(), table: String(data.get('table') || '').trim() } : undefined,
        redis: driver === 'redis' ? { address: String(data.get('address') || '').trim(), password: String(data.get('password') || ''), db: Number(data.get('db') || 0), keyPrefix: String(data.get('keyPrefix') || '').trim() } : undefined,
      })
      form.reset(); setDirectory(''); setFilename(''); setDirectoryBrowser(null); setRemoteBrowser(null)
      setStatus({ message: '数据源已连接', error: false }); await reload()
    } catch (error) { setStatus({ message: error.message, error: true }) }
    finally { setSubmitting(false) }
  }
  const remove = async (item) => {
    if (!window.confirm(`删除数据源 ${item.name || item.id}？`)) return
    try { await api.deleteReplaySource(item.id); setStatus({ message: '数据源已删除', error: false }); await reload() }
    catch (error) { setStatus({ message: error.message, error: true }) }
  }
  return <>
    <div className="settings-section-head"><h2>数据源发现</h2><p>连接 JSONL、MySQL 或 Redis 数据源，在 Replay 版面统一管理 FSM 记录。</p></div>
    <section className="settings-block source-config-block">
      <div className="settings-block-head"><div><strong>添加数据源</strong><span>选择数据源类型后填写对应的连接配置。</span></div><span className="settings-block-badge">STORE</span></div>
      <nav className="source-type-nav" aria-label="数据源类型" role="tablist">
        {[['jsonl', 'JSONL', '本地或 SSH 目录'], ['mysql', 'MySQL', '关系数据库'], ['redis', 'Redis', '键值存储']].map(([value, label, hint]) => <button key={value} className={driver === value ? 'active' : ''} type="button" role="tab" aria-selected={driver === value} onClick={() => { setDriver(value); setDirectoryBrowser(null); setRemoteBrowser(null); setStatus({ message: '', error: false }) }}><strong>{label}</strong><span>{hint}</span></button>)}
      </nav>
      <form ref={formRef} className="discovery-form source-config-form" onSubmit={submit}>
        <div className="discovery-field"><label htmlFor="sourceId">数据源 ID</label><input id="sourceId" name="id" required placeholder={`${driver}-archive`} /></div>
        <div className="discovery-field"><label htmlFor="sourceName">显示名称</label><input id="sourceName" name="name" placeholder={`${driver.toUpperCase()} Archive`} /></div>
        {driver === 'jsonl' && <>
          <div className="source-location-nav wide" role="tablist" aria-label="JSONL 目录位置"><button className={jsonlMode === 'local' ? 'active' : ''} type="button" role="tab" aria-selected={jsonlMode === 'local'} onClick={() => { setJSONLMode('local'); setDirectoryBrowser(null); setRemoteBrowser(null) }}>本地目录</button><button className={jsonlMode === 'ssh' ? 'active' : ''} type="button" role="tab" aria-selected={jsonlMode === 'ssh'} onClick={() => { setJSONLMode('ssh'); setDirectoryBrowser(null); setRemoteBrowser(null) }}>SSH / SFTP</button></div>
          {jsonlMode === 'ssh' && <><div className="discovery-field"><label htmlFor="sshAddress">SSH 地址</label><input id="sshAddress" name="sshAddress" required placeholder="server.example.com:22" /></div><div className="discovery-field"><label htmlFor="sshUsername">用户名</label><input id="sshUsername" name="sshUsername" required placeholder="fsm-reader" /></div><div className="discovery-field"><label htmlFor="sshPassword">密码</label><input id="sshPassword" name="sshPassword" type="password" autoComplete="new-password" /></div><div className="discovery-field"><label htmlFor="privateKeyFile">私钥文件</label><input id="privateKeyFile" name="privateKeyFile" placeholder="~/.ssh/id_ed25519" /></div><div className="discovery-field"><label htmlFor="privateKeyPassphrase">私钥口令</label><input id="privateKeyPassphrase" name="privateKeyPassphrase" type="password" /></div><div className="discovery-field"><label htmlFor="knownHostsFile">Known Hosts</label><input id="knownHostsFile" name="knownHostsFile" placeholder="~/.ssh/known_hosts" /></div></>}
          <div className="discovery-field wide"><label htmlFor="jsonlDirectory">{jsonlMode === 'local' ? '服务端本地目录' : '远程 JSONL 目录'}</label><div className="directory-input"><input id="jsonlDirectory" value={directory} onChange={(event) => { setDirectory(event.target.value); setRemoteBrowser(null) }} required placeholder={jsonlMode === 'local' ? '/var/lib/fsm' : '/srv/fsm/data'} />{jsonlMode === 'local' ? <button type="button" onClick={() => browseDirectory()}>选择目录</button> : <button type="button" disabled={sshBrowsing} onClick={() => browseRemoteDirectory()}>{sshBrowsing ? '连接中…' : remoteBrowser ? '刷新远程目录' : '连接并选择'}</button>}</div></div>
          <div className="discovery-field"><label htmlFor="jsonlFilename">写入文件名</label><input id="jsonlFilename" name="filename" value={filename} onChange={(event) => setFilename(event.target.value)} placeholder="fsm.jsonl" /></div>
          {jsonlMode === 'ssh' && <><div className="discovery-field"><label htmlFor="sshTimeout">连接超时（秒）</label><input id="sshTimeout" name="timeoutSeconds" type="number" min="1" defaultValue="10" /></div><label className="source-check wide"><input name="insecureIgnoreHostKey" type="checkbox"/><span>跳过 SSH Host Key 校验（仅限受信网络）</span></label></>}
          {jsonlMode === 'local' && directoryBrowser && <div className="directory-browser wide"><div className="directory-browser-head"><strong title={directoryBrowser.path}>{directoryBrowser.path}</strong><div>{directoryBrowser.parent && <button type="button" onClick={() => browseDirectory(directoryBrowser.parent)}>上一级</button>}<button type="button" onClick={() => { setDirectory(directoryBrowser.path); setDirectoryBrowser(null) }}>选择当前目录</button><button type="button" onClick={() => setDirectoryBrowser(null)}>关闭</button></div></div><div className="directory-browser-list">{directoryBrowser.directories.map((name) => <button key={name} type="button" onClick={() => openChildDirectory(name)}><span>□</span>{name}</button>)}{!directoryBrowser.directories.length && <span>当前目录没有子目录</span>}</div></div>}
          {jsonlMode === 'ssh' && remoteBrowser && <div className="directory-browser remote-directory-browser wide"><div className="directory-browser-head"><strong title={remoteBrowser.path}>{remoteBrowser.path}</strong><div>{remoteBrowser.parent && <button type="button" disabled={sshBrowsing} onClick={() => browseRemoteDirectory(remoteBrowser.parent)}>上一级</button>}<button type="button" onClick={() => { setDirectory(remoteBrowser.path); setRemoteBrowser(null); setStatus({ message: `已选择目录 ${remoteBrowser.path}`, error: false }) }}>选择当前目录</button><button type="button" onClick={() => setRemoteBrowser(null)}>关闭</button></div></div><div className="directory-browser-list">{remoteBrowser.entries.map((entry) => <button className={entry.directory ? 'directory-entry' : 'file-entry'} key={entry.path} type="button" disabled={sshBrowsing} onClick={() => chooseRemoteEntry(entry)} title={entry.path}><span>{entry.directory ? '□' : 'JSONL'}</span><b>{entry.name}</b>{!entry.directory && <small>{entry.size} B</small>}</button>)}{!remoteBrowser.entries.length && <span>当前目录没有子目录或 JSONL 文件</span>}</div></div>}
        </>}
        {driver === 'mysql' && <><div className="discovery-field wide"><label htmlFor="mysqlDSN">MySQL DSN</label><input id="mysqlDSN" name="dsn" required placeholder="user:password@tcp(127.0.0.1:3306)/fsm" /></div><div className="discovery-field"><label htmlFor="mysqlTable">数据表</label><input id="mysqlTable" name="table" placeholder="fsm_records" /></div></>}
        {driver === 'redis' && <><div className="discovery-field"><label htmlFor="redisAddress">Redis 地址</label><input id="redisAddress" name="address" required placeholder="127.0.0.1:6379" /></div><div className="discovery-field"><label htmlFor="redisDB">DB</label><input id="redisDB" name="db" type="number" min="0" defaultValue="0" /></div><div className="discovery-field"><label htmlFor="redisPassword">密码</label><input id="redisPassword" name="password" type="password" /></div><div className="discovery-field"><label htmlFor="redisPrefix">Key Prefix</label><input id="redisPrefix" name="keyPrefix" placeholder="fsm:records" /></div></>}
        <div className="discovery-form-footer"><button className="discovery-submit" type="submit" disabled={submitting}>{submitting ? '正在连接…' : '添加数据源'}</button><span className={`discovery-form-status${status.error ? ' error' : ''}`} role="status">{status.message}</span></div>
      </form>
    </section>
    <section className="settings-block"><div className="settings-block-head"><div><strong>已发现的数据源</strong><span>数据源中的 FSM 将显示在 Replay 左侧边栏。</span></div></div><div className="discovery-list">{!sources.length ? <div className="discovery-empty">尚未配置数据源</div> : sources.map((item) => <article className="discovery-item" key={item.id}><div className="discovery-item-head"><span className={`discovery-status-dot${item.lastError ? ' error' : ''}`}/><strong>{item.name || item.id}</strong><span className="discovery-item-count">{item.driver}</span></div><div className="discovery-item-meta">{item.location}</div>{item.lastError && <div className="discovery-item-error">{item.lastError}</div>}{!item.primary && <div className="discovery-actions"><button className="discovery-action danger" type="button" onClick={() => remove(item)}>删除</button></div>}</article>)}</div></section>
  </>
}

function DiscoverySettings({ open, onClose, discoveries, reloadDiscoveries, reloadFSMs }) {
  const [section, setSection] = useState('discovery')
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
          <nav className="settings-nav" aria-label="设置分类"><span className="settings-nav-label">连接</span><button className={`settings-nav-item${section === 'discovery' ? ' active' : ''}`} type="button" aria-current={section === 'discovery' ? 'page' : undefined} onClick={() => setSection('discovery')}><span className="settings-nav-icon"><svg viewBox="0 0 24 24"><circle cx="6" cy="12" r="2"/><circle cx="18" cy="6" r="2"/><circle cx="18" cy="18" r="2"/><path d="M8 11l8-4M8 13l8 4"/></svg></span><strong>服务发现</strong><small>Client Server 与 Pull 配置</small></button><button className={`settings-nav-item${section === 'replay' ? ' active' : ''}`} type="button" aria-current={section === 'replay' ? 'page' : undefined} onClick={() => setSection('replay')}><span className="settings-nav-icon"><svg viewBox="0 0 24 24"><path d="M4 7h16M4 12h16M4 17h10"/><path d="M17 15l3 2-3 2z"/></svg></span><strong>数据源发现</strong><small>JSONL、MySQL 与 Redis</small></button></nav>
          <main className="settings-content">
            {section === 'discovery' ? <>
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
            </> : <ReplaySourceSettings />}
          </main>
        </div>
      </section>
    </div>
  )
}

function App() {
  const [page, setPage] = useState(() => stored('fsm-page', 'visualization'))
  const [fsms, setFSMs] = useState([])
  const [discoveries, setDiscoveries] = useState([])
  const [selected, setSelected] = useState('')
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [theme, setTheme] = useState(() => stored('fsm-visualization-theme', matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark'))
  const [transparency, setTransparency] = useState(() => Number(stored('fsm-overlay-transparency', '85')))
  const [opacityOpen, setOpacityOpen] = useState(false)
  const [sidebarCollapsed, setSidebarCollapsed] = useState(() => stored('fsm-sidebar-collapsed', 'open') === 'collapsed')
  const [rightSidebarCollapsed, setRightSidebarCollapsed] = useState(() => stored("fsm-right-sidebar-collapsed", "open") === "collapsed")
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
  useEffect(() => { remember('fsm-page', page) }, [page])
  useEffect(() => {
    document.body.classList.toggle('sidebar-collapsed', sidebarCollapsed)
    remember('fsm-sidebar-collapsed', sidebarCollapsed ? 'collapsed' : 'open')
  }, [sidebarCollapsed])
  useEffect(() => {
    document.body.classList.toggle("right-sidebar-collapsed", rightSidebarCollapsed)
    remember("fsm-right-sidebar-collapsed", rightSidebarCollapsed ? "collapsed" : "open")
  }, [rightSidebarCollapsed])
  useEffect(() => {
    const value = Math.max(0, Math.min(100, Number(transparency) || 0))
    const opacity = 100 - value
    const root = document.documentElement
    root.style.setProperty("--panel-opacity", (opacity / 100).toFixed(2))
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
  const currentPath = current ? "/" + [current.path, machineName(current)].filter(Boolean).join("/") : "/"
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
        {page === 'visualization' && <Sidebar fsms={fsms} selected={selected} onSelect={setSelected} collapsed={sidebarCollapsed} onToggle={() => setSidebarCollapsed((value) => !value)} />}
        <div id="sidebarResize" className="resize-handle" role="separator" aria-label="调整状态机列表宽度" aria-orientation="vertical" tabIndex="0" onPointerDown={resizeStart} onKeyDown={(event) => { if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return; event.preventDefault(); const currentWidth = layoutRef.current?.querySelector(".left-sidebar")?.getBoundingClientRect().width || 288; const width = Math.max(190, currentWidth + (event.key === "ArrowLeft" ? -16 : 16)); document.documentElement.style.setProperty("--sidebar-width", `${width}px`); remember("fsm-sidebar-width", width) }} onDoubleClick={() => { document.documentElement.style.removeProperty('--sidebar-width'); remember('fsm-sidebar-width', '') }} />
        <header className="topbar">
          <div className="topbar-current" title={page === 'replay' ? 'Replay / Data' : currentPath}><span className="topbar-current-label">{page === 'replay' ? 'VIEW' : 'FSM'}</span><span className="topbar-current-path">{page === 'replay' ? '/replay' : currentPath}</span></div>
          <nav className="page-nav" aria-label="主版面">
            <button className={page === 'visualization' ? 'active' : ''} type="button" aria-current={page === 'visualization' ? 'page' : undefined} onClick={() => setPage('visualization')}>Visualization</button>
            <button className={page === 'replay' ? 'active' : ''} type="button" aria-current={page === 'replay' ? 'page' : undefined} onClick={() => setPage('replay')}>Replay</button>
          </nav>
          <div className="topbar-actions">
            <div ref={opacityRef} className={`opacity-control${opacityOpen ? ' open' : ''}`}>
              <button className="icon-button" type="button" aria-label={`全局透明度 ${Math.round(transparency)}%`} title={`全局透明度 ${Math.round(transparency)}%`} aria-expanded={opacityOpen} onClick={() => setOpacityOpen((value) => !value)}><svg viewBox="0 0 24 24"><path d="M12 3s5 6 5 10a5 5 0 0 1-10 0c0-4 5-10 5-10z"/><path d="M9 14.5c.7 1.3 1.7 2 3 2"/></svg></button>
              <div className="opacity-popover" role="dialog" aria-label="全局透明度"><div className="opacity-popover-head"><strong>全局透明度</strong><span className="opacity-value">{Math.round(transparency)}%</span></div><input className="opacity-slider" type="range" min="0" max="100" step="1" value={transparency} onChange={(event) => setTransparency(Number(event.target.value))} aria-label="全局透明度"/><div className="opacity-scale"><span>不透明</span><span>全透明</span></div></div>
            </div>
            <button id="themeToggle" className="icon-button" type="button" aria-label={`切换为${theme === 'dark' ? '浅色' : '深色'}主题`} title={`切换为${theme === 'dark' ? '浅色' : '深色'}主题`} onClick={() => setTheme((value) => value === 'light' ? 'dark' : 'light')}><svg className="theme-sun" viewBox="0 0 24 24"><circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.93 4.93l1.42 1.42M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.42-1.42M17.66 6.34l1.41-1.41"/></svg><svg className="theme-moon" viewBox="0 0 24 24"><path d="M20.5 14.2A8.5 8.5 0 0 1 9.8 3.5a8.5 8.5 0 1 0 10.7 10.7z"/></svg></button>
            <button id="discoveryToggle" className="icon-button" type="button" aria-label="打开设置" title="设置" aria-expanded={settingsOpen} onClick={() => setSettingsOpen(true)}><svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .34 1.88l.06.06-1.86 1.86-.06-.06A1.7 1.7 0 0 0 16 18.4a1.7 1.7 0 0 0-1 .6 1.7 1.7 0 0 0-.4 1.1V20h-2.6v-.09A1.7 1.7 0 0 0 10.9 18.4a1.7 1.7 0 0 0-1.88.34l-.06.06-1.86-1.86.06-.06A1.7 1.7 0 0 0 7.5 15a1.7 1.7 0 0 0-1.5-1H5.9v-2.6H6a1.7 1.7 0 0 0 1.5-1 1.7 1.7 0 0 0-.34-1.88l-.06-.06L8.96 6.6l.06.06A1.7 1.7 0 0 0 10.9 7a1.7 1.7 0 0 0 1-1.5v-.1h2.6v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.88-.34l.06-.06 1.86 1.86-.06.06A1.7 1.7 0 0 0 18.9 10.4a1.7 1.7 0 0 0 1.5 1h.1V14h-.1a1.7 1.7 0 0 0-1 .6z"/></svg></button>
            <button className="sidebar-toggle right-sidebar-toggle" type="button" aria-label={(rightSidebarCollapsed ? "展开" : "折叠") + "右侧栏"} title={(rightSidebarCollapsed ? "展开" : "折叠") + "右侧栏"} aria-expanded={!rightSidebarCollapsed} onClick={() => setRightSidebarCollapsed((value) => !value)}>
              <svg className="right-sidebar-open-icon" viewBox="0 0 16 16"><rect x="1.5" y="2.5" width="13" height="11" rx="1.5"/><rect className="sidebar-pane-fill" x="10" y="3.5" width="3.5" height="9" rx=".75"/><path d="M9.5 3v10"/></svg>
              <svg className="right-sidebar-closed-icon" viewBox="0 0 16 16"><rect x="1.5" y="2.5" width="13" height="11" rx="1.5"/><path d="M9.5 3v10"/></svg>
            </button>
          </div>
        </header>
        {page === 'replay' ? <ReplayPage sidebarCollapsed={sidebarCollapsed} onSidebarToggle={() => setSidebarCollapsed((value) => !value)} payloadOpen={payloadOpen} onPayloadToggle={(value) => setPanel("payloadPanel", value)} /> : <main className="workspace">
          {!current ? <div className="empty"><div><strong>等待 FSM 注册</strong><span>注册完成后将在此显示状态图。</span></div></div> : <div className="content visible"><section className="graph-surface" aria-label="FSM 状态图画布">
            <StateGraph item={current} />
            {current.collectionError && <div className="collection-error" style={{ display: 'block' }}>{current.collectionError}</div>}
            <ActiveEventPanel item={current} open={activeOpen} onToggle={(value) => setPanel("availablePanel", value)} />
            <TracePanel traces={current.traces} />
          </section></div>}
        </main>}
        {page === 'visualization' && <RightSidebar item={current} payloadOpen={payloadOpen} onPayloadToggle={(value) => setPanel("payloadPanel", value)} />}
      </div>
      <DiscoverySettings open={settingsOpen} onClose={closeSettings} discoveries={discoveries} reloadDiscoveries={loadDiscoveries} reloadFSMs={loadFSMs} />
    </>
  )
}

export default App
