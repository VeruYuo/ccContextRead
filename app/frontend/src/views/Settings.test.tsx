import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AppConfig } from '../api'
import { FileChangeMode, ImageMode, ThemeMode } from '../api'
import Settings from './Settings'

const { getConfig, saveConfig, chooseOutputDir } = vi.hoisted(() => ({
  getConfig: vi.fn(),
  saveConfig: vi.fn(),
  chooseOutputDir: vi.fn(),
}))

vi.mock('../api', async () => {
  const actual = await vi.importActual<typeof import('../api')>('../api')
  return { ...actual, getConfig, saveConfig, chooseOutputDir }
})

function defaultConfig(): AppConfig {
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
    resolvedOutputDir: 'D:\\ccContextRead',
    theme: ThemeMode.System,
  }
}

beforeEach(() => {
  getConfig.mockReset().mockResolvedValue(defaultConfig())
  saveConfig.mockReset().mockResolvedValue(undefined)
  chooseOutputDir.mockReset().mockResolvedValue('')
})

describe('Settings', () => {
  it('reflects the loaded config in the checkbox states', async () => {
    render(<Settings />)

    expect(await screen.findByLabelText('用户输入')).toBeChecked()
    expect(screen.getByLabelText('助手最终回复')).toBeChecked()
    expect(screen.getByLabelText('工具调用（含参数）')).not.toBeChecked()
    expect(screen.getByLabelText('Compact 摘要正文')).not.toBeChecked()
  })

  it('immediately saves the full updated config when a switch is toggled', async () => {
    const user = userEvent.setup()
    render(<Settings />)

    await user.click(await screen.findByLabelText('工具调用（含参数）'))

    await waitFor(() => expect(saveConfig).toHaveBeenCalledTimes(1))
    const saved = saveConfig.mock.calls[0][0] as AppConfig
    expect(saved.filter.toolUse).toBe(true)
    // untouched defaults must still be present, not just the changed field.
    expect(saved.filter.userPrompt).toBe(true)
    expect(saved.filter.assistantText).toBe(true)
  })

  it('keeps the file-change radio group mutually exclusive', async () => {
    const user = userEvent.setup()
    render(<Settings />)

    const none = await screen.findByLabelText('不记录')
    const summary = screen.getByLabelText('仅路径与操作')
    const full = screen.getByLabelText('完整 diff')

    expect(none).toBeChecked()

    await user.click(summary)
    await waitFor(() => expect(summary).toBeChecked())
    expect(none).not.toBeChecked()
    expect(full).not.toBeChecked()

    await user.click(full)
    await waitFor(() => expect(full).toBeChecked())
    expect(summary).not.toBeChecked()
  })

  it('keeps the theme radio group mutually exclusive and saves the choice (PLAN.md 12.2.1 ⑤ / T1.17)', async () => {
    const user = userEvent.setup()
    render(<Settings />)

    const system = await screen.findByLabelText('跟随系统')
    const light = screen.getByLabelText('浅色')
    const dark = screen.getByLabelText('深色')

    expect(system).toBeChecked()

    await user.click(dark)
    await waitFor(() => expect(dark).toBeChecked())
    expect(system).not.toBeChecked()
    expect(light).not.toBeChecked()
    const saved = saveConfig.mock.calls.at(-1)?.[0] as AppConfig
    expect(saved.theme).toBe(ThemeMode.Dark)

    await user.click(light)
    await waitFor(() => expect(light).toBeChecked())
    expect(dark).not.toBeChecked()
  })
})
