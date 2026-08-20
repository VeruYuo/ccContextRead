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

// nodeGeneration guards against a stale async render round clobbering a
// fresher one for the *same* DOM node (PLAN.md 12.2.1 问题④ Z). This only
// matters when the same node is targeted twice before the first round
// resolves (e.g. a tab visibility flip re-triggers the effect while a slow
// render() from before is still in flight); a brand-new node from a
// replaced html string is simply never in this map.
const nodeGeneration = new WeakMap<HTMLElement, number>()

// isDarkMode reads the theme actually in effect (PLAN.md 12.2.1 ⑤: theme.ts
// resolves the user's light/dark/system setting and writes it onto
// <html data-theme>), not the raw OS media query directly — a manual
// override wouldn't otherwise be reflected here. Falls back to the media
// query only if nothing has applied a theme yet (e.g. before Settings has
// loaded config, or in a test that doesn't render the theme controller).
function isDarkMode(): boolean {
  if (typeof document !== 'undefined') {
    const applied = document.documentElement.dataset.theme
    if (applied) return applied === 'dark'
  }
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

function markDone(node: HTMLElement, html: string): void {
  node.innerHTML = html
  node.removeAttribute('data-mermaid-src')
  node.setAttribute('data-mermaid-done', 'true')
}

// renderMermaidBlocks finds unrendered mermaid placeholders (see
// markdownRender.ts) inside container and replaces each with its rendered,
// sanitized SVG. A diagram that fails to parse degrades to a code block
// instead of throwing, and does not affect any other diagram in the same
// container (PLAN.md T1.16 边界).
//
// PLAN.md 12.2.1 问题④: a tab round-trip (or any doc update while the
// preview is hidden) can reset the DOM back to the placeholder markup for a
// diagram that was already rendered once. Two things make that recoverable
// rather than a permanent regression to the code block:
//  - X: nodes whose hash is already in svgCache are restored synchronously,
//    before any `await` — so even a caller that never awaits this promise
//    (e.g. a React effect) sees them reappear immediately.
//  - Z: `data-mermaid-src` is only removed (and `data-mermaid-done` set) on
//    success, so a failed attempt stays selectable by the query below and
//    gets retried on the next pass instead of degrading forever; a
//    per-node generation counter discards a stale round's result if a
//    fresher round already started for the same node.
export async function renderMermaidBlocks(container: HTMLElement, loader: Loader = defaultLoader): Promise<void> {
  const nodes = Array.from(container.querySelectorAll<HTMLElement>('[data-mermaid-src]:not([data-mermaid-done])'))
  if (nodes.length === 0) return

  // Read once, synchronously, before any await: a diagram's SVG embeds its
  // colors inline, so a light-rendered and a dark-rendered copy are not
  // interchangeable — the cache key must include theme (PLAN.md 12.2.1 ⑤),
  // and X's synchronous cache-hit restore below needs this value before it
  // can look anything up.
  const cacheKeySuffix = isDarkMode() ? ':dark' : ':light'

  const pending: HTMLElement[] = []
  for (const node of nodes) {
    const cacheKey = (node.dataset.mermaidHash ?? '') + cacheKeySuffix
    const cached = svgCache.get(cacheKey)
    if (cached !== undefined) {
      markDone(node, cached)
    } else {
      pending.push(node)
    }
  }
  if (pending.length === 0) return

  const mermaid = await loadMermaid(loader)

  await Promise.all(
    pending.map(async (node) => {
      const cacheKey = (node.dataset.mermaidHash ?? '') + cacheKeySuffix
      const src = decodeURIComponent(node.dataset.mermaidSrc ?? '')
      const gen = (nodeGeneration.get(node) ?? 0) + 1
      nodeGeneration.set(node, gen)
      const isStale = () => nodeGeneration.get(node) !== gen

      const renderId = `mermaid-${node.dataset.mermaidHash ?? ''}-${nextRenderId++}`
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
        svgCache.set(cacheKey, clean)
        if (isStale()) return
        markDone(node, clean)
      } catch {
        if (isStale()) return
        // Deliberately does not remove data-mermaid-src or set
        // data-mermaid-done: this node stays selectable on the next pass.
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
