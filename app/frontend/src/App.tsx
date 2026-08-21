import { useCallback, useEffect, useRef, useState } from 'react'
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
  // Mirrors selectedId synchronously (state updates are batched/async, but
  // this ref is written in the same tick as every setSelectedId call) so
  // the session:updated handler and fetchSnapshot's stale-response check
  // always compare against the *current* selection, not a value captured
  // in a stale closure (PLAN.md 12.2.1 问题③, App.tsx:66 的守卫基准).
  const selectedIdRef = useRef<string | null>(null)
  const [tab, setTab] = useState<Tab>('preview')
  const [status, setStatus] = useState<StatusInfo | null>(null)
  const [follow, setFollow] = useState(false)
  const [error, setError] = useState<string | null>(null)
  // Markdown state lives here, above Preview, so switching tabs (which no
  // longer unmounts Preview, see below) or re-selecting the same session
  // doesn't lose it (PLAN.md T1.13 现象一).
  const [doc, setDoc] = useState<UpdateEvent | null>(null)
  // Discards out-of-order fetchSnapshot responses (two snapshot fetches for
  // the same id, resolving in reverse order) independent of the id check.
  const fetchGenerationRef = useRef(0)

  const selectId = useCallback((id: string | null) => {
    selectedIdRef.current = id
    setSelectedId(id)
  }, [])

  // Fetches a snapshot for id and applies it only if it's both the latest
  // fetch issued and still the user's current selection — otherwise a
  // response for a session the backend hasn't finished switching to (or
  // that arrives after the user has already moved on) would clobber newer
  // state (PLAN.md 12.2.1 问题③).
  const fetchSnapshot = useCallback((id: string) => {
    const generation = ++fetchGenerationRef.current
    getCurrentDocument(id).then(({ Event, Ok }) => {
      if (generation !== fetchGenerationRef.current) return
      if (selectedIdRef.current !== id) return
      setDoc(Ok && Event.SessionID === id ? Event : null)
    })
  }, [])

  useEffect(() => {
    getStatus().then((s) => {
      setStatus(s)
      if (s.Watching) {
        selectId(s.SessionID)
        fetchSnapshot(s.SessionID)
      }
    })
  }, [selectId, fetchSnapshot])

  useSessionUpdated((ev) => {
    if (ev.SessionID !== selectedIdRef.current) return
    setStatus({
      Watching: true,
      SessionID: ev.SessionID,
      OutputPath: ev.OutputPath,
      EventCount: ev.EventCount,
      LastUpdatedAt: new Date().toISOString(),
    })
    setDoc(ev)
  })

  useError((ev) => {
    setError(ev.Message)
  })

  useFollowChanged((sessionID) => {
    // The backend has already switched to sessionID by the time this event
    // fires (Service.tickFollow calls StartWatching before emitting
    // follow:changed), so no await-startWatching step is needed here — but
    // the previous session's doc must still be cleared immediately so it
    // isn't shown under the new selection while the snapshot is in flight.
    selectId(sessionID)
    setDoc(null)
    fetchSnapshot(sessionID)
  })

  async function selectSession(id: string) {
    setError(null)
    selectId(id)
    // Clear immediately: otherwise the previous session's content stays on
    // screen — under the new selection — until the snapshot fetch below
    // resolves (PLAN.md 12.2.1 问题③ 预览正文与选中不一致).
    setDoc(null)
    try {
      // Wait for the backend to actually finish switching before asking it
      // for a snapshot — requesting one concurrently with StartWatching
      // used to race and could return the *previous* session's document
      // (PLAN.md App.tsx:49/12.2.1 问题③).
      await startWatching(id)
      fetchSnapshot(id)
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
