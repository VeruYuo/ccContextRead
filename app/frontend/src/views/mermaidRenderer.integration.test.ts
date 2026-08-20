// Integration tests that exercise the *real* mermaid package (no stubbed
// loader) end to end through jsdom. PLAN.md 12.2.1: the unit tests in
// mermaidRenderer.test.ts stub mermaid.render() with toy SVG fixtures, which
// is exactly why S5d's real-machine regressions (①②) shipped with every
// automated test green — a toy stub never exercises mermaid's actual
// foreignObject/htmlLabels output or its real error path. These tests are
// the only ones that would have caught that class of bug.
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { renderMarkdownHtml } from './markdownRender'
import { renderMermaidBlocks } from './mermaidRenderer'

function makeContainer(markdown: string): HTMLDivElement {
  const div = document.createElement('div')
  div.innerHTML = renderMarkdownHtml(markdown)
  document.body.appendChild(div)
  return div
}

beforeEach(() => {
  document.body.innerHTML = ''
})

afterEach(() => {
  document.body.innerHTML = ''
})

describe('renderMermaidBlocks against the real mermaid package', () => {
  it('renders a valid flowchart with the node label text visible', async () => {
    const container = makeContainer('```mermaid\nflowchart TD\n  A[JSONL 会话文件] --> B[Markdown]\n```')

    await renderMermaidBlocks(container)

    const svg = container.querySelector('svg')
    expect(svg).not.toBeNull()
    expect(container.querySelector('.mermaid-fallback')).toBeNull()
    expect(container.textContent).toContain('JSONL 会话文件')
  }, 20000)

  it('renders a <br/> inside a node label as an actual line break, not literal text', async () => {
    const container = makeContainer('```mermaid\nflowchart TD\n  A["第一行<br/>第二行"] --> B[C]\n```')

    await renderMermaidBlocks(container)

    const svg = container.querySelector('svg')
    expect(svg).not.toBeNull()
    expect(container.querySelector('br')).not.toBeNull()
    expect(container.textContent).not.toContain('<br/>')
  }, 20000)

  it('renders a valid sequenceDiagram (regression guard — this path was already correct pre-S5e)', async () => {
    const container = makeContainer('```mermaid\nsequenceDiagram\n  Alice->>Bob: 你好\n```')

    await renderMermaidBlocks(container)

    const svg = container.querySelector('svg')
    expect(svg).not.toBeNull()
    expect(container.textContent).toContain('Alice')
    expect(container.textContent).toContain('Bob')
  }, 20000)

  it('degrades a syntax error to a fallback code block without mermaid\'s own error graphic leaking into the DOM', async () => {
    const container = makeContainer('```mermaid\nthis is }{ not valid mermaid syntax\n```')

    await renderMermaidBlocks(container)

    const block = container.querySelector('.mermaid-block')
    expect(block?.querySelector('svg')).toBeNull()
    expect(block?.querySelector('pre.mermaid-fallback')).not.toBeNull()
    // No leftover mermaid-library node (its error graphic, or any temporary
    // render container) anywhere in the document outside our own container.
    expect(document.body.textContent).not.toContain('Syntax error')
    const strayNodes = Array.from(document.body.children).filter((el) => el !== container)
    expect(strayNodes).toHaveLength(0)
  }, 20000)
})
