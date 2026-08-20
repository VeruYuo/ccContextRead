import { afterEach, describe, expect, it, vi } from 'vitest'
import { ThemeMode } from './api'
import { applyTheme, resolveTheme } from './theme'

afterEach(() => {
  vi.unstubAllGlobals()
  document.documentElement.removeAttribute('data-theme')
})

describe('resolveTheme (pure system-preference resolution, PLAN.md 12.2.1 ⑤)', () => {
  it('passes an explicit light/dark choice straight through, ignoring the OS preference', () => {
    expect(resolveTheme(ThemeMode.Light, true)).toBe('light')
    expect(resolveTheme(ThemeMode.Dark, false)).toBe('dark')
  })

  it('resolves "system" using the OS prefers-dark flag', () => {
    expect(resolveTheme(ThemeMode.System, true)).toBe('dark')
    expect(resolveTheme(ThemeMode.System, false)).toBe('light')
  })
})

describe('applyTheme (writes document.documentElement.dataset.theme)', () => {
  it('writes the resolved theme onto the document root', () => {
    vi.stubGlobal('matchMedia', vi.fn().mockReturnValue({ matches: true, addEventListener: vi.fn(), removeEventListener: vi.fn() }))

    applyTheme(ThemeMode.System)

    expect(document.documentElement.dataset.theme).toBe('dark')
  })

  it('an explicit choice overrides whatever the OS preference is', () => {
    vi.stubGlobal('matchMedia', vi.fn().mockReturnValue({ matches: true, addEventListener: vi.fn(), removeEventListener: vi.fn() }))

    applyTheme(ThemeMode.Light)

    expect(document.documentElement.dataset.theme).toBe('light')
  })
})
