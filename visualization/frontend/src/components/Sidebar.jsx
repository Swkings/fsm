import { useMemo, useState } from 'react'
import { buildFolderTree, compareMachines, folderSize, machineName } from '../lib/machines.js'
import { remember, stored } from '../lib/storage.js'

export function MachineButton({ item, selected, onSelect }) {
  return <button className={`fsm-button${selected ? ' active' : ''}`} type="button" onClick={() => onSelect(item.id)}>
    <span className="fsm-name">{machineName(item)}</span><span className="state-mini">{item.currentState || '—'}</span>
    <span className="fsm-meta"><i className={`mode-dot ${item.mode || 'push'}`} />{item.mode || 'push'} · {item.name || item.id}</span>
  </button>
}

function Folder({ node, selected, onSelect }) {
  const children = [...node.folders.values()].sort((a, b) => a.name.localeCompare(b.name, 'zh-CN'))
  return <details className="folder-node" data-path={node.path} open={stored(`fsm-folder:${node.path}`, 'open') !== 'closed'} onToggle={(event) => remember(`fsm-folder:${node.path}`, event.currentTarget.open ? 'open' : 'closed')}>
    <summary><span className="folder-chevron">›</span><span className="folder-icon">□</span><span>{node.name}</span><span className="folder-count">{folderSize(node)}</span></summary>
    <div className="folder-content">
      {[...node.items].sort(compareMachines).map((item) => <MachineButton key={item.id} item={item} selected={selected === item.id} onSelect={onSelect} />)}
      {children.map((child) => <Folder key={child.path} node={child} selected={selected} onSelect={onSelect} />)}
    </div>
  </details>
}

export default function Sidebar({ fsms, selected, onSelect, collapsed, onToggle, title = 'Machines', countLabel, searchPlaceholder = '搜索 FSM、状态或路径', emptyLabel = '尚未注册 FSM' }) {
  const [query, setQuery] = useState('')
  const visible = useMemo(() => {
    const needle = query.trim().toLocaleLowerCase()
    if (!needle) return fsms
    return fsms.filter((item) => [item.displayName, item.name, item.id, item.path, item.currentState, item.mode].some((value) => String(value || '').toLocaleLowerCase().includes(needle)))
  }, [fsms, query])
  const tree = useMemo(() => buildFolderTree(visible), [visible])
  return <aside className="left-sidebar">
    <div className="aside-heading"><span className="aside-title">{title}</span><span className="count-pill">{countLabel || `${fsms.length} registered`}</span>
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
}
