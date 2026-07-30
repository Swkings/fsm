import { useCallback, useEffect, useRef, useState } from 'react'
import { api } from './api.js'
import StateGraph from './StateGraph.jsx'
import Sidebar from './components/Sidebar.jsx'
import ReplayPage from './features/replay/ReplayPage.jsx'
import ActiveEventPanel from './features/visualization/ActiveEventPanel.jsx'
import RightSidebar from './features/visualization/RightSidebar.jsx'
import TracePanel from './features/visualization/TracePanel.jsx'
import { machineName } from './lib/machines.js'
import { remember, stored } from './lib/storage.js'

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
