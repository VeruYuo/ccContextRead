import { renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

// useSessionUpdated/useError/useFollowChanged/useSessionsChanged all wrap
// EventsOn: PLAN.md T1.13's "附带缺陷" is that an inline arrow-function
// handler (a fresh reference every render) used to be passed straight as
// the useEffect dependency, causing every render to unsubscribe and
// resubscribe — a window during which events landing in between are lost.
// These tests assert the fix: subscription happens exactly once regardless
// of how many times the handler reference changes across re-renders, while
// the *latest* handler is still the one invoked.
const { EventsOn } = vi.hoisted(() => ({
  EventsOn: vi.fn((_event: string, _cb: (...data: unknown[]) => void) => vi.fn()),
}))

vi.mock('../../wailsjs/runtime/runtime', () => ({ EventsOn }))

import type { ErrorEvent, UpdateEvent } from './index'
import { useError, useFollowChanged, useSessionUpdated, useSessionsChanged } from './index'

beforeEach(() => {
  EventsOn.mockReset()
  EventsOn.mockImplementation(() => vi.fn())
})

describe('useSessionUpdated', () => {
  it('subscribes to EventsOn exactly once across re-renders', () => {
    const { rerender } = renderHook(({ h }: { h: (ev: UpdateEvent) => void }) => useSessionUpdated(h), {
      initialProps: { h: vi.fn() },
    })
    rerender({ h: vi.fn() })
    rerender({ h: vi.fn() })

    expect(EventsOn).toHaveBeenCalledTimes(1)
    expect(EventsOn).toHaveBeenCalledWith('session:updated', expect.any(Function))
  })

  it('always invokes the latest handler passed in, not the first one', () => {
    let capturedCallback: (data: unknown) => void = () => {}
    EventsOn.mockImplementation((_event: string, cb: (...data: unknown[]) => void) => {
      capturedCallback = cb
      return vi.fn()
    })

    const first = vi.fn()
    const second = vi.fn()
    const { rerender } = renderHook(({ h }: { h: (ev: UpdateEvent) => void }) => useSessionUpdated(h), {
      initialProps: { h: first },
    })
    rerender({ h: second })

    capturedCallback({ SessionID: 'sess1' })

    expect(first).not.toHaveBeenCalled()
    expect(second).toHaveBeenCalledWith({ SessionID: 'sess1' })
  })
})

describe('useError', () => {
  it('subscribes to EventsOn exactly once across re-renders', () => {
    const { rerender } = renderHook(({ h }: { h: (ev: ErrorEvent) => void }) => useError(h), {
      initialProps: { h: vi.fn() },
    })
    rerender({ h: vi.fn() })

    expect(EventsOn).toHaveBeenCalledTimes(1)
    expect(EventsOn).toHaveBeenCalledWith('error', expect.any(Function))
  })
})

describe('useFollowChanged', () => {
  it('subscribes to EventsOn exactly once across re-renders', () => {
    const { rerender } = renderHook(({ h }: { h: (id: string) => void }) => useFollowChanged(h), {
      initialProps: { h: vi.fn() },
    })
    rerender({ h: vi.fn() })

    expect(EventsOn).toHaveBeenCalledTimes(1)
    expect(EventsOn).toHaveBeenCalledWith('follow:changed', expect.any(Function))
  })
})

describe('useSessionsChanged', () => {
  it('subscribes to EventsOn exactly once across re-renders', () => {
    const { rerender } = renderHook(({ h }: { h: () => void }) => useSessionsChanged(h), {
      initialProps: { h: vi.fn() },
    })
    rerender({ h: vi.fn() })

    expect(EventsOn).toHaveBeenCalledTimes(1)
    expect(EventsOn).toHaveBeenCalledWith('sessions:changed', expect.any(Function))
  })
})
