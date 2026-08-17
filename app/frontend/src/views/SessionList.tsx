import { useEffect, useState } from 'react'
import type { SessionSummary } from '../api'
import { listSessions } from '../api'

interface Props {
  selectedId: string | null
  onSelect: (id: string) => void
}

export default function SessionList({ selectedId, onSelect }: Props) {
  const [sessions, setSessions] = useState<SessionSummary[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    listSessions()
      .then(setSessions)
      .finally(() => setLoading(false))
  }, [])

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
