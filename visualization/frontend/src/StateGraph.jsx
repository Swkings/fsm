import { useEffect, useMemo, useRef, useState } from 'react'

const clamp = (value, minimum, maximum) => Math.max(minimum, Math.min(maximum, value))

function graphGeometry(item) {
  const transitions = item.transitions || []
  const rawStates = item.states?.length
    ? item.states
    : [item.currentState, ...transitions.flatMap((edge) => [edge.fromState, edge.toState])]
  const states = [...new Set(rawStates.filter((state) => state !== undefined && state !== null).map(String))].sort()
  if (!states.length) return { states, transitions, width: 760, height: 560, positions: new Map(), edges: [] }

  const radius = Math.max(155, Math.min(360, states.length * 38))
  const width = Math.max(760, radius * 2 + 240)
  const height = Math.max(560, radius * 2 + 180)
  const centerX = width / 2
  const centerY = height / 2
  const positions = new Map()
  states.forEach((state, index) => {
    if (states.length === 1) positions.set(state, { x: centerX, y: centerY })
    else {
      const angle = -Math.PI / 2 + (index * Math.PI * 2) / states.length
      positions.set(state, {
        x: centerX + Math.cos(angle) * radius,
        y: centerY + Math.sin(angle) * Math.min(radius, height / 2 - 100),
      })
    }
  })

  const groups = new Map()
  transitions.forEach((edge) => {
    const from = String(edge.fromState)
    const to = String(edge.toState)
    const key = JSON.stringify([from, to])
    if (!groups.has(key)) groups.set(key, { from, to, events: [] })
    groups.get(key).events.push(String(edge.event))
  })
  const latest = item.traces?.length ? item.traces[item.traces.length - 1] : null
  const edges = [...groups.values()].flatMap((group) => {
    const from = positions.get(group.from)
    const to = positions.get(group.to)
    if (!from || !to) return []
    const processing = group.from === String(item.currentState) && group.events.includes(String(item.processingEvent || ''))
    const recent = latest && group.from === String(latest.fromState) && group.to === String(latest.toState) && group.events.includes(String(latest.event))
    const available = group.from === String(item.currentState)
    const status = processing ? 'processing' : recent ? (latest.success ? 'recent' : 'failed') : available ? 'available' : ''
    let d
    let labelX
    let labelY
    if (group.from === group.to) {
      d = `M ${from.x - 25} ${from.y - 31} C ${from.x - 82} ${from.y - 112} ${from.x + 82} ${from.y - 112} ${from.x + 25} ${from.y - 31}`
      labelX = from.x
      labelY = from.y - 100
    } else {
      const dx = to.x - from.x
      const dy = to.y - from.y
      const length = Math.max(1, Math.hypot(dx, dy))
      const startX = from.x + (dx / length) * 58
      const startY = from.y + (dy / length) * 38
      const endX = to.x - (dx / length) * 62
      const endY = to.y - (dy / length) * 40
      const reverse = groups.has(JSON.stringify([group.to, group.from]))
      const curve = reverse ? 34 : 0
      const controlX = (startX + endX) / 2 - (dy / length) * curve
      const controlY = (startY + endY) / 2 + (dx / length) * curve
      d = curve
        ? `M ${startX} ${startY} Q ${controlX} ${controlY} ${endX} ${endY}`
        : `M ${startX} ${startY} L ${endX} ${endY}`
      labelX = curve ? (startX + 2 * controlX + endX) / 4 : (startX + endX) / 2 - (dy / length) * 12
      labelY = curve ? (startY + 2 * controlY + endY) / 4 : (startY + endY) / 2 + (dx / length) * 12
    }
    return [{ ...group, status, d, labelX, labelY }]
  })
  return { states, transitions, width, height, positions, edges }
}

function ArrowMarkers() {
  return (
    <defs>
      {[
        ['arrow-default', 'var(--graph-line)'],
        ['arrow-available', 'var(--primary)'],
        ['arrow-recent', 'var(--success)'],
        ['arrow-processing', 'var(--warning)'],
        ['arrow-failed', 'var(--danger)'],
      ].map(([id, fill]) => (
        <marker key={id} id={id} viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
          <path d="M 0 0 L 10 5 L 0 10 z" fill={fill} />
        </marker>
      ))}
    </defs>
  )
}

