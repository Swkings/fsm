import { useEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { formatNanos } from '../../lib/format.js'
import { remember, stored } from '../../lib/storage.js'

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
  return createPortal(<div className="trace-context-menu" style={{ top: menu.top, left: menu.left }} role="menu" aria-label="Trace 操作" onContextMenu={(event) => event.preventDefault()}>
    <button type="button" role="menuitem" onClick={() => onView(menu.trace)}><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M2.5 12s3.4-6 9.5-6 9.5 6 9.5 6-3.4 6-9.5 6-9.5-6-9.5-6Z"/><circle cx="12" cy="12" r="2.7"/></svg><span><strong>查看 FSM 数据</strong><small>进入和退出快照</small></span></button>
  </div>, document.body)
}

function TraceMetadataDialog({ trace, onClose }) {
  if (!trace) return null
  return createPortal(<div className="trace-metadata-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose() }}>
    <section className="trace-metadata-dialog" role="dialog" aria-modal="true" aria-labelledby="traceMetadataTitle">
      <header><div><strong id="traceMetadataTitle">FSM 数据快照</strong><span>{trace.event || '-'} · {trace.fromState || '-'} → {trace.finalState || '-'}</span></div><button type="button" aria-label="关闭 FSM 数据" title="关闭" onClick={onClose}><svg viewBox="0 0 24 24"><path d="M6 6l12 12M18 6 6 18"/></svg></button></header>
      <div className="trace-metadata-grid"><section><strong>进入 FSM</strong><pre>{JSON.stringify(trace.entryMetadata === undefined ? null : trace.entryMetadata, null, 2)}</pre></section><section><strong>退出 FSM</strong><pre>{JSON.stringify(trace.exitMetadata === undefined ? null : trace.exitMetadata, null, 2)}</pre></section></div>
    </section>
  </div>, document.body)
}

