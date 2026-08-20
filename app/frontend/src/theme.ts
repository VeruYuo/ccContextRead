import { ThemeMode } from './api'

export type ResolvedTheme = 'light' | 'dark'

// resolveTheme is the pure decision (PLAN.md 12.2.1 ⑤): an explicit
// light/dark choice always wins; "system" defers to the OS preference.
export function resolveTheme(theme: ThemeMode, prefersDark: boolean): ResolvedTheme {
  if (theme === ThemeMode.System) return prefersDark ? 'dark' : 'light'
  return theme
}

function prefersDark(): boolean {
  return typeof window !== 'undefined' && window.matchMedia?.('(prefers-color-scheme: dark)').matches === true
}

let systemListenerCleanup: (() => void) | null = null

// applyTheme writes the resolved theme onto <html data-theme="...">, which
// style.css keys off of instead of @media (prefers-color-scheme) directly —
// that's what makes a manual light/dark override possible at all. When the
// setting is "system", it also keeps listening for OS-level scheme changes
// so the app follows live instead of only resolving once at load time.
export function applyTheme(theme: ThemeMode): void {
  if (typeof document === 'undefined') return

  systemListenerCleanup?.()
  systemListenerCleanup = null

  document.documentElement.dataset.theme = resolveTheme(theme, prefersDark())

  if (theme !== ThemeMode.System || typeof window === 'undefined' || !window.matchMedia) return
  const mql = window.matchMedia('(prefers-color-scheme: dark)')
  const onChange = () => {
    document.documentElement.dataset.theme = resolveTheme(theme, mql.matches)
  }
  mql.addEventListener('change', onChange)
  systemListenerCleanup = () => mql.removeEventListener('change', onChange)
}