function exportPNG(svg, item) {
  if (!svg) return
  const clone = svg.cloneNode(true)
  const properties = ['fill', 'stroke', 'stroke-width', 'stroke-dasharray', 'stroke-dashoffset', 'stroke-linecap', 'stroke-linejoin', 'opacity', 'filter', 'font-family', 'font-size', 'font-weight', 'letter-spacing', 'text-anchor', 'dominant-baseline', 'paint-order']
  const sources = [svg, ...svg.querySelectorAll('*')]
  const targets = [clone, ...clone.querySelectorAll('*')]
  sources.forEach((source, index) => {
    const computed = getComputedStyle(source)
    const style = properties.map((property) => `${property}:${computed.getPropertyValue(property)}`).join(';')
    targets[index].setAttribute('style', style)
  })
  const { x, y, width, height } = svg.viewBox.baseVal
  const background = getComputedStyle(document.documentElement).getPropertyValue('--graph-bg').trim() || '#fff'
  const rect = document.createElementNS('http://www.w3.org/2000/svg', 'rect')
  Object.entries({ x, y, width, height, fill: background }).forEach(([key, value]) => rect.setAttribute(key, value))
  clone.insertBefore(rect, clone.firstChild)
  clone.setAttribute('xmlns', 'http://www.w3.org/2000/svg')
  clone.setAttribute('width', width)
  clone.setAttribute('height', height)
  const sourceURL = URL.createObjectURL(new Blob([new XMLSerializer().serializeToString(clone)], { type: 'image/svg+xml;charset=utf-8' }))
  const image = new Image()
  image.onload = () => {
    URL.revokeObjectURL(sourceURL)
    const canvas = document.createElement('canvas')
    canvas.width = width * 2
    canvas.height = height * 2
    const context = canvas.getContext('2d')
    context.scale(2, 2)
    context.fillStyle = background
    context.fillRect(0, 0, width, height)
    context.drawImage(image, 0, 0, width, height)
    canvas.toBlob((blob) => {
      if (!blob) return
      const url = URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.download = `${String(item.displayName || item.name || 'fsm').replace(/[^a-zA-Z0-9._-]+/g, '-') || 'fsm'}-state-graph.png`
      link.click()
      setTimeout(() => URL.revokeObjectURL(url), 1000)
    }, 'image/png')
  }
  image.src = sourceURL
}

