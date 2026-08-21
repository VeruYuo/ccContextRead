// The single IPC adapter layer (PLAN.md 5, layering rule 2): every call
// into the Go backend and every event subscription goes through this
// file. No other module may import from "../../wailsjs/..." directly —
// that keeps a future Wails -> Tauri/Electron migration to a rewrite of
// this one file.
import { useEffect, useRef } from 'react'
import * as Backend from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'

// ---- Types (mirror the Go DTOs in internal/app, internal/config, internal/model) ----

export const FileChangeMode = {
  None: 0,
  Summary: 1,
  Full: 2,
} as const
export type FileChangeMode = (typeof FileChangeMode)[keyof typeof FileChangeMode]

export const ImageMode = {
  Placeholder: 0,
  Attachment: 1,
} as const
export type ImageMode = (typeof ImageMode)[keyof typeof ImageMode]

// internal/config.Theme (json-tagged, string enum on the wire).
export const ThemeMode = {
  Light: 'light',
  Dark: 'dark',
  System: 'system',
} as const
export type ThemeMode = (typeof ThemeMode)[keyof typeof ThemeMode]

// internal/model.FilterConfig (json-tagged camelCase).
export interface FilterConfig {
  userPrompt: boolean
  assistantText: boolean
  thinking: boolean
  toolUse: boolean
  toolResult: boolean
  subagent: boolean
  contextInjection: boolean
  systemNote: boolean
  compactSummary: boolean
  fileChange: FileChangeMode
  truncateChars: number
}

// internal/config.Config (json-tagged camelCase).
export interface AppConfig {
  filter: FilterConfig
  imageMode: ImageMode
  outputDirOverride: string
  fallbackApplied: boolean
  resolvedOutputDir: string
  theme: ThemeMode
}

// internal/app.SessionSummary (no json tags -> PascalCase on the wire).
export interface SessionSummary {
  SessionID: string
  ProjectDir: string
  ProjectName: string
  Title: string
  Path: string
  CreatedAt: string
  LastActiveAt: string
  IsActive: boolean
}

// internal/app.StatusInfo.
export interface StatusInfo {
  Watching: boolean
  SessionID: string
  OutputPath: string
  EventCount: number
  LastUpdatedAt: string
}

// internal/claude.LiveSession.
export interface LiveSessionInfo {
  PID: number
  SessionID: string
  Cwd: string
  Kind: string
  Status: string
  Entrypoint: string
  Name: string
  NameSource: string
  JobID: string
  StartedAt: string
  UpdatedAt: string
}

// main.OutputDirInfo.
export interface OutputDirInfo {
  Dir: string
  FellBack: boolean
}

// internal/app.UpdateEvent ("session:updated" payload).
export interface UpdateEvent {
  SessionID: string
  Markdown: string
  OutputPath: string
  EventCount: number
  Full: boolean
}

// main.CurrentDocumentResult (GetDocument's return value). Wrapped in a
// struct rather than a (UpdateEvent, bool) Go return because Wails'
// generated binding only preserves a second return value when it's an
// `error` — any other type is silently dropped on the wire.
export interface CurrentDocumentResult {
  Event: UpdateEvent
  Ok: boolean
}

// internal/app.ErrorEvent ("error" payload).
export interface ErrorEvent {
  SessionID: string
  Message: string
}

// ---- Backend calls ----

export function listSessions(): Promise<SessionSummary[]> {
  return Backend.ListSessions()
}

// The generated config.Config binding types fileChange as a plain number
// and requires a class instance (with a convertValues method) on the way
// in, neither of which matches our narrower, plain-object AppConfig — the
// wire format is identical, so this is a type-level bridge only.
export function getConfig(): Promise<AppConfig> {
  return Backend.GetConfig() as unknown as Promise<AppConfig>
}

export function saveConfig(cfg: AppConfig): Promise<void> {
  return Backend.SaveConfig(cfg as unknown as Parameters<typeof Backend.SaveConfig>[0])
}

export function effectiveOutputDir(): Promise<OutputDirInfo> {
  return Backend.EffectiveOutputDir()
}

export function chooseOutputDir(): Promise<string> {
  return Backend.ChooseOutputDir()
}

export function startWatching(sessionID: string): Promise<void> {
  return Backend.StartWatching(sessionID)
}

export function stopWatching(): Promise<void> {
  return Backend.StopWatching()
}

export function getStatus(): Promise<StatusInfo> {
  return Backend.GetStatus()
}

// getCurrentDocument fetches the snapshot for sessionID specifically — the
// backend rejects (Ok:false) if sessionID isn't the session it's currently
// watching, rather than silently returning whatever it has (PLAN.md
// 12.2.1 问题③ fix 4: the no-arg GetCurrentDocument was the root cause of
// the frontend/backend switch race, not just a frontend guard problem).
export function getCurrentDocument(sessionID: string): Promise<CurrentDocumentResult> {
  return Backend.GetDocument(sessionID) as unknown as Promise<CurrentDocumentResult>
}

export function listLiveSessions(): Promise<LiveSessionInfo[]> {
  return Backend.ListLiveSessions()
}

export function setFollowActive(enabled: boolean): Promise<void> {
  return Backend.SetFollowActive(enabled)
}

// ---- Event subscriptions (hooks live here too, per layering rule 2) ----

export function onSessionUpdated(handler: (ev: UpdateEvent) => void): () => void {
  return EventsOn('session:updated', (ev: UpdateEvent) => handler(ev))
}

export function onError(handler: (ev: ErrorEvent) => void): () => void {
  return EventsOn('error', (ev: ErrorEvent) => handler(ev))
}

export function onFollowChanged(handler: (sessionID: string) => void): () => void {
  return EventsOn('follow:changed', (sessionID: string) => handler(sessionID))
}

export function onSessionsChanged(handler: () => void): () => void {
  return EventsOn('sessions:changed', () => handler())
}

// useLatestHandlerEffect subscribes once for the component's lifetime
// (empty deps array) rather than resubscribing on every render — a fresh
// inline arrow function is a new reference each render, so depending on
// `handler` directly used to unsubscribe/resubscribe constantly, and events
// arriving in that gap window were lost (PLAN.md T1.13 附带缺陷). A ref
// always holds the latest handler so the single, stable subscription still
// calls current logic.
function useLatestHandlerEffect<H>(subscribe: (call: (arg: H) => void) => () => void, handler: (arg: H) => void): void {
  const handlerRef = useRef(handler)
  handlerRef.current = handler
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => subscribe((arg) => handlerRef.current(arg)), [])
}

export function useSessionUpdated(handler: (ev: UpdateEvent) => void): void {
  useLatestHandlerEffect(onSessionUpdated, handler)
}

export function useError(handler: (ev: ErrorEvent) => void): void {
  useLatestHandlerEffect(onError, handler)
}

export function useFollowChanged(handler: (sessionID: string) => void): void {
  useLatestHandlerEffect(onFollowChanged, handler)
}

export function useSessionsChanged(handler: () => void): void {
  const handlerRef = useRef(handler)
  handlerRef.current = handler
  useEffect(() => onSessionsChanged(() => handlerRef.current()), [])
}
