import { render } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { UpdateEvent } from '../api'

// PLAN.md 12.2.1 问题④ visible: while the preview tab is hidden (CSS
// display:none, Preview itself stays mounted per T1.13), mermaid must not
// attempt to render — that's candidate mechanism (a) for the diagram
// permanently reverting to a code block. Switching back to visible must
// trigger a render pass so any content that arrived while hidden catches up.
const { renderMermaidBlocks } = vi.hoisted(() => ({
  renderMermaidBlocks: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('./mermaidRenderer', () => ({ renderMermaidBlocks }))

import Preview from './Preview'

function makeDoc(markdown: string): UpdateEvent {
  return { SessionID: 's1', Markdown: markdown, OutputPath: 'x', EventCount: 1, Full: true }
}

describe('Preview mermaid rendering gated by tab visibility', () => {
  it('does not call renderMermaidBlocks while visible=false', () => {
    renderMermaidBlocks.mockClear()

    render(<Preview selectedId="s1" doc={makeDoc('```mermaid\ngraph TD; A-->B;\n```')} visible={false} />)

    expect(renderMermaidBlocks).not.toHaveBeenCalled()
  })

  it('calls renderMermaidBlocks once visible flips to true, to catch up on content that arrived while hidden', () => {
    renderMermaidBlocks.mockClear()
    const doc = makeDoc('```mermaid\ngraph TD; A-->B;\n```')
    const { rerender } = render(<Preview selectedId="s1" doc={doc} visible={false} />)
    expect(renderMermaidBlocks).not.toHaveBeenCalled()

    rerender(<Preview selectedId="s1" doc={doc} visible={true} />)

    expect(renderMermaidBlocks).toHaveBeenCalledTimes(1)
  })
})
