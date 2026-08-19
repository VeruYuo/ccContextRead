import DOMPurify from 'dompurify'

export interface MermaidModule {
  default: {
    initialize: (options: Record<string, unknown>) => void
    render: (id: string, text: string) => Promise<{ svg: string }>
  }
}

type Loader = () => Promise<MermaidModule>

const defaultLoader: Loader = () => import('mermaid')

// svgCache persists across renderMermaidBlocks calls (module-level, not
// component state) so the same diagram appearing in consecutive
// session:updated events is rendered once, not re-rendered every tick
// (PLAN.md T1.16 边界).
const svgCache = new Map<string, string>()
let nextRenderId = 0

function isDarkMode(): boolean {
  return typeof window !== 'undefined' && window.matchMedia?.('(prefers-color-scheme: dark)').matches === true
}

// loadMermaid does not memoize the loader promise itself: production code
// always passes the same loader (native dynamic import()), which the JS
// engine's own module cache already deduplicates, so re-calling it here is
// free. Re-running initialize() each time is intentional and cheap — it
// keeps the diagram theme in sync with the current light/dark mode.
async function loadMermaid(loader: Loader): Promise<MermaidModule['default']> {
  const mermaid = (await loader()).default
  mermaid.initialize({ startOnLoad: false, theme: isDarkMode() ? 'dark' : 'default' })
  return mermaid
}

function escapeHtml(input: string): string {
  return input.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

// renderMermaidBlocks finds unrendered mermaid placeholders (see
// markdownRender.ts) inside container and replaces each with its rendered,
// sanitized SVG. A diagram that fails to parse degrades to a code block
// instead of throwing, and does not affect any other diagram in the same
// container (PLAN.md T1.16 边界).
export async function renderMermaidBlocks(container: HTMLElement, loader: Loader = defaultLoader): Promise<void> {
  const nodes = Array.from(container.querySelectorAll<HTMLElement>('[data-mermaid-src]'))
  if (nodes.length === 0) return

  const mermaid = await loadMermaid(loader)

  await Promise.all(
    nodes.map(async (node) => {
      const hash = node.dataset.mermaidHash ?? ''
      const src = decodeURIComponent(node.dataset.mermaidSrc ?? '')
      node.removeAttribute('data-mermaid-src')

      const cached = svgCache.get(hash)
      if (cached !== undefined) {
        node.innerHTML = cached
        return
      }

      try {
        const { svg } = await mermaid.render(`mermaid-${hash}-${nextRenderId++}`, src)
        const clean = DOMPurify.sanitize(svg, { USE_PROFILES: { svg: true, svgFilters: true } })
        svgCache.set(hash, clean)
        node.innerHTML = clean
      } catch {
        node.innerHTML = `<pre class="mermaid-fallback"><code>${escapeHtml(src)}</code></pre>`
      }
    }),
  )
}