export default function TracePanel({ traces, selectedId = '', onSelect }) {
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
      return [trace.event, trace.fromState, trace.toState, trace.finalState, trace.success ? 'success' : 'failed', trace.error, trace.transitionId, trace.beginTime, trace.endTime, date?.toLocaleString(), date?.toLocaleTimeString(), date?.toISOString()].some((value) => String(value ?? '').toLocaleLowerCase().includes(needle))
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
    const onKeyDown = (event) => { if (event.key === 'Escape') { setContextMenu(null); setMetadataTrace(null) } }
    const onPointerDown = (event) => { if (contextMenu && !event.target.closest('.trace-context-menu')) setContextMenu(null) }
    document.addEventListener('keydown', onKeyDown); document.addEventListener('pointerdown', onPointerDown)
    return () => { document.removeEventListener('keydown', onKeyDown); document.removeEventListener('pointerdown', onPointerDown) }
  }, [contextMenu, metadataTrace])
  const filtered = query.trim() || startTime || endTime
  const clearFilters = () => { setQuery(''); setStartTime(''); setEndTime('') }
  const openTraceMenu = (event, trace) => {
    event.preventDefault()
    const rect = event.currentTarget.getBoundingClientRect()
    setMetadataTrace(null)
    setContextMenu({ trace, left: Math.max(8, Math.min(event.clientX || rect.left + 24, window.innerWidth - 220)), top: Math.max(8, Math.min(event.clientY || rect.top + rect.height / 2, window.innerHeight - 62)) })
  }
  const maximumHeight = () => Math.max(minimumHeight, (panelRef.current?.parentElement?.getBoundingClientRect().height || window.innerHeight) - 28)
  const setClampedHeight = (value, persist = false) => {
    const height = Math.round(Math.max(minimumHeight, Math.min(maximumHeight(), value)))
    setTraceHeight(height); if (persist) remember('fsm-trace-height', height); return height
  }
  const startTraceResize = (event) => { if (!expanded || event.button !== 0) return; event.preventDefault(); event.currentTarget.setPointerCapture(event.pointerId); resizeRef.current = { pointerId: event.pointerId, startY: event.clientY, startHeight: panelRef.current?.getBoundingClientRect().height || traceHeight, height: traceHeight }; setResizing(true) }
  const moveTraceResize = (event) => { const resize = resizeRef.current; if (!resize || resize.pointerId !== event.pointerId) return; resize.height = setClampedHeight(resize.startHeight + resize.startY - event.clientY) }
  const stopTraceResize = (event) => { const resize = resizeRef.current; if (!resize || (event.pointerId !== undefined && resize.pointerId !== event.pointerId)) return; resizeRef.current = null; setResizing(false); remember('fsm-trace-height', resize.height) }
  const resizeTraceWithKeyboard = (event) => { if (!['ArrowUp', 'ArrowDown', 'Home'].includes(event.key)) return; event.preventDefault(); setClampedHeight(event.key === 'Home' ? defaultHeight : traceHeight + (event.key === 'ArrowUp' ? 24 : -24), true) }
  useEffect(() => { document.body.classList.toggle('trace-resizing', resizing); return () => document.body.classList.remove('trace-resizing') }, [resizing])
  return <section ref={panelRef} className={`monitor-panel trace-panel${expanded ? ' expanded' : ''}${resizing ? ' resizing' : ''}`} style={expanded ? { height: `min(${traceHeight}px, calc(100% - 28px))` } : undefined} aria-labelledby="traceTitle">
    <div className="trace-resize-handle" role="separator" aria-label="调整 Trace 面板高度" aria-orientation="horizontal" aria-valuemin={minimumHeight} aria-valuenow={Math.round(traceHeight)} tabIndex={expanded ? 0 : -1} title="拖动调整高度 · 双击恢复默认" onPointerDown={startTraceResize} onPointerMove={moveTraceResize} onPointerUp={stopTraceResize} onPointerCancel={stopTraceResize} onLostPointerCapture={stopTraceResize} onKeyDown={resizeTraceWithKeyboard} onDoubleClick={() => setClampedHeight(defaultHeight, true)} />
    <div className="trace-status-bar"><div className="trace-title"><strong id="traceTitle">Trace</strong><span className="panel-summary">{allTraces.length} traces</span></div><div className="trace-latest">{!latest ? <span className="trace-latest-empty">暂无 Trace</span> : <><span>{latest.beginTime ? new Date(latest.beginTime / 1000000).toLocaleTimeString() : '-'}</span><span className="trace-latest-event">{latest.event || '-'}</span><span>{latest.fromState || '-'} → {latest.finalState || '-'}</span><span className={`trace-latest-result ${latest.success ? 'ok' : 'fail'}`} title={latest.error || ''}>{latest.success ? 'success' : 'failed'}</span><span>{formatNanos(latest.duration)}</span></>}</div><button className="trace-toggle" type="button" aria-label={expanded ? '折叠 Trace 状态栏' : '展开完整 Trace'} title={expanded ? '折叠 Trace 状态栏' : '展开完整 Trace'} aria-expanded={expanded} onClick={() => setExpanded((value) => !value)}><svg className="trace-expand-icon" viewBox="0 0 24 24"><path d="M6 15l6-6 6 6" /></svg><svg className="trace-collapse-icon" viewBox="0 0 24 24"><path d="M6 9l6 6 6-6" /></svg></button></div>
    <div className="trace-history"><div className="trace-toolbar"><input className="trace-search" type="search" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索事件、状态、结果、错误或时间" aria-label="搜索 Trace" /><label><span>开始</span><input type="datetime-local" step="1" value={startTime} max={endTime || undefined} onChange={(event) => setStartTime(event.target.value)} /></label><label><span>结束</span><input type="datetime-local" step="1" value={endTime} min={startTime || undefined} onChange={(event) => setEndTime(event.target.value)} /></label><label><span>排序</span><select value={sortBy} onChange={(event) => setSortBy(event.target.value)}><option value="newest">最新优先</option><option value="oldest">最早优先</option><option value="duration-desc">耗时从长到短</option><option value="duration-asc">耗时从短到长</option></select></label><span className="trace-filter-count">{visible.length} / {allTraces.length}</span><button className="trace-clear" type="button" disabled={!filtered} onClick={clearFilters}>清空</button></div>
      <div className="trace-table-wrap"><table><thead><tr><th>时间</th><th>事件</th><th>状态流转</th><th>结果</th><th>耗时</th><th>错误</th></tr></thead><tbody>{visible.map((trace, index) => { const traceId = trace.replayRecordId || trace.transitionId || String(index); return <tr className={`trace-row${selectedId === traceId ? ' selected' : ''}${onSelect ? ' selectable' : ''}`} key={traceId} tabIndex={0} title="右键查看 FSM 数据" aria-current={selectedId === traceId ? 'true' : undefined} onClick={() => onSelect?.(trace)} onKeyDown={(event) => { if (onSelect && (event.key === 'Enter' || event.key === ' ')) { event.preventDefault(); onSelect(trace) } }} onContextMenu={(event) => openTraceMenu(event, trace)}><td>{formatTraceTime(trace)}</td><td>{trace.event}</td><td>{trace.fromState} → {trace.finalState}</td><td className={trace.success ? 'ok' : 'fail'}>{trace.success ? 'success' : 'failed'}</td><td>{formatNanos(trace.duration)}</td><td className={trace.error ? 'trace-error' : ''}>{trace.error || '-'}</td></tr> })}{!visible.length && <tr><td className="trace-empty" colSpan="6">没有匹配的 Trace</td></tr>}</tbody></table></div>
    </div>
    <TraceContextMenu menu={contextMenu} onView={(trace) => { setContextMenu(null); setMetadataTrace(trace) }} /><TraceMetadataDialog trace={metadataTrace} onClose={() => setMetadataTrace(null)} />
  </section>
}
