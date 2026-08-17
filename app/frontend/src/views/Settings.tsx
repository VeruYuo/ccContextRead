import { useEffect, useState } from 'react'
import type { AppConfig, FilterConfig } from '../api'
import { FileChangeMode, ImageMode, chooseOutputDir, getConfig, saveConfig } from '../api'
import {
  setFileChange,
  setImageMode,
  setOutputDirOverride,
  setTruncateChars,
  toggleFilterSwitch,
} from './settingsLogic'

type SwitchKey = keyof Omit<FilterConfig, 'fileChange' | 'truncateChars'>

const FILTER_SWITCHES: Array<{ key: SwitchKey; label: string }> = [
  { key: 'userPrompt', label: '用户输入' },
  { key: 'assistantText', label: '助手最终回复' },
  { key: 'thinking', label: '思维链' },
  { key: 'toolUse', label: '工具调用（含参数）' },
  { key: 'toolResult', label: '工具结果' },
  { key: 'subagent', label: '子 Agent 会话' },
  { key: 'contextInjection', label: '上下文注入' },
  { key: 'systemNote', label: '系统记录' },
  { key: 'compactSummary', label: 'Compact 摘要正文' },
]

export default function Settings() {
  const [cfg, setCfg] = useState<AppConfig | null>(null)

  useEffect(() => {
    getConfig().then(setCfg)
  }, [])

  function apply(next: AppConfig) {
    setCfg(next)
    saveConfig(next).catch((err) => console.error('ccContextRead: saveConfig failed', err))
  }

  if (!cfg) {
    return <div className="settings">加载设置中…</div>
  }

  return (
    <div className="settings">
      <fieldset>
        <legend>内容过滤</legend>
        {FILTER_SWITCHES.map(({ key, label }) => (
          <label key={key}>
            <input type="checkbox" checked={cfg.filter[key]} onChange={() => apply(toggleFilterSwitch(cfg, key))} />
            {label}
          </label>
        ))}
      </fieldset>

      <fieldset>
        <legend>文件变更</legend>
        <label>
          <input
            type="radio"
            name="fileChange"
            checked={cfg.filter.fileChange === FileChangeMode.None}
            onChange={() => apply(setFileChange(cfg, FileChangeMode.None))}
          />
          不记录
        </label>
        <label>
          <input
            type="radio"
            name="fileChange"
            checked={cfg.filter.fileChange === FileChangeMode.Summary}
            onChange={() => apply(setFileChange(cfg, FileChangeMode.Summary))}
          />
          仅路径与操作
        </label>
        <label>
          <input
            type="radio"
            name="fileChange"
            checked={cfg.filter.fileChange === FileChangeMode.Full}
            onChange={() => apply(setFileChange(cfg, FileChangeMode.Full))}
          />
          完整 diff
        </label>
      </fieldset>

      <fieldset>
        <legend>图片处理</legend>
        <label>
          <input
            type="radio"
            name="imageMode"
            checked={cfg.imageMode === ImageMode.Placeholder}
            onChange={() => apply(setImageMode(cfg, ImageMode.Placeholder))}
          />
          占位符
        </label>
        <label>
          <input
            type="radio"
            name="imageMode"
            checked={cfg.imageMode === ImageMode.Attachment}
            onChange={() => apply(setImageMode(cfg, ImageMode.Attachment))}
          />
          落盘为附件
        </label>
      </fieldset>

      <label>
        工具结果截断阈值（字符，0 表示不截断）
        <input
          type="number"
          value={cfg.filter.truncateChars}
          onChange={(e) => apply(setTruncateChars(cfg, e.target.value))}
        />
      </label>

      <fieldset>
        <legend>输出目录</legend>
        <input type="text" readOnly value={cfg.outputDirOverride || cfg.resolvedOutputDir} />
        <button
          type="button"
          onClick={async () => {
            const dir = await chooseOutputDir()
            if (dir) apply(setOutputDirOverride(cfg, dir))
          }}
        >
          浏览…
        </button>
        <button
          type="button"
          disabled={!cfg.outputDirOverride}
          onClick={() => apply(setOutputDirOverride(cfg, ''))}
        >
          恢复自动
        </button>
      </fieldset>
    </div>
  )
}
