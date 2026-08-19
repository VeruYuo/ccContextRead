import { describe, expect, it, vi } from 'vitest'
import { renderMarkdownHtml } from './markdownRender'
import type { MermaidModule } from './mermaidRenderer'
import { renderMermaidBlocks } from './mermaidRenderer'

function makeContainer(markdown: string): HTMLDivElement {
  const div = document.createElement('div')
  div.innerHTML = renderMarkdownHtml(markdown)
  document.body.appendChild(div)
  return div
}

function stubLoader(render: MermaidModule['default']['render']): () => Promise<MermaidModule> {
  return async () => ({ default: { initialize: vi.fn(), render } })
}

describe('renderMermaidBlocks', () => {
  it('replaces the placeholder with a sanitized SVG whose structural attributes survive', async () => {
    const container = makeContainer('```mermaid\ngraph TD; A-->B;\n```')
    const render = vi.fn().mockResolvedValue({ svg: '<svg viewBox="0 0 10 10"><path d="M0 0"/></svg>' })

    await renderMermaidBlocks(container, stubLoader(render))

    expect(render).toHaveBeenCalledTimes(1)
    const svg = container.querySelector('svg')
    expect(svg).not.toBeNull()
    expect(svg?.getAttribute('viewBox')).toBe('0 0 10 10')
    expect(container.querySelector('[data-mermaid-src]')).toBeNull()
  })

  it('degrades a syntax error to a code block without throwing and without affecting a second, valid diagram', async () => {
    const container = makeContainer(
      '```mermaid\nbad syntax ((\n```\n\n```mermaid\ngraph TD; C-->D;\n```',
    )
    const render = vi
      .fn()
      .mockRejectedValueOnce(new Error('Parse error'))
      .mockResolvedValueOnce({ svg: '<svg viewBox="0 0 1 1"></svg>' })

    await expect(renderMermaidBlocks(container, stubLoader(render))).resolves.toBeUndefined()

    const blocks = container.querySelectorAll('.mermaid-block')
    expect(blocks).toHaveLength(2)
    expect(blocks[0].querySelector('svg')).toBeNull()
    expect(blocks[0].querySelector('pre.mermaid-fallback')).not.toBeNull()
    expect(blocks[0].textContent).toContain('bad syntax')
    expect(blocks[1].querySelector('svg')).not.toBeNull()
  })

  it('does not call mermaid.render again for a diagram whose content was already rendered (hash cache)', async () => {
    const markdown = '```mermaid\ngraph TD; E-->F;\n```'
    const render = vi.fn().mockResolvedValue({ svg: '<svg viewBox="0 0 2 2"></svg>' })
    const loader = stubLoader(render)

    const first = makeContainer(markdown)
    await renderMermaidBlocks(first, loader)
    expect(render).toHaveBeenCalledTimes(1)

    // 模拟同一份图在下一次 session:updated 中再次出现（新的 DOM 节点，同样的源码）
    const second = makeContainer(markdown)
    await renderMermaidBlocks(second, loader)
    expect(render).toHaveBeenCalledTimes(1)
    expect(second.querySelector('svg')).not.toBeNull()
  })

  it('is a no-op when the container has no mermaid placeholders (does not load mermaid at all)', async () => {
    const container = makeContainer('```go\nfunc main() {}\n```')
    const loader = vi.fn(stubLoader(vi.fn()))

    await renderMermaidBlocks(container, loader)

    expect(loader).not.toHaveBeenCalled()
  })
})
