import { useEffect, useMemo, useRef } from 'react'
import type { UpdateEvent } from '../api'
import { renderMarkdownHtml } from './markdownRender'
import { renderMermaidBlocks } from './mermaidRenderer'

interface Props {
  selectedId: string | null
  doc: UpdateEvent | null
  // Whether the preview tab is the one currently visible (App keeps this
  // component mounted under display:none while on Settings, per T1.13).
  // Rendering mermaid into a hidden container measures zero-size text and
  // fails (PLAN.md 12.2.1 问题④ candidate mechanism (a)) — gating on
  // visibility avoids that entirely, and re-running the effect when it
  // flips back to true catches up on any content that arrived while hidden.
  visible: boolean
}

// Preview is a pure display component (PLAN.md T1.13): the Markdown state
// it renders now lives in App, not here, so a tab switch that no longer
// unmounts this component (or one that did) never loses content.
export default function Preview({ selectedId, doc, visible }: Props) {
  const containerRef = useRef<HTMLDivElement>(null)
  const html = useMemo(() => {
    if (!doc) return ''
    return renderMarkdownHtml(doc.Markdown)
  }, [doc])

  // Mermaid blocks render async, after the synchronous HTML above is already
  // in the DOM (PLAN.md T1.16): mermaid itself is dynamically imported here
  // so its ~500KB stays out of the main bundle.
  useEffect(() => {
    if (!visible) return
    if (!containerRef.current) return
    renderMermaidBlocks(containerRef.current).catch((err) => {
      console.error('ccContextRead: mermaid render failed', err)
    })
  }, [html, visible])

  if (!selectedId) {
    return <div className="preview preview-empty">从左侧选择一个会话开始监听</div>
  }
  if (!doc) {
    return <div className="preview preview-empty">等待第一次写入…</div>
  }

  return <div ref={containerRef} className="preview" dangerouslySetInnerHTML={{ __html: html }} />
}
