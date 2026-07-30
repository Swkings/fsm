import { useCallback, useEffect, useRef, useState } from 'react'
import { formatTime } from '../../lib/format.js'
import { remember, stored } from '../../lib/storage.js'

function PayloadContent({ item }) {
  return <div id="payloadPanel" className="payload-content" aria-label="Payload">
    <div className="meta payload-meta"><div><label>Transition ID</label><span>{item.transitionId}</span></div><div><label>Updated</label><span>{formatTime(item.updatedAt)}</span></div><div><label>Previous State</label><span>{item.previousState || '-'}</span></div><div><label>Previous Event</label><span>{item.previousEvent || '-'}</span></div></div>
    {item.payloadError && <div className="payload-error" style={{ display: 'block' }}>{String(item.payloadError)}</div>}
    <pre className="payload-code"><code>{JSON.stringify(item.payload === undefined ? null : item.payload, null, 2)}</code></pre>
  </div>
}

export default function RightSidebar({ item, payloadOpen, onPayloadToggle }) {
  const defaultWidth = window.innerWidth <= 520 ? 220 : window.innerWidth <= 820 ? 280 : window.innerWidth <= 1100 ? 320 : 400
  const minimumWidth = 220
  const [width, setWidth] = useState(() => {
    const value = Number(stored('fsm-right-sidebar-width', String(defaultWidth)))
    return Math.round(Math.max(minimumWidth, Math.min(window.innerWidth * .6, Number.isFinite(value) ? value : defaultWidth)))
  })
  const [resizing, setResizing] = useState(false)
  const resizeRef = useRef(null)
  const maximumWidth = useCallback(() => Math.max(minimumWidth, Math.min(720, window.innerWidth * .6)), [])
  const setClampedWidth = useCallback((value, persist = false) => {
    const next = Math.round(Math.max(minimumWidth, Math.min(maximumWidth(), value)))
    setWidth(next)
    if (resizeRef.current) resizeRef.current.width = next
    if (persist) remember('fsm-right-sidebar-width', next)
    return next
  }, [maximumWidth])
  const startResize = (event) => { if (event.button !== 0) return; event.preventDefault(); event.currentTarget.setPointerCapture(event.pointerId); resizeRef.current = { pointerId: event.pointerId, startX: event.clientX, startWidth: width, width }; setResizing(true) }
  const moveResize = (event) => { const resize = resizeRef.current; if (!resize || resize.pointerId !== event.pointerId) return; setClampedWidth(resize.startWidth + resize.startX - event.clientX) }
  const stopResize = (event) => { const resize = resizeRef.current; if (!resize || (event.pointerId !== undefined && resize.pointerId !== event.pointerId)) return; resizeRef.current = null; setResizing(false); remember('fsm-right-sidebar-width', resize.width) }
  const resizeWithKeyboard = (event) => { if (!['ArrowLeft', 'ArrowRight', 'Home'].includes(event.key)) return; event.preventDefault(); setClampedWidth(event.key === 'Home' ? defaultWidth : width + (event.key === 'ArrowLeft' ? 20 : -20), true) }
  useEffect(() => { document.documentElement.style.setProperty('--right-sidebar-width', `${width}px`) }, [width])
  useEffect(() => { const fit = () => setClampedWidth(width); window.addEventListener('resize', fit); return () => window.removeEventListener('resize', fit) }, [setClampedWidth, width])
  useEffect(() => { document.body.classList.toggle('right-sidebar-resizing', resizing); return () => document.body.classList.remove('right-sidebar-resizing') }, [resizing])
  return <aside className={`right-sidebar${resizing ? ' resizing' : ''}`} aria-label="FSM Inspector">
    <div className="right-sidebar-resize" role="separator" aria-label="调整右侧边栏宽度" aria-orientation="vertical" aria-valuemin={minimumWidth} aria-valuemax={Math.round(maximumWidth())} aria-valuenow={width} tabIndex="0" title="拖动调整宽度 · 双击恢复默认" onPointerDown={startResize} onPointerMove={moveResize} onPointerUp={stopResize} onPointerCancel={stopResize} onLostPointerCapture={stopResize} onKeyDown={resizeWithKeyboard} onDoubleClick={() => setClampedWidth(defaultWidth, true)} />
    <nav className="right-sidebar-nav" aria-label="FSM Inspector 面板"><button className={`inspector-tab${payloadOpen ? ' active' : ''}`} type="button" disabled={!item} aria-controls="payloadPanel" aria-expanded={payloadOpen} onClick={() => onPayloadToggle(!payloadOpen)}><strong>Payload</strong></button></nav>
    <div className="right-sidebar-content">{!item ? <div className="right-sidebar-empty">选择 FSM 后查看 Payload</div> : payloadOpen ? <PayloadContent item={item} /> : <div className="right-sidebar-empty">点击 Payload 查看当前数据</div>}</div>
  </aside>
}
