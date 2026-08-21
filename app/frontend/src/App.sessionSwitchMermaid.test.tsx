import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { StatusInfo, UpdateEvent } from './api'

// Real mermaid + real markdownRender run here (only api/index.ts is
// mocked) — this is the only kind of test that would have caught this:
// a stubbed renderMermaidBlocks can't reproduce a real container getting
// replaced out from under an in-flight mermaid.render() call, exactly the
// class of bug PLAN.md 12.2.1 问题④ warns about (S5d shipped green with
// every test stubbed).
//
// Reported 2026-08-21: after S5f's session-switch race fix, a session's
// mermaid diagram that had already rendered successfully would degrade to
// the raw code block once you switched to a different session and back.
// Root cause: StartWatching's synchronous initial runPass emits
// session:updated *before* the StartWatching RPC call returns, so
// selectSession's `await startWatching(id)` and the event handler's
// `setDoc(ev)` race — the event applies the document first, then
// fetchSnapshot's later, independent getCurrentDocument(id) call applies
// what is content-identical data again. Two `setDoc` calls for the exact
// same content still make Preview replace the container's DOM twice
// (dangerouslySetInnerHTML always receives a fresh {__html} object, so the
// second replacement swaps out the placeholder node mermaid's first
// render() call was still working on) — the render finishes into a
// now-detached node, invisible on screen, permanently stuck on the
// fallback code block. Fixed by comparing content in the functional setDoc
// updater and returning the same reference when nothing actually changed,
// so React bails out of the second, redundant re-render entirely.
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

function makeStatus(overrides: Partial<StatusInfo> = {}): StatusInfo {
  return { Watching: false, SessionID: '', OutputPath: '', EventCount: 0, LastUpdatedAt: '', ...overrides }
}

// Controllable promise so the test can fire session:updated *while*
// startWatching is still pending, matching the real backend timing
// (orch.Start's initial runPass emits before the RPC call returns).
function deferred<T>() {
  let resolve!: (v: T) => void
  const promise = new Promise<T>((res) => {
    resolve = res
  })
  return { promise, resolve }
}

const MD_A = '# Session A\n\n```mermaid\nflowchart TD\n  A[Session A Node] --> B[End]\n```\n'
const MD_B = '# Session B\n\n```mermaid\nflowchart TD\n  X[Session B Node] --> Y[End]\n```\n'

beforeEach(() => {
  sessionUpdatedHandlers.length = 0
  errorHandlers.length = 0
  followChangedHandlers.length = 0
  sessionsChangedHandlers.length = 0

  listSessions.mockReset().mockResolvedValue([
    { SessionID: 'sessA', ProjectDir: 'D:/p', ProjectName: 'p', Title: 'Sess A', Path: 'x', CreatedAt: '', LastActiveAt: '', IsActive: true },
    { SessionID: 'sessB', ProjectDir: 'D:/p', ProjectName: 'p', Title: 'Sess B', Path: 'y', CreatedAt: '', LastActiveAt: '', IsActive: true },
  ])
  getStatus.mockReset().mockResolvedValue(makeStatus())
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
  stopWatching.mockReset().mockResolvedValue(undefined)
  setFollowActive.mockReset().mockResolvedValue(undefined)
})

// selectSessionRacingBackend drives the exact sequence that reproduces the
// race: click the session, let session:updated land first (as the real
// backend does), then resolve startWatching and getCurrentDocument with
// content-identical data for the same session.
function makeUpdateEvent(sessionID: string, markdown: string): UpdateEvent {
  return { SessionID: sessionID, Markdown: markdown, OutputPath: `${sessionID}.md`, EventCount: 1, Full: true }
}

async function selectSessionRacingBackend(user: ReturnType<typeof userEvent.setup>, sessionID: string, title: string, markdown: string) {
  const sw = deferred<void>()
  startWatching.mockImplementation(() => sw.promise)
  const gcd = deferred<{ Event: UpdateEvent; Ok: boolean }>()
  getCurrentDocument.mockImplementation(() => gcd.promise)

  await user.click(await screen.findByText(title))
  // Two independent object literals with equal content, deliberately not
  // the same reference: the real backend never reuses a JS object between
  // an EventsOn payload and an RPC response — both go through separate
  // JSON (de)serialization over the Wails bridge, so relying on
  // React's same-*reference* useState bailout here would test something
  // that can't happen in production and would mask this exact bug.
  emitSessionUpdated(makeUpdateEvent(sessionID, markdown))
  await act(async () => {
    sw.resolve()
    await Promise.resolve()
  })
  await act(async () => {
    gcd.resolve({ Event: makeUpdateEvent(sessionID, markdown), Ok: true })
    await Promise.resolve()
  })
}

describe('App + real mermaid: session switch does not degrade a rendered diagram to a code block', () => {
  it('still shows the diagram after switching away and back to a previously-rendered session', async () => {
    const user = userEvent.setup()
    render(<App />)

    await selectSessionRacingBackend(user, 'sessA', 'Sess A', MD_A)
    await waitFor(() => expect(document.body.textContent).toContain('Session A Node'), { timeout: 15000 })
    await waitFor(() => expect(document.querySelector('.mermaid-fallback')).toBeNull(), { timeout: 15000 })
    expect(document.querySelector('svg')).not.toBeNull()

    await selectSessionRacingBackend(user, 'sessB', 'Sess B', MD_B)
    await waitFor(() => expect(document.body.textContent).toContain('Session B Node'), { timeout: 15000 })

    await selectSessionRacingBackend(user, 'sessA', 'Sess A', MD_A)
    await waitFor(() => expect(document.body.textContent).toContain('Session A Node'), { timeout: 15000 })

    // The regression: this used to stay a raw fallback code block forever
    // (svgCache had the diagram cached, but the node holding it was
    // detached, so the visible container's placeholder was never restored).
    await waitFor(() => expect(document.querySelector('.mermaid-fallback')).toBeNull(), { timeout: 15000 })
    expect(document.querySelector('svg')).not.toBeNull()
  }, 30000)
})
