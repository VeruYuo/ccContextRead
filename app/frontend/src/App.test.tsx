import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { StatusInfo, UpdateEvent } from './api'

// PLAN.md T1.13 fixes three coupled bugs, all covered here against the real
// App component (mocking only the api/index.ts adapter, per layering rule
// 2): (1) Markdown state must survive a 预览<->设置 tab round-trip, so it
// has to live above Preview's own component lifecycle; (2) selecting a
// session must fetch a snapshot via getCurrentDocument() rather than only
// waiting for the next session:updated event, which may have already fired
// before the frontend was listening; (3) a follow-mode auto-switch
// (follow:changed) must trigger the same backfill.
const {
  listSessions,
  getStatus,
  getCurrentDocument,
  getConfig,
  startWatching,
  stopWatching,
  setFollowActive,
  sessionUpdatedHandlers,
  errorHandlers,
  followChangedHandlers,
  sessionsChangedHandlers,
} = vi.hoisted(() => {
  return {
    listSessions: vi.fn(),
    getStatus: vi.fn(),
    getCurrentDocument: vi.fn(),
    getConfig: vi.fn(),
    startWatching: vi.fn(),
    stopWatching: vi.fn(),
    setFollowActive: vi.fn(),
    sessionUpdatedHandlers: [] as Array<(ev: UpdateEvent) => void>,
    errorHandlers: [] as Array<(ev: { SessionID: string; Message: string }) => void>,
    followChangedHandlers: [] as Array<(sessionID: string) => void>,
    sessionsChangedHandlers: [] as Array<() => void>,
  }
})

vi.mock('./api', async () => {
  const actual = await vi.importActual<typeof import('./api')>('./api')
  return {
    ...actual,
    listSessions,
    getStatus,
    getCurrentDocument,
    getConfig,
    startWatching,
    stopWatching,
    setFollowActive,
    // Each hook mock keeps only the latest handler per calling component
    // (matching the real hooks' single stable subscription with a ref
    // holding the current closure) rather than accumulating one entry per
    // render, which would replay stale closures and double-fire effects.
    useSessionUpdated: (h: (ev: UpdateEvent) => void) => {
      sessionUpdatedHandlers[0] = h
    },
    useError: (h: (ev: { SessionID: string; Message: string }) => void) => {
      errorHandlers[0] = h
    },
    useFollowChanged: (h: (sessionID: string) => void) => {
      followChangedHandlers[0] = h
    },
    useSessionsChanged: (h: () => void) => {
      sessionsChangedHandlers[0] = h
    },
  }
})

import App from './App'

function emitSessionUpdated(ev: UpdateEvent) {
  act(() => {
    sessionUpdatedHandlers.forEach((h) => h(ev))
  })
}

function emitFollowChanged(sessionID: string) {
  act(() => {
    followChangedHandlers.forEach((h) => h(sessionID))
  })
}

function makeUpdateEvent(overrides: Partial<UpdateEvent> = {}): UpdateEvent {
  return {
    SessionID: 'sess1',
    Markdown: '# hello world',
    OutputPath: 'D:/out/sess1.md',
    EventCount: 1,
    Full: true,
    ...overrides,
  }
}

function makeStatus(overrides: Partial<StatusInfo> = {}): StatusInfo {
  return {
    Watching: false,
    SessionID: '',
    OutputPath: '',
    EventCount: 0,
    LastUpdatedAt: '',
    ...overrides,
  }
}

beforeEach(() => {
  sessionUpdatedHandlers.length = 0
  errorHandlers.length = 0
  followChangedHandlers.length = 0
  sessionsChangedHandlers.length = 0

  listSessions.mockReset().mockResolvedValue([
    { SessionID: 'sess1', ProjectDir: 'D:/p', ProjectName: 'p', Title: 'Sess One', Path: 'x', CreatedAt: '', LastActiveAt: '', IsActive: true },
  ])
  getStatus.mockReset().mockResolvedValue(makeStatus())
  getCurrentDocument.mockReset().mockResolvedValue({ Event: makeUpdateEvent(), Ok: false })
  getConfig.mockReset().mockResolvedValue({
    filter: {
      userPrompt: true,
      assistantText: true,
      thinking: false,
      toolUse: false,
      toolResult: false,
      subagent: false,
      contextInjection: false,
      systemNote: false,
      compactSummary: false,
      fileChange: 0,
      truncateChars: 2000,
    },
    imageMode: 0,
    outputDirOverride: '',
    fallbackApplied: false,
    resolvedOutputDir: 'D:\\ccContextRead',
    theme: 'system',
  })
  startWatching.mockReset().mockResolvedValue(undefined)
  stopWatching.mockReset().mockResolvedValue(undefined)
  setFollowActive.mockReset().mockResolvedValue(undefined)
})

