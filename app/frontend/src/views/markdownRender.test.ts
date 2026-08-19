import { describe, expect, it } from 'vitest'
import { hashSource, renderMarkdownHtml } from './markdownRender'

describe('renderMarkdownHtml', () => {
  it('replaces a mermaid code block with a placeholder carrying the raw source, not a rendered SVG', () => {
    const html = renderMarkdownHtml('```mermaid\ngraph TD; A-->B;\n```')
    expect(html).toContain('data-mermaid-src')
    expect(html).not.toContain('<svg')
    // 边界：语法渲染前的兜底内容是原始源码，供 mermaid 渲染失败时降级展示
    expect(html).toContain('graph TD; A--&gt;B;')
  })

  it('leaves a normal fenced code block untouched by the mermaid path and applies syntax highlighting', () => {
    const html = renderMarkdownHtml('```go\nfunc main() {}\n```')
    expect(html).not.toContain('data-mermaid-src')
    expect(html).toContain('language-go')
    expect(html).toContain('hljs')
  })

  it('falls back to a plain escaped code block for an unknown language', () => {
    const html = renderMarkdownHtml('```notalanguage\nsome text\n```')
    expect(html).not.toContain('data-mermaid-src')
    expect(html).toContain('<pre><code>')
    expect(html).toContain('some text')
  })
})

describe('hashSource', () => {
  it('is deterministic for identical input', () => {
    expect(hashSource('graph TD; A-->B;')).toBe(hashSource('graph TD; A-->B;'))
  })

  it('differs for different input', () => {
    expect(hashSource('graph TD; A-->B;')).not.toBe(hashSource('graph TD; C-->D;'))
  })
})
