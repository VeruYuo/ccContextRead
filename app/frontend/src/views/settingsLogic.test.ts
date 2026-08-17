import { describe, expect, it } from 'vitest'
import type { AppConfig } from '../api'
import { FileChangeMode, ImageMode } from '../api'
import {
  setFileChange,
  setImageMode,
  setOutputDirOverride,
  setTruncateChars,
  toggleFilterSwitch,
} from './settingsLogic'

function baseConfig(): AppConfig {
  return {
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
      fileChange: FileChangeMode.None,
      truncateChars: 2000,
    },
    imageMode: ImageMode.Placeholder,
    outputDirOverride: '',
    fallbackApplied: false,
    resolvedOutputDir: '',
  }
}

describe('toggleFilterSwitch', () => {
  it('flips exactly the named switch, leaving everything else untouched', () => {
    const cfg = baseConfig()
    const next = toggleFilterSwitch(cfg, 'toolUse')
    expect(next.filter.toolUse).toBe(true)
    expect(next.filter.userPrompt).toBe(true)
    expect(next.filter.assistantText).toBe(true)
    expect(next.filter.thinking).toBe(false)
    // original is untouched (pure function)
    expect(cfg.filter.toolUse).toBe(false)
  })

  it('toggles back off on a second call', () => {
    const cfg = baseConfig()
    const next = toggleFilterSwitch(toggleFilterSwitch(cfg, 'subagent'), 'subagent')
    expect(next.filter.subagent).toBe(false)
  })
})

describe('setFileChange', () => {
  it('is a three-way exclusive choice, not a toggle', () => {
    const cfg = baseConfig()
    const summary = setFileChange(cfg, FileChangeMode.Summary)
    expect(summary.filter.fileChange).toBe(FileChangeMode.Summary)
    const full = setFileChange(summary, FileChangeMode.Full)
    expect(full.filter.fileChange).toBe(FileChangeMode.Full)
    const none = setFileChange(full, FileChangeMode.None)
    expect(none.filter.fileChange).toBe(FileChangeMode.None)
  })
})

describe('setImageMode', () => {
  it('sets the top-level imageMode field', () => {
    const next = setImageMode(baseConfig(), ImageMode.Attachment)
    expect(next.imageMode).toBe(ImageMode.Attachment)
  })
})

describe('setTruncateChars', () => {
  it('parses a normal numeric string', () => {
    const next = setTruncateChars(baseConfig(), '500')
    expect(next.filter.truncateChars).toBe(500)
  })

  it('treats an empty input as 0 (disables truncation)', () => {
    const next = setTruncateChars(baseConfig(), '')
    expect(next.filter.truncateChars).toBe(0)
  })

  it('clamps a negative input to 0 rather than passing it through', () => {
    const next = setTruncateChars(baseConfig(), '-5')
    expect(next.filter.truncateChars).toBe(0)
  })
})

describe('setOutputDirOverride', () => {
  it('sets the override path verbatim', () => {
    const next = setOutputDirOverride(baseConfig(), 'D:\\out')
    expect(next.outputDirOverride).toBe('D:\\out')
  })

  it('clears back to auto-resolve with an empty string', () => {
    const withOverride = setOutputDirOverride(baseConfig(), 'D:\\out')
    const cleared = setOutputDirOverride(withOverride, '')
    expect(cleared.outputDirOverride).toBe('')
  })
})