export default function StateGraph({ item }) {
  const hostRef = useRef(null)
  const svgRef = useRef(null)
  const viewsRef = useRef(new Map())
  const dragRef = useRef(null)
  const [view, setView] = useState(() => ({ x: 0, y: 0, zoom: 1 }))
  const [dragging, setDragging] = useState(false)
  const [tooltip, setTooltip] = useState(null)
  const geometry = useMemo(() => graphGeometry(item), [item])

  useEffect(() => {
    const stored = viewsRef.current.get(item.id) || { x: 0, y: 0, zoom: 1 }
    setView({ ...stored })
    setTooltip(null)
  }, [item.id])

  const updateView = (next) => {
    viewsRef.current.set(item.id, next)
    setView(next)
  }
  const moveTooltip = (event, state) => {
    const host = hostRef.current
    if (!host) return
    const bounds = host.getBoundingClientRect()
    const outgoing = geometry.transitions.filter((edge) => String(edge.fromState) === state)
    const incoming = geometry.transitions.filter((edge) => String(edge.toState) === state)
    setTooltip({
      left: clamp(event.clientX - bounds.left + 14, 8, Math.max(8, bounds.width - 288)),
      top: clamp(event.clientY - bounds.top + 14, 8, Math.max(8, bounds.height - 138)),
      state,
      status: state === String(item.currentState) ? '当前状态' : state === String(item.previousState) ? '上一状态' : '普通状态',
      incoming: incoming.length,
      outgoing: outgoing.length,
      accepted: outgoing.length ? outgoing.map((edge) => `${edge.event} → ${edge.toState}`).join('，') : '无',
    })
  }
  const pointerDown = (event) => {
    if (event.button !== 0) return
    dragRef.current = { pointerId: event.pointerId, originX: event.clientX, originY: event.clientY, startX: view.x, startY: view.y }
    setDragging(true)
    setTooltip(null)
    event.currentTarget.setPointerCapture(event.pointerId)
    event.preventDefault()
  }
  const pointerMove = (event) => {
    const drag = dragRef.current
    if (!drag || event.pointerId !== drag.pointerId) return
    updateView({ ...view, x: drag.startX + event.clientX - drag.originX, y: drag.startY + event.clientY - drag.originY })
  }
  const pointerEnd = (event) => {
    const drag = dragRef.current
    if (!drag || event.pointerId !== drag.pointerId) return
    dragRef.current = null
    setDragging(false)
    if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId)
  }
  const reset = () => updateView({ x: 0, y: 0, zoom: 1 })
  const zoom = (delta) => updateView({ ...view, zoom: Math.round(clamp(view.zoom + delta, 0.5, 2) * 100) / 100 })
  const fit = Math.max(0.65, Math.min(1.15, 760 / geometry.width, 560 / geometry.height))

  return (
    <>
      <div
        ref={hostRef}
        id="stateGraph"
        className={`state-graph${dragging ? ' dragging' : ''}`}
        onPointerDown={pointerDown}
        onPointerMove={pointerMove}
        onPointerUp={pointerEnd}
        onPointerCancel={pointerEnd}
        onDoubleClick={(event) => { if (!event.target.closest('.graph-node')) reset() }}
      >
        {!geometry.states.length ? <div className="available-empty">尚未配置状态</div> : (
          <div className="graph-stage" style={{ width: geometry.width * fit, height: geometry.height * fit, transform: `translate(calc(-50% + ${view.x}px), calc(-50% + ${view.y}px)) scale(${view.zoom})` }}>
            <svg ref={svgRef} viewBox={`0 0 ${geometry.width} ${geometry.height}`} role="img" aria-label="FSM 状态转移图">
              <ArrowMarkers />
              <g>
                {geometry.edges.map((edge) => {
                  const marker = edge.status === 'processing' ? 'arrow-processing' : edge.status === 'recent' ? 'arrow-recent' : edge.status === 'failed' ? 'arrow-failed' : edge.status === 'available' ? 'arrow-available' : 'arrow-default'
                  return <path key={`${edge.from}-${edge.to}`} d={edge.d} className={`graph-edge${edge.status ? ` ${edge.status}` : ''}`} markerEnd={`url(#${marker})`}><title>{`${edge.from} -- ${edge.events.join(', ')} --&gt; ${edge.to}`}</title></path>
                })}
              </g>
              <g>
                {geometry.edges.map((edge) => <text key={`${edge.from}-${edge.to}-label`} x={edge.labelX} y={edge.labelY} className={`graph-edge-label${edge.status ? ` ${edge.status}` : ''}`} textAnchor="middle">{edge.events.join(' / ')}</text>)}
              </g>
              <g>
                {geometry.states.map((state) => {
                  const position = geometry.positions.get(state)
                  const width = Math.max(104, Math.min(190, state.length * 9 + 38))
                  const current = state === String(item.currentState)
                  const previous = state === String(item.previousState) && !current
                  return (
                    <g
                      key={state}
                      className={`graph-node${current ? ' current' : previous ? ' previous' : ''}`}
                      tabIndex="0"
                      aria-label={`状态 ${state}`}
                      onPointerEnter={(event) => moveTooltip(event, state)}
                      onPointerMove={(event) => moveTooltip(event, state)}
                      onPointerLeave={() => setTooltip(null)}
                      onFocus={(event) => { const bounds = event.currentTarget.getBoundingClientRect(); moveTooltip({ clientX: bounds.right, clientY: bounds.top }, state) }}
                      onBlur={() => setTooltip(null)}
                    >
                      <rect x={position.x - width / 2} y={position.y - 31} width={width} height="62" rx="13" />
                      <text x={position.x} y={position.y - (current ? 7 : 0)}>{state}</text>
                      {current && <text x={position.x} y={position.y + 17} className="node-status">CURRENT</text>}
                    </g>
                  )
                })}
              </g>
            </svg>
          </div>
        )}
      </div>
      <div id="nodeTooltip" className={`node-tooltip${tooltip ? ' visible' : ''}`} style={tooltip ? { left: tooltip.left, top: tooltip.top } : undefined} role="tooltip">
        {tooltip && <><strong>{tooltip.state || '(empty)'}</strong><div className="node-tooltip-row"><span>状态</span><span>{tooltip.status}</span></div><div className="node-tooltip-row"><span>进入转移</span><span>{tooltip.incoming}</span></div><div className="node-tooltip-row"><span>离开转移</span><span>{tooltip.outgoing}</span></div><div className="node-tooltip-row"><span>可接收事件</span><span>{tooltip.accepted}</span></div></>}
      </div>
      <div className="canvas-toolbar" role="toolbar" aria-label="状态图画布工具">
        <button className="icon-button" type="button" aria-label="缩小状态图" title="缩小" disabled={!geometry.states.length} onClick={() => zoom(-0.15)}><svg viewBox="0 0 24 24"><circle cx="11" cy="11" r="7"/><path d="M8 11h6M16 16l5 5"/></svg></button>
        <span className="canvas-toolbar-value">{Math.round(view.zoom * 100)}%</span>
        <button className="icon-button" type="button" aria-label="放大状态图" title="放大" disabled={!geometry.states.length} onClick={() => zoom(0.15)}><svg viewBox="0 0 24 24"><circle cx="11" cy="11" r="7"/><path d="M8 11h6M11 8v6M16 16l5 5"/></svg></button>
        <button className="icon-button" type="button" aria-label="恢复默认 FSM 图" title="恢复默认 FSM 图" disabled={!geometry.states.length} onClick={reset}><svg viewBox="0 0 24 24"><path d="M4 9V4h5M20 15v5h-5M5.5 5.5A8 8 0 0 1 19 9M18.5 18.5A8 8 0 0 1 5 15"/></svg></button>
        <span className="canvas-toolbar-separator" aria-hidden="true" />
        <button className="icon-button" type="button" aria-label="导出状态图 PNG" title="导出状态图 PNG" disabled={!geometry.states.length} onClick={() => exportPNG(svgRef.current, item)}><svg viewBox="0 0 24 24"><path d="M12 3v12M7 10l5 5 5-5M4 19h16"/></svg></button>
      </div>
    </>
  )
}
