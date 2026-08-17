import DOMPurify from 'dompurify'
import { marked } from 'marked'
import { useEffect, useMemo, useState } from 'react'
import type { UpdateEvent } from '../api'
import { useSessionUpdated } from '../api'

interface Props {
  selectedId: string | null
}

export default function Preview({ selectedId }: Props) {
  const [latest, setLatest] = useState<UpdateEvent | null>(null)

  useEffect(() => {
    setLatest(null)
  }, [selectedId])

  useSessionUpdated((ev) => {
    if (ev.SessionID === selectedId) setLatest(ev)
  })

  const html = useMemo(() => {
    if (!latest) return ''
    return DOMPurify.sanitize(marked.parse(latest.Markdown) as string)
  }, [latest])

  if (!selectedId) {
    return <div className="preview preview-empty">从左侧选择一个会话开始监听</div>
  }
  if (!latest) {
    return <div className="preview preview-empty">等待第一次写入…</div>
  }

  return <div className="preview" dangerouslySetInnerHTML={{ __html: html }} />
}
