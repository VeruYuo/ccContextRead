import DOMPurify from 'dompurify'
import { marked } from 'marked'
import { useMemo } from 'react'
import type { UpdateEvent } from '../api'

interface Props {
  selectedId: string | null
  doc: UpdateEvent | null
}

// Preview is a pure display component (PLAN.md T1.13): the Markdown state
// it renders now lives in App, not here, so a tab switch that no longer
// unmounts this component (or one that did) never loses content.
export default function Preview({ selectedId, doc }: Props) {
  const html = useMemo(() => {
    if (!doc) return ''
    return DOMPurify.sanitize(marked.parse(doc.Markdown) as string)
  }, [doc])

  if (!selectedId) {
    return <div className="preview preview-empty">从左侧选择一个会话开始监听</div>
  }
  if (!doc) {
    return <div className="preview preview-empty">等待第一次写入…</div>
  }

  return <div className="preview" dangerouslySetInnerHTML={{ __html: html }} />
}
