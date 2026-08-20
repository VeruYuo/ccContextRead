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

function stubLoader(
  render: MermaidModule['default']['render'],
  parse: MermaidModule['default']['parse'] = vi.fn().mockResolvedValue(true),
): () => Promise<MermaidModule> {
  return async () => ({ default: { initialize: vi.fn(), parse, render } })
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

  it('keeps foreignObject node-label text through sanitization (flowchart htmlLabels shape)', async () => {
    const container = makeContainer('```mermaid\nflowchart TD; A-->B;\n```')
    const svg =
      '<svg viewBox="0 0 10 10"><g class="node">' +
      '<rect/>' +
      '<foreignObject width="20" height="10">' +
      '<div xmlns="http://www.w3.org/1999/xhtml" class="nodeLabel"><span class="nodeLabel">JSONL 会话文件</span></div>' +
      '</foreignObject>' +
      '</g></svg>'
    const render = vi.fn().mockResolvedValue({ svg })

    await renderMermaidBlocks(container, stubLoader(render))

    const label = container.querySelector('.nodeLabel')
    expect(label).not.toBeNull()
    expect(label?.textContent).toContain('JSONL 会话文件')
  })

  it('initializes mermaid with suppressErrorRendering so the library never mounts its own error graphic', async () => {
    const container = makeContainer('```mermaid\ngraph TD; A-->B;\n```')
    const initialize = vi.fn()
    const parse = vi.fn().mockResolvedValue(true)
    const render = vi.fn().mockResolvedValue({ svg: '<svg viewBox="0 0 1 1"></svg>' })
    const loader = async () => ({ default: { initialize, parse, render } })

    await renderMermaidBlocks(container, loader)

    expect(initialize).toHaveBeenCalledWith(expect.objectContaining({ suppressErrorRendering: true }))
  })

  it('skips mermaid.render entirely when mermaid.parse reports invalid syntax', async () => {
    const container = makeContainer('```mermaid\nbad syntax ((\n```')
    const parse = vi.fn().mockResolvedValue(false)
    const render = vi.fn()

    await renderMermaidBlocks(container, stubLoader(render, parse))

    expect(render).not.toHaveBeenCalled()
    const block = container.querySelector('.mermaid-block')
    expect(block?.querySelector('pre.mermaid-fallback')).not.toBeNull()
    expect(block?.textContent).toContain('bad syntax')
  })

  it('removes the mermaid-library error node left in document.body before rejecting, and degrades to a fallback code block', async () => {
    const container = makeContainer('```mermaid\nbad syntax ((\n```')
    const render = vi.fn().mockImplementation(async (id: string) => {
      const bomb = document.createElement('div')
      bomb.id = id
      bomb.className = 'mermaid-error-bomb'
      document.body.appendChild(bomb)
      const dPrefixed = document.createElement('div')
      dPrefixed.id = `d${id}`
      dPrefixed.className = 'mermaid-error-bomb'
      document.body.appendChild(dPrefixed)
      throw new Error('Parse error')
    })

    await renderMermaidBlocks(container, stubLoader(render))

    expect(document.querySelectorAll('.mermaid-error-bomb')).toHaveLength(0)
    const block = container.querySelector('.mermaid-block')
    expect(block?.querySelector('pre.mermaid-fallback')).not.toBeNull()
    expect(block?.querySelector('svg')).toBeNull()
  })
})
