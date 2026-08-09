import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ChangeEvent,
  type KeyboardEvent,
  type MouseEvent,
} from 'react'

import { cn } from '@s4wave/web/style/utils.js'

interface CanvasTextNodeProps {
  content: string
  autoEdit?: boolean
  onChange?: (content: string) => void
  onCancel?: () => void
  className?: string
}

interface CanvasTextEditorProps extends CanvasTextNodeProps {
  onFinish: () => void
}

function CanvasTextEditor({
  content,
  onChange,
  onCancel,
  onFinish,
  className,
}: CanvasTextEditorProps) {
  const [draft, setDraft] = useState(content)
  const textareaRef = useRef<HTMLTextAreaElement | null>(null)

  useEffect(() => {
    const timer = window.setTimeout(() => textareaRef.current?.focus(), 0)
    return () => window.clearTimeout(timer)
  }, [])

  const commit = useCallback(() => {
    onFinish()
    if (!draft.trim() && !content) {
      onCancel?.()
    } else if (draft !== content) {
      onChange?.(draft)
    }
  }, [content, draft, onCancel, onChange, onFinish])

  const updateDraft = useCallback((event: ChangeEvent<HTMLTextAreaElement>) => {
    setDraft(event.target.value)
  }, [])

  const handleKeyDown = useCallback(
    (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.stopPropagation()
        onFinish()
        if (!content && !draft.trim()) onCancel?.()
      } else if (event.key === 'Enter' && (event.ctrlKey || event.metaKey)) {
        event.stopPropagation()
        commit()
      }
    },
    [commit, content, draft, onCancel, onFinish],
  )

  return (
    <textarea
      ref={textareaRef}
      aria-label="Canvas text"
      className={cn(
        'h-full w-full resize-none border-none bg-transparent p-2 text-sm outline-none',
        'font-[family-name:var(--font-display)]',
        className,
      )}
      value={draft}
      onChange={updateDraft}
      onBlur={commit}
      onKeyDown={handleKeyDown}
    />
  )
}

// CanvasTextNode renders a text node and confines draft state to its editor.
export function CanvasTextNode({
  content,
  autoEdit = false,
  onChange,
  onCancel,
  className,
}: CanvasTextNodeProps) {
  const [editing, setEditing] = useState(autoEdit)
  const finishEditing = useCallback(() => setEditing(false), [])
  const handleDoubleClick = useCallback(
    (event: MouseEvent) => {
      event.stopPropagation()
      if (onChange) setEditing(true)
    },
    [onChange],
  )

  if (editing) {
    return (
      <CanvasTextEditor
        content={content}
        onChange={onChange}
        onCancel={onCancel}
        onFinish={finishEditing}
        className={className}
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
