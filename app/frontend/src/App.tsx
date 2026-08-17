import { useEffect, useState } from 'react'
import './App.css'
import type { StatusInfo } from './api'
import { getStatus, setFollowActive, startWatching, stopWatching, useError, useFollowChanged, useSessionUpdated } from './api'
import Preview from './views/Preview'
import SessionList from './views/SessionList'
import Settings from './views/Settings'

type Tab = 'preview' | 'settings'

function App() {
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [tab, setTab] = useState<Tab>('preview')
  const [status, setStatus] = useState<StatusInfo | null>(null)
  const [follow, setFollow] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    getStatus().then((s) => {
      setStatus(s)
      if (s.Watching) setSelectedId(s.SessionID)
    })
  }, [])

  useSessionUpdated((ev) => {
    setStatus({
      Watching: true,
      SessionID: ev.SessionID,
      OutputPath: ev.OutputPath,
      EventCount: ev.EventCount,
      LastUpdatedAt: new Date().toISOString(),
    })
  })

  useError((ev) => {
    setError(ev.Message)
  })

  useFollowChanged((sessionID) => {
    setSelectedId(sessionID)
  })

  async function selectSession(id: string) {
    setError(null)
    setSelectedId(id)
    try {
      await startWatching(id)
    } catch (err) {
      setError(String(err))
    }
  }

  async function stopSession() {
    await stopWatching()
    setStatus(null)
  }

  async function toggleFollow() {
    const next = !follow
    setFollow(next)
    await setFollowActive(next)
  }

  return (
    <div id="app">
      <header className="status-bar">
        <span className="status-text">
          {status?.Watching ? `监听中：${status.SessionID}（已写入 ${status.EventCount} 条）` : '未监听'}
        </span>
        {status?.OutputPath && <span className="status-path">{status.OutputPath}</span>}
        {status?.Watching && (
          <button type="button" onClick={stopSession}>
            停止监听
          </button>
        )}
        <label className="follow-toggle">
          <input type="checkbox" checked={follow} onChange={toggleFollow} />
          跟随活跃会话
        </label>
        {error && <span className="status-error">{error}</span>}
      </header>
      <div className="layout">
        <aside>
          <SessionList selectedId={selectedId} onSelect={selectSession} />
        </aside>
        <main>
          <nav className="tabs">
            <button type="button" className={tab === 'preview' ? 'active' : ''} onClick={() => setTab('preview')}>
              预览
            </button>
            <button type="button" className={tab === 'settings' ? 'active' : ''} onClick={() => setTab('settings')}>
              设置
            </button>
          </nav>
          {tab === 'preview' ? <Preview selectedId={selectedId} /> : <Settings />}
        </main>
      </div>
    </div>
  )
}

export default App
