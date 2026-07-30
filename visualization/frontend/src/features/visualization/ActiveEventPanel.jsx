export default function ActiveEventPanel({ item, open, onToggle }) {
  const available = [...(item.availableTransitions || (item.transitions || []).filter((edge) => edge.fromState === item.currentState))]
    .sort((left, right) => String(left.event).localeCompare(String(right.event)) || String(left.toState).localeCompare(String(right.toState)))
  const currentState = String(item.currentState || 'empty')
  return <section className={`monitor-panel available-panel${open ? '' : ' collapsed'}`} aria-labelledby="activeEventTitle">
    <div className="monitor-panel-head"><strong id="activeEventTitle">Active Event</strong><span className="panel-summary">{available.length} events</span><button className="active-event-toggle" type="button" aria-label={open ? '折叠 Active Event' : '展开 Active Event'} title={open ? '折叠 Active Event' : '展开 Active Event'} aria-expanded={open} aria-controls="activeEventBody" onClick={() => onToggle(!open)}><svg className="active-event-collapse-icon" viewBox="0 0 24 24"><path d="M6 15l6-6 6 6" /></svg><svg className="active-event-expand-icon" viewBox="0 0 24 24"><path d="M6 9l6 6 6-6" /></svg></button></div>
    <div id="activeEventBody" className="panel-body"><div className="active-event-tree" role="tree" aria-label={`${currentState} 可接收事件`}>
      <div className="active-tree-root" role="treeitem" aria-expanded={available.length > 0}><span className="active-tree-root-dot" aria-hidden="true" /><strong>{currentState}</strong><span className="active-tree-current">CURRENT</span></div>
      {!available.length && <div className="available-empty">当前状态没有可接收的事件</div>}
      {!!available.length && <div className="active-tree-branches" role="group">{available.map((edge, index) => {
        const processing = String(edge.event) === String(item.processingEvent || '')
        return <div key={`${edge.event}-${edge.toState}-${index}`} className={`active-tree-branch${processing ? ' processing' : ''}`} role="treeitem"><span className="active-tree-prefix" aria-hidden="true">{index === available.length - 1 ? '└──' : '├──'}</span><span className="event-token">{edge.event}</span><span className="active-tree-arrow" aria-hidden="true">→</span><span className="target-state">{edge.toState}</span>{processing && <span className="active-tree-processing">PROCESSING</span>}</div>
      })}</div>}
    </div></div>
  </section>
}
