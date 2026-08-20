import { useEffect, useState } from 'react'
import './App.css'
import type { StatusInfo, UpdateEvent } from './api'
import {
  getCurrentDocument,
  getStatus,
  setFollowActive,
  startWatching,
  stopWatching,
  useError,
  useFollowChanged,
  useSessionUpdated,
} from './api'
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
  // Markdown state lives here, above Preview, so switching tabs (which no
  // longer unmounts Preview, see below) or re-selecting the same session
  // doesn't lose it (PLAN.md T1.13 现象一).
  const [doc, setDoc] = useState<UpdateEvent | null>(null)

  useEffect(() => {
    getStatus().then((s) => {
      setStatus(s)
      if (s.Watching) setSelectedId(s.SessionID)
    })
  }, [])

  // Whenever the selected session changes, fetch a snapshot immediately
  // rather than only waiting for the next session:updated event — that
  // event may already have fired (e.g. StartWatching's initial synchronous
  // pass) before selectedId caught up, and there was previously no way to
  // recover it (PLAN.md T1.13 现象二).
  useEffect(() => {
    if (!selectedId) {
      setDoc(null)
      return
    }
    let cancelled = false
    getCurrentDocument().then(({ Event, Ok }) => {
      if (cancelled) return
      setDoc(Ok ? Event : null)
    })
    return () => {
      cancelled = true
    }
  }, [selectedId])

  useSessionUpdated((ev) => {
    setStatus({
      Watching: true,
      SessionID: ev.SessionID,
      OutputPath: ev.OutputPath,
      EventCount: ev.EventCount,
      LastUpdatedAt: new Date().toISOString(),
    })
    setDoc((prev) => (ev.SessionID === (prev?.SessionID ?? selectedId) ? ev : prev))
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
    setDoc(null)
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
          <div style={{ display: tab === 'preview' ? 'contents' : 'none' }}>
            <Preview selectedId={selectedId} doc={doc} visible={tab === 'preview'} />
          </div>
          <div style={{ display: tab === 'settings' ? 'contents' : 'none' }}>
            <Settings />
          </div>
        </main>
      </div>
    </div>
  )
}

export default App
