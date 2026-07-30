import { useCallback, useEffect, useMemo, useState } from 'react'
import { api } from '../../api.js'
import StateGraph from '../../StateGraph.jsx'
import { MachineButton } from '../../components/Sidebar.jsx'
import { compareMachines } from '../../lib/machines.js'
import { remember, stored } from '../../lib/storage.js'
import RightSidebar from '../visualization/RightSidebar.jsx'
import TracePanel from '../visualization/TracePanel.jsx'

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
  return <aside className="left-sidebar replay-sidebar">
    <div className="aside-heading"><span className="aside-title">Replay Sources</span><span className="count-pill">{sources.length} / {machineCount}</span><button className="sidebar-tool" type="button" aria-label="刷新 Replay 数据源" title="刷新" onClick={onRefresh}><svg viewBox="0 0 24 24"><path d="M20 7v5h-5M4 17v-5h5"/><path d="M6.1 8.2A7 7 0 0 1 18.8 9M17.9 15.8A7 7 0 0 1 5.2 15"/></svg></button><button className="sidebar-toggle" type="button" aria-label={`${collapsed ? '展开' : '折叠'}左侧栏`} title={`${collapsed ? '展开' : '折叠'}左侧栏`} aria-expanded={!collapsed} onClick={onToggle}><svg className="left-sidebar-open-icon" viewBox="0 0 16 16"><rect x="1.5" y="2.5" width="13" height="11" rx="1.5"/><rect className="sidebar-pane-fill" x="2.5" y="3.5" width="3.5" height="9" rx=".75"/><path d="M6.5 3v10"/></svg><svg className="left-sidebar-closed-icon" viewBox="0 0 16 16"><rect x="1.5" y="2.5" width="13" height="11" rx="1.5"/><path d="M6.5 3v10"/></svg></button></div>
    <div className="tree-toolbar"><input className="tree-search" type="search" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索数据源、FSM 或状态" aria-label="搜索 Replay 数据源和 FSM" /></div>
    <div className="fsm-list">{visibleSources.map(({ source, machines }) => <details className="folder-node replay-source-node" key={source.id} open={stored(`replay-source:${source.id}`, 'open') !== 'closed'} onToggle={(event) => remember(`replay-source:${source.id}`, event.currentTarget.open ? 'open' : 'closed')}><summary title={source.location || source.name}><span className="folder-chevron">›</span><span className={`source-status${source.lastError ? ' error' : ''}`} /><span>{source.name || source.id}</span><span className="source-driver">{source.driver}</span><span className="folder-count">{machines.length}</span></summary><div className="folder-content">{machines.map((item) => <MachineButton key={item.id} item={item} selected={selectedSource === source.id && selectedFSM === item.id} onSelect={(id) => onSelect(source.id, id)} />)}{!machines.length && <div className="source-empty">未发现 FSM</div>}</div></details>)}{!visibleSources.length && <div className="available-empty">没有匹配的数据源或 FSM</div>}</div>
  </aside>
}

export default function ReplayPage({ sidebarCollapsed, onSidebarToggle, payloadOpen, onPayloadToggle }) {
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
      try { return [item.id, await api.replayRecords(item.id)] } catch (error) { console.error(error); return [item.id, []] }
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
    setSource(nextSource?.id || ''); setSelectedFSM(nextSource ? machinesBySource[nextSource.id]?.[0]?.id || '' : '')
  }, [machinesBySource, selectedFSM, source, sources])
  const records = useMemo(() => (recordsBySource[source] || []).filter((item) => item.fsmId === selectedFSM), [recordsBySource, selectedFSM, source])
  const traces = useMemo(() => records.map((record) => ({ ...(record.trace || {}), replayRecordId: record.id,
    transitionId: record.trace?.transitionId || record.transitionId || record.id, event: record.trace?.event || record.event || '',
    finalState: record.trace?.finalState || record.state || '', success: record.trace?.transitionId || record.trace?.event ? record.trace.success : record.success,
    beginTime: record.trace?.beginTime || new Date(record.recordedAt).getTime() * 1000000 })), [records])
  useEffect(() => {
    let cancelled = false
    if (!source || !selectedFSM || !records.length) { setSelected(null); return () => { cancelled = true } }
    if (selected?.sourceId === source && selected?.fsmId === selectedFSM && records.some((item) => item.id === selected.id)) return () => { cancelled = true }
    api.replayRecord(source, records[0].id).then((record) => { if (!cancelled) setSelected(record) }).catch(console.error)
    return () => { cancelled = true }
  }, [records, selected?.fsmId, selected?.id, selected?.sourceId, selectedFSM, source])
  useEffect(() => {
    if (!playing || records.length < 2) return undefined
    const ordered = [...records].reverse(); let index = Math.max(0, ordered.findIndex((item) => item.id === selected?.id))
    const timer = setInterval(async () => { index = (index + 1) % ordered.length; setSelected(await api.replayRecord(source, ordered[index].id)) }, 1000)
    return () => clearInterval(timer)
  }, [playing, records, selected?.id, source])
  const chooseRecord = async (record) => { setPlaying(false); setSelected(await api.replayRecord(source, record.id)) }
  const removeSelected = async () => {
    if (!selected || !window.confirm('删除当前回放记录？')) return
    setPlaying(false); await api.deleteReplayRecord(source, selected.id); setSelected(null); await load()
  }
  const selectMachine = (sourceID, fsmID) => { setPlaying(false); setSelected(null); setSource(sourceID); setSelectedFSM(fsmID) }
  const selectTrace = (trace) => { const record = records.find((item) => item.id === trace.replayRecordId); if (record) chooseRecord(record).catch(console.error) }
  return <><ReplaySidebar sources={sources} machinesBySource={machinesBySource} selectedSource={source} selectedFSM={selectedFSM} onSelect={selectMachine} onRefresh={() => load().catch(console.error)} collapsed={sidebarCollapsed} onToggle={onSidebarToggle} />
    <main className="replay-page"><section className="replay-stage">{selected ? <><div className="replay-controls"><button type="button" disabled={records.length < 2} onClick={() => setPlaying((value) => !value)}>{playing ? '暂停' : '播放'}</button><span>{selected.snapshot.previousState || '-'} -- {selected.snapshot.previousEvent || '-'} → {selected.snapshot.currentState}</span><button type="button" onClick={() => load().catch(console.error)}>刷新</button><button className="replay-delete" type="button" onClick={removeSelected}>删除</button></div><div className="replay-graph"><StateGraph item={selected.snapshot}/></div></> : <div className="replay-empty">请选择数据源中的 FSM</div>}</section><TracePanel traces={traces} selectedId={selected?.id || ''} onSelect={selectTrace} /></main>
    <RightSidebar item={selected?.snapshot} payloadOpen={payloadOpen} onPayloadToggle={onPayloadToggle} /></>
}
