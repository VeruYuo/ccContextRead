// Pure state-transition helpers for Settings.tsx, kept separate from the
// component so they can be unit-tested without touching the DOM.
import type { AppConfig, FileChangeMode, FilterConfig, ImageMode } from '../api'

export function toggleFilterSwitch(cfg: AppConfig, key: keyof Omit<FilterConfig, 'fileChange' | 'truncateChars'>): AppConfig {
  return { ...cfg, filter: { ...cfg.filter, [key]: !cfg.filter[key] } }
}

export function setFileChange(cfg: AppConfig, mode: FileChangeMode): AppConfig {
  return { ...cfg, filter: { ...cfg.filter, fileChange: mode } }
}

export function setImageMode(cfg: AppConfig, mode: ImageMode): AppConfig {
  return { ...cfg, imageMode: mode }
}

export function setTruncateChars(cfg: AppConfig, raw: string): AppConfig {
  const n = Number(raw)
  const truncateChars = Number.isFinite(n) && n > 0 ? Math.trunc(n) : 0
  return { ...cfg, filter: { ...cfg.filter, truncateChars } }
}

export function setOutputDirOverride(cfg: AppConfig, dir: string): AppConfig {
  return { ...cfg, outputDirOverride: dir }
}
