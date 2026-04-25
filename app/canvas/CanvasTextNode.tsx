import { useState, useCallback, useRef, useEffect } from 'react'

import { cn } from '@s4wave/web/style/utils.js'

// CanvasTextNodeProps are the props for CanvasTextNode.
interface CanvasTextNodeProps {
  content: string
  autoEdit?: boolean
  onChange?: (content: string) => void
  onCancel?: () => void
  className?: string
}

// CanvasTextNode renders a text node with view/edit modes.
// Double-click to enter edit mode, Escape or blur to exit.
export function CanvasTextNode({
  content,
  autoEdit,
  onChange,
  onCancel,
  className,
}: CanvasTextNodeProps) {
  const [editing, setEditing] = useState(autoEdit ?? false)
  const [draft, setDraft] = useState(content)
  const draftRef = useRef(draft)
  const textareaRef = useRef<HTMLTextAreaElement | null>(null)

  useEffect(() => {
    draftRef.current = draft
  }, [draft])

  useEffect(() => {
    if (editing && textareaRef.current) {
      const el = textareaRef.current
      const timer = window.setTimeout(() => {
        el.focus()
      }, 0)
      return () => {
        window.clearTimeout(timer)
      }
    }
  }, [editing])

  const handleDoubleClick = useCallback(
    (e: React.MouseEvent) => {
      e.stopPropagation()
      if (!onChange) return
      setDraft(content)
      setEditing(true)
    },
    [content, onChange],
  )

  const commitEdit = useCallback(() => {
    setEditing(false)
    const current = draftRef.current
    const trimmed = current.trim()
    if (!trimmed && !content) {
      onCancel?.()
      return
    }
    if (current !== content) {
      onChange?.(current)
    }
  }, [content, onChange, onCancel])

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation()
        if (!content && !draftRef.current.trim()) {
          onCancel?.()
        }
        setEditing(false)
      } else if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
        e.stopPropagation()
        commitEdit()
      }
    },
    [commitEdit, content, onCancel],
  )

  if (editing) {
    return (
      <textarea
        ref={textareaRef}
        className={cn(
          'h-full w-full resize-none border-none bg-transparent p-2 text-sm outline-none',
          'font-[family-name:var(--font-display)]',
          className,
        )}
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={commitEdit}
        onKeyDown={handleKeyDown}
      />
    )
  }

  return (
    <pre
      className={cn(
        'h-full w-full cursor-text overflow-auto p-2 text-sm whitespace-pre-wrap',
        'font-[family-name:var(--font-display)]',
        className,
      )}
      onDoubleClick={handleDoubleClick}
    >
      {content}
    </pre>
  )
}