describe('App tab switching', () => {
  it('keeps the preview content visible after switching to settings and back', async () => {
    const user = userEvent.setup()
    render(<App />)

    await screen.findByText('从左侧选择一个会话开始监听')
    await user.click(await screen.findByText('Sess One'))

    emitSessionUpdated(makeUpdateEvent({ Markdown: '# live content' }))

    await waitFor(() => expect(screen.getByText('live content')).toBeInTheDocument())

    await user.click(screen.getByRole('button', { name: '设置' }))
    // The preview tab is hidden via CSS display:none rather than unmounted
    // (PLAN.md T1.13), so the element stays in the DOM but stops being
    // visible — toBeVisible, not toBeInTheDocument, is the right assertion.
    await waitFor(() => expect(screen.getByText('live content')).not.toBeVisible())

    await user.click(screen.getByRole('button', { name: '预览' }))
    expect(screen.getByText('live content')).toBeVisible()
  })
})

describe('App session selection', () => {
  it('backfills via getCurrentDocument when session:updated already fired before selectedId caught up', async () => {
    getCurrentDocument.mockResolvedValue({ Event: makeUpdateEvent({ Markdown: '# backfilled content' }), Ok: true })

    const user = userEvent.setup()
    render(<App />)

    await user.click(await screen.findByText('Sess One'))

    await waitFor(() => expect(screen.getByText('backfilled content')).toBeInTheDocument())
  })

  it('shows the waiting message when the snapshot fetch returns ok=false', async () => {
    getCurrentDocument.mockResolvedValue({ Event: makeUpdateEvent(), Ok: false })

    const user = userEvent.setup()
    render(<App />)

    await user.click(await screen.findByText('Sess One'))

    await waitFor(() => expect(screen.getByText('等待第一次写入…')).toBeInTheDocument())
  })

  it('backfills the new session document after a follow-mode auto-switch', async () => {
    getCurrentDocument.mockResolvedValue({ Event: makeUpdateEvent({ Markdown: '# followed content' }), Ok: true })

    render(<App />)
    await screen.findByText('从左侧选择一个会话开始监听')

    emitFollowChanged('sess1')

    await waitFor(() => expect(screen.getByText('followed content')).toBeInTheDocument())
  })
})

describe('App session switch race (PLAN.md 12.2.1 问题③)', () => {
  it('does not display a stale snapshot for a different session, and recovers on the next session:updated', async () => {
    listSessions.mockResolvedValue([
      { SessionID: 'sess1', ProjectDir: 'D:/p', ProjectName: 'p', Title: 'Sess One', Path: 'x', CreatedAt: '', LastActiveAt: '', IsActive: true },
      { SessionID: 'sess2', ProjectDir: 'D:/p', ProjectName: 'p', Title: 'Sess Two', Path: 'y', CreatedAt: '', LastActiveAt: '', IsActive: true },
    ])
    // Simulates the backend race in PLAN.md App.tsx:49: whichever session is
    // requested, the snapshot fetch resolves to the *previous* session's
    // document, as if StartWatching(sess2) hadn't caught up on the backend
    // side yet when the snapshot request landed.
    getCurrentDocument.mockResolvedValue({
      Event: makeUpdateEvent({ SessionID: 'sess1', Markdown: '# stale sess1 content' }),
      Ok: true,
    })

    const user = userEvent.setup()
    render(<App />)

    await user.click(await screen.findByText('Sess Two'))

    // The stale sess1 snapshot must never be shown while sess2 is selected.
    await waitFor(() => expect(screen.queryByText('stale sess1 content')).not.toBeInTheDocument())

    // A correct session:updated for sess2 must still get through — with the
    // old prev.SessionID-based guard, the stale doc above would have set
    // prev.SessionID to 'sess1', permanently rejecting every future sess2
    // event ("此后A的所有事件永远被拒" in PLAN.md).
    emitSessionUpdated(makeUpdateEvent({ SessionID: 'sess2', Markdown: '# live sess2 content' }))

    await waitFor(() => expect(screen.getByText('live sess2 content')).toBeInTheDocument())
  })
})

describe('App session list refresh', () => {
  it('refetches the session list when sessions:changed fires', async () => {
    render(<App />)
    await screen.findByText('Sess One')

    listSessions.mockResolvedValueOnce([
      { SessionID: 'sess1', ProjectDir: 'D:/p', ProjectName: 'p', Title: 'Sess One', Path: 'x', CreatedAt: '', LastActiveAt: '', IsActive: true },
      { SessionID: 'sess2', ProjectDir: 'D:/p2', ProjectName: 'p2', Title: 'New Session', Path: 'y', CreatedAt: '', LastActiveAt: '', IsActive: true },
    ])

    act(() => {
      sessionsChangedHandlers.forEach((h) => h())
    })

    await waitFor(() => expect(screen.getByText('New Session')).toBeInTheDocument())
  })
})
