import { useEffect, useState } from 'react'
import type { SessionSummary } from '../api'
import { listSessions, useSessionsChanged } from '../api'

// SESSION_LIST_POLL_INTERVAL_MS is the polling fallback for the session
// list, matching 4.5's "fsnotify 会漏事件" double-channel approach: the
// sessions:changed event drives immediate refreshes, and polling catches
// whatever it misses (e.g. a brand-new session created by a process this
// app never watched, so StartWatching never fired the event).
const SESSION_LIST_POLL_INTERVAL_MS = 5000

interface Props {
  selectedId: string | null
  onSelect: (id: string) => void
}

export default function SessionList({ selectedId, onSelect }: Props) {
  const [sessions, setSessions] = useState<SessionSummary[]>([])
  const [loading, setLoading] = useState(true)

  function refresh() {
    listSessions()
      .then(setSessions)
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    refresh()
    const id = setInterval(refresh, SESSION_LIST_POLL_INTERVAL_MS)
    return () => clearInterval(id)
  }, [])

  useSessionsChanged(refresh)

  if (loading) {
    return <div className="session-list-empty">加载会话列表…</div>
  }
  if (sessions.length === 0) {
    return <div className="session-list-empty">未发现任何会话</div>
  }

  return (
    <ul className="session-list">
      {sessions.map((s) => (
        <li key={s.SessionID}>
          <button
            type="button"
            className={s.SessionID === selectedId ? 'session-item selected' : 'session-item'}
            onClick={() => onSelect(s.SessionID)}
          >
            <span className={s.IsActive ? 'active-dot' : 'active-dot inactive'} title={s.IsActive ? '活跃中' : ''} />
            <span className="session-project">{s.ProjectName}</span>
            <span className="session-title">{s.Title}</span>
            <span className="session-time">{formatTime(s.LastActiveAt)}</span>
          </button>
        </li>
      ))}
    </ul>
  )
}

function formatTime(iso: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleString()
}
