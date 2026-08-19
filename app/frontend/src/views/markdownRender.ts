import DOMPurify from 'dompurify'
import hljs from 'highlight.js/lib/core'
import bash from 'highlight.js/lib/languages/bash'
import c from 'highlight.js/lib/languages/c'
import cpp from 'highlight.js/lib/languages/cpp'
import csharp from 'highlight.js/lib/languages/csharp'
import css from 'highlight.js/lib/languages/css'
import diff from 'highlight.js/lib/languages/diff'
import go from 'highlight.js/lib/languages/go'
import java from 'highlight.js/lib/languages/java'
import javascript from 'highlight.js/lib/languages/javascript'
import json from 'highlight.js/lib/languages/json'
import markdownLang from 'highlight.js/lib/languages/markdown'
import php from 'highlight.js/lib/languages/php'
import python from 'highlight.js/lib/languages/python'
import ruby from 'highlight.js/lib/languages/ruby'
import rust from 'highlight.js/lib/languages/rust'
import sql from 'highlight.js/lib/languages/sql'
import typescript from 'highlight.js/lib/languages/typescript'
import xml from 'highlight.js/lib/languages/xml'
import yaml from 'highlight.js/lib/languages/yaml'
import { marked } from 'marked'
import type { Tokens } from 'marked'

// highlight.js/lib/core ships zero languages by default (the plain
// 'highlight.js' entry point statically bundles all ~190 of them, which
// alone bloats the main bundle by roughly 1 MB) — register only the
// languages CC sessions realistically contain.
hljs.registerLanguage('bash', bash)
hljs.registerLanguage('shell', bash)
hljs.registerLanguage('c', c)
hljs.registerLanguage('cpp', cpp)
hljs.registerLanguage('csharp', csharp)
hljs.registerLanguage('css', css)
hljs.registerLanguage('diff', diff)
hljs.registerLanguage('go', go)
hljs.registerLanguage('java', java)
hljs.registerLanguage('javascript', javascript)
hljs.registerLanguage('js', javascript)
hljs.registerLanguage('json', json)
hljs.registerLanguage('markdown', markdownLang)
hljs.registerLanguage('md', markdownLang)
hljs.registerLanguage('php', php)
hljs.registerLanguage('python', python)
hljs.registerLanguage('py', python)
hljs.registerLanguage('ruby', ruby)
hljs.registerLanguage('rust', rust)
hljs.registerLanguage('sql', sql)
hljs.registerLanguage('typescript', typescript)
hljs.registerLanguage('ts', typescript)
hljs.registerLanguage('xml', xml)
hljs.registerLanguage('html', xml)
hljs.registerLanguage('yaml', yaml)
hljs.registerLanguage('yml', yaml)

// hashSource is a small non-cryptographic hash (djb2 variant) used purely as
// a cache key for rendered mermaid diagrams — collisions are not a security
// concern here, only a cache-key concern.
export function hashSource(input: string): string {
  let hash = 5381
  for (let i = 0; i < input.length; i++) {
    hash = (hash * 33) ^ input.charCodeAt(i)
  }
  return (hash >>> 0).toString(16)
}

function escapeHtml(input: string): string {
  return input.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

// renderCode overrides marked's fenced-code rendering: mermaid blocks become
// a placeholder div (rendered async, later, by renderMermaidBlocks — see
// PLAN.md T1.16), everything else gets highlight.js syntax highlighting when
// the language is recognized, or a plain escaped block otherwise.
function renderCode({ text, lang }: Tokens.Code): string {
  const language = (lang ?? '').trim().split(/\s+/)[0]

  if (language === 'mermaid') {
    const hash = hashSource(text)
    return (
      `<div class="mermaid-block" data-mermaid-hash="${hash}" data-mermaid-src="${encodeURIComponent(text)}">` +
      `<pre class="mermaid-fallback"><code>${escapeHtml(text)}</code></pre>` +
      `</div>`
    )
  }

  if (language && hljs.getLanguage(language)) {
    const highlighted = hljs.highlight(text, { language }).value
    return `<pre><code class="hljs language-${language}">${highlighted}</code></pre>`
  }

  return `<pre><code>${escapeHtml(text)}</code></pre>`
}

const renderer = new marked.Renderer()
renderer.code = renderCode

export function renderMarkdownHtml(markdown: string): string {
  const html = marked.parse(markdown, { renderer }) as string
  return DOMPurify.sanitize(html, { ADD_ATTR: ['data-mermaid-src', 'data-mermaid-hash'] })
}
