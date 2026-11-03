import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
/* eslint-disable react-doctor/rerender-state-only-in-handlers */
import { useState, useCallback, useRef, useEffect } from 'react'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    '--tw-border-style': 'none',
    '--tw-outline-style': 'none',
    padding: `calc(${themeLayout['--spacing']} * 2)`,
    borderStyle: 'none',
    backgroundColor: '#0000',
    fontFamily: themeFonts['--font-display'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    outlineStyle: 'none',
    resize: 'none',
    height: '100%',
    width: '100%',
  },
  rule2: {
    padding: `calc(${themeLayout['--spacing']} * 2)`,
    overflow: 'auto',
    cursor: 'text',
    fontFamily: themeFonts['--font-display'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    whiteSpace: 'pre-wrap',
    height: '100%',
    width: '100%',
  },
})

// CanvasTextNodeProps are the props for CanvasTextNode.
interface CanvasTextNodeProps {
  className?: never
  content: string
  autoEdit?: boolean
  onChange?: (content: string) => void
  onCancel?: () => void
  sx?: ComponentStyles
}

// CanvasTextNode renders a text node with view/edit modes.
// Double-click to enter edit mode, Escape or blur to exit.
export function CanvasTextNode({
  content,
  autoEdit,
  onChange,
  onCancel,
  sx,
}: Omit<CanvasTextNodeProps, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  const [editing, setEditing] = useState(autoEdit ?? false)
  const initialContentRef = useRef(content)
  const [draft, setDraft] = useState(() => initialContentRef.current)
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
        aria-label="Node text"
        sx={[stylexStyles.rule1, sx]}
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={commitEdit}
        onKeyDown={handleKeyDown}
      />
    )
  }

  return (
    <pre sx={[stylexStyles.rule2, sx]} onDoubleClick={handleDoubleClick}>
      {content}
    </pre>
  )
}
