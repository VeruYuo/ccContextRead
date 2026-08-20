import DOMPurify from 'dompurify'

export interface MermaidModule {
  default: {
    initialize: (options: Record<string, unknown>) => void
    parse: (text: string, options: { suppressErrors: true }) => Promise<unknown>
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
  mermaid.initialize({
    startOnLoad: false,
    theme: isDarkMode() ? 'dark' : 'default',
    // mermaid v11 paints its own "bomb + Syntax error" graphic into the DOM
    // before rejecting render() — suppressErrorRendering stops that at the
    // source (PLAN.md 12.2.1 问题②). The parse() precheck and finally-block
    // cleanup below are defense in depth in case a version doesn't honor it.
    suppressErrorRendering: true,
  })
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

      const renderId = `mermaid-${hash}-${nextRenderId++}`
      try {
        const parseOk = await mermaid.parse(src, { suppressErrors: true })
        if (!parseOk) throw new Error('mermaid parse failed')

        const { svg } = await mermaid.render(renderId, src)
        // Three settings, all required together (PLAN.md 12.2.1 问题①):
        // mermaid v11's default htmlLabels:true renders flowchart node text as
        // <foreignObject><div class="nodeLabel"><span>…</span></div></foreignObject>.
        //  - html profile allows div/span at all (svg/svgFilters alone don't).
        //  - foreignObject is hardcoded into DOMPurify's hidden default
        //    FORBID_TAGS-adjacent disallow list — ADD_TAGS is required just to
        //    keep the wrapper itself.
        //  - even kept, DOMPurify's namespace check only lets HTML content
        //    live inside foreignObject if 'foreignobject' is a declared HTML
        //    integration point; DOMPurify's own default only lists
        //    'annotation-xml' (a MathML integration point), so without this
        //    override every element inside foreignObject is force-removed
        //    (not hoisted) regardless of the html/ADD_TAGS settings above.
        const clean = DOMPurify.sanitize(svg, {
          USE_PROFILES: { svg: true, svgFilters: true, html: true },
          ADD_TAGS: ['foreignObject'],
          HTML_INTEGRATION_POINTS: { 'annotation-xml': true, foreignobject: true },
        })
        svgCache.set(hash, clean)
        node.innerHTML = clean
      } catch {
        node.innerHTML = `<pre class="mermaid-fallback"><code>${escapeHtml(src)}</code></pre>`
      } finally {
        // Defense in depth for 问题②: if mermaid still painted its error
        // graphic into the DOM despite suppressErrorRendering, remove it —
        // mermaid has historically used both the bare id and a `d`-prefixed
        // id for this temporary container. Must check node.contains() first:
        // on success, mermaid's own <svg id="renderId"> is now legitimately
        // inside `node` (we just put it there), and getElementById would
        // find that instead of a stray error node and delete the diagram we
        // just rendered.
        for (const strayId of [renderId, `d${renderId}`]) {
          const stray = document.getElementById(strayId)
          if (stray && !node.contains(stray)) stray.remove()
        }
      }
    }),
  )
}
