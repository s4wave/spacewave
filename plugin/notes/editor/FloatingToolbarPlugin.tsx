import { useCallback, useEffect, useRef, useState } from 'react'
import { useLexicalComposerContext } from '@lexical/react/LexicalComposerContext'
import {
  $getSelection,
  $isRangeSelection,
  FORMAT_TEXT_COMMAND,
  type TextFormatType,
} from 'lexical'
import { $isLinkNode, TOGGLE_LINK_COMMAND } from '@lexical/link'
import { cn } from '@s4wave/web/style/utils.js'
import { LuBold, LuItalic, LuCode, LuLink } from 'react-icons/lu'

// FloatingToolbarPlugin shows a floating toolbar when text is selected.
function FloatingToolbarPlugin() {
  const [editor] = useLexicalComposerContext()
  const [isVisible, setIsVisible] = useState(false)
  const [isBold, setIsBold] = useState(false)
  const [isItalic, setIsItalic] = useState(false)
  const [isCode, setIsCode] = useState(false)
  const [isLink, setIsLink] = useState(false)
  const [position, setPosition] = useState({ top: 0, left: 0 })
  const floatingRef = useRef<HTMLDivElement | null>(null)

  const updatePosition = useCallback(() => {
    const nativeSelection = window.getSelection()
    if (
      !nativeSelection ||
      nativeSelection.rangeCount === 0 ||
      nativeSelection.isCollapsed
    ) {
      setIsVisible(false)
      return
    }

    const range = nativeSelection.getRangeAt(0)
    const rect = range.getBoundingClientRect()

    if (rect.width === 0 && rect.height === 0) {
      setIsVisible(false)
      return
    }

    const editorRoot = editor.getRootElement()
    if (!editorRoot) return
    const editorRect = editorRoot.getBoundingClientRect()

    setPosition({
      top: rect.top - editorRect.top - 44,
      left: rect.left - editorRect.left + rect.width / 2,
    })
    setIsVisible(true)
  }, [editor])

  const updateState = useCallback(() => {
    editor.getEditorState().read(() => {
      const selection = $getSelection()
      if (!$isRangeSelection(selection) || selection.isCollapsed()) {
        setIsVisible(false)
        return
      }

      setIsBold(selection.hasFormat('bold'))
      setIsItalic(selection.hasFormat('italic'))
      setIsCode(selection.hasFormat('code'))

      const node = selection.anchor.getNode()
      const parent = node.getParent()
      setIsLink($isLinkNode(parent) || $isLinkNode(node))

      updatePosition()
    })
  }, [editor, updatePosition])

  useEffect(() => {
    return editor.registerUpdateListener(() => {
      updateState()
    })
  }, [editor, updateState])

  useEffect(() => {
    const handleSelectionChange = () => {
      const rootElement = editor.getRootElement()
      if (!rootElement) return

      const nativeSelection = window.getSelection()
      if (!nativeSelection || nativeSelection.rangeCount === 0) {
        setIsVisible(false)
        return
      }

      if (!rootElement.contains(nativeSelection.anchorNode)) {
        setIsVisible(false)
        return
      }

      updateState()
    }

    document.addEventListener('selectionchange', handleSelectionChange)
    return () =>
      document.removeEventListener('selectionchange', handleSelectionChange)
  }, [editor, updateState])

  const handleFormat = useCallback(
    (format: TextFormatType) => {
      editor.dispatchCommand(FORMAT_TEXT_COMMAND, format)
    },
    [editor],
  )

  const handleLink = useCallback(() => {
    editor.getEditorState().read(() => {
      const selection = $getSelection()
      if (!$isRangeSelection(selection)) return
      const node = selection.anchor.getNode()
      const parent = node.getParent()
      if ($isLinkNode(parent) || $isLinkNode(node)) {
        editor.dispatchCommand(TOGGLE_LINK_COMMAND, null)
      } else {
        const url = window.prompt('Enter URL:')
        if (url && /^https?:\/\//i.test(url.trim())) {
          editor.dispatchCommand(TOGGLE_LINK_COMMAND, url.trim())
        }
      }
    })
  }, [editor])

  if (!isVisible) return null

  const btnClass =
    'flex items-center justify-center rounded p-1.5 text-foreground hover:bg-list-hover-background'
  const activeClass = 'bg-brand/10 text-brand'

  return (
    <div
      ref={floatingRef}
      className="bg-popover border-border absolute z-50 flex items-center gap-0.5 rounded-lg border p-1 shadow-lg"
      style={{
        top: position.top,
        left: position.left,
        transform: 'translateX(-50%)',
      }}
    >
      <button
        type="button"
        className={cn(btnClass, isBold && activeClass)}
        onMouseDown={(e) => {
          e.preventDefault()
          handleFormat('bold')
        }}
        title="Bold"
      >
        <LuBold className="h-3.5 w-3.5" />
      </button>
      <button
        type="button"
        className={cn(btnClass, isItalic && activeClass)}
        onMouseDown={(e) => {
          e.preventDefault()
          handleFormat('italic')
        }}
        title="Italic"
      >
        <LuItalic className="h-3.5 w-3.5" />
      </button>
      <button
        type="button"
        className={cn(btnClass, isCode && activeClass)}
        onMouseDown={(e) => {
          e.preventDefault()
          handleFormat('code')
        }}
        title="Code"
      >
        <LuCode className="h-3.5 w-3.5" />
      </button>
      <button
        type="button"
        className={cn(btnClass, isLink && activeClass)}
        onMouseDown={(e) => {
          e.preventDefault()
          handleLink()
        }}
        title="Link"
      >
        <LuLink className="h-3.5 w-3.5" />
      </button>
    </div>
  )
}

export default FloatingToolbarPlugin
