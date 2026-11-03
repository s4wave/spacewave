import { useCallback, useEffect, useEffectEvent, useRef, useState } from 'react'
import { useLexicalComposerContext } from '@lexical/react/LexicalComposerContext'
import {
  $getSelection,
  $isRangeSelection,
  FORMAT_TEXT_COMMAND,
  type TextFormatType,
} from 'lexical'
import { $isLinkNode, TOGGLE_LINK_COMMAND } from '@lexical/link'

import { LuBold, LuItalic, LuCode, LuLink } from '@s4wave/web/images/icons.js'
import { TextInputDialog } from '../NoteDialogs.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    top: 'var(--floating-toolbar-top)',
    left: 'var(--floating-toolbar-left)',
    transform: 'translateX(-50%)',
    position: 'absolute',
    zIndex: 50,
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * .5)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: themeColors['--color-border'],
    backgroundColor: themeColors['--color-popover'],
    padding: themeLayout['--spacing'],
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
  },
  rule2: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '.25rem',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-list-hover-background'],
      },
    },
    padding: `calc(${themeLayout['--spacing']} * 1.5)`,
    color: themeColors['--color-brand'],
  },
  rule3: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '.25rem',
    padding: `calc(${themeLayout['--spacing']} * 1.5)`,
    color: themeColors['--color-foreground'],
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-list-hover-background'],
      },
    },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

// FloatingToolbarPlugin shows a floating toolbar when text is selected.
function FloatingToolbarPlugin() {
  const [editor] = useLexicalComposerContext()
  const [isVisible, setIsVisible] = useState(false)
  const [isBold, setIsBold] = useState(false)
  const [isItalic, setIsItalic] = useState(false)
  const [isCode, setIsCode] = useState(false)
  const [isLink, setIsLink] = useState(false)
  const [position, setPosition] = useState({ top: 0, left: 0 })
  const [linkDialogOpen, setLinkDialogOpen] = useState(false)
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

  const updateState = useEffectEvent(() => {
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
  })

  useEffect(() => {
    return editor.registerUpdateListener(() => {
      updateState()
    })
  }, [editor])

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
  }, [editor])

  const handleFormat = useCallback(
    (format: TextFormatType) => {
      editor.dispatchCommand(FORMAT_TEXT_COMMAND, format)
    },
    [editor],
  )

  const handleLink = useCallback(() => {
    if (isLink) {
      editor.dispatchCommand(TOGGLE_LINK_COMMAND, null)
      return
    }
    setLinkDialogOpen(true)
  }, [editor, isLink])

  const confirmLink = useCallback(
    (url: string) => {
      if (!/^https?:\/\//i.test(url)) return
      editor.dispatchCommand(TOGGLE_LINK_COMMAND, url)
      setLinkDialogOpen(false)
    },
    [editor],
  )

  if (!isVisible && !linkDialogOpen) return null

  return (
    <>
      {isVisible && (
        <div
          ref={floatingRef}
          sx={stylexStyles2.rule1}
          style={{
            '--floating-toolbar-top': `${position.top}px`,
            '--floating-toolbar-left': `${position.left}px`,
          }}
        >
          <button
            type="button"
            sx={isBold ? stylexStyles2.rule2 : stylexStyles2.rule3}
            onMouseDown={(e) => {
              e.preventDefault()
              handleFormat('bold')
            }}
            title="Bold"
          >
            <LuBold sx={stylexStyles.rule1} />
          </button>
          <button
            type="button"
            sx={isItalic ? stylexStyles2.rule2 : stylexStyles2.rule3}
            onMouseDown={(e) => {
              e.preventDefault()
              handleFormat('italic')
            }}
            title="Italic"
          >
            <LuItalic sx={stylexStyles.rule1} />
          </button>
          <button
            type="button"
            sx={isCode ? stylexStyles2.rule2 : stylexStyles2.rule3}
            onMouseDown={(e) => {
              e.preventDefault()
              handleFormat('code')
            }}
            title="Code"
          >
            <LuCode sx={stylexStyles.rule1} />
          </button>
          <button
            type="button"
            sx={isLink ? stylexStyles2.rule2 : stylexStyles2.rule3}
            onMouseDown={(e) => {
              e.preventDefault()
              handleLink()
            }}
            title="Link"
          >
            <LuLink sx={stylexStyles.rule1} />
          </button>
        </div>
      )}
      <TextInputDialog
        open={linkDialogOpen}
        title="Insert link"
        label="URL"
        placeholder="https://example.com"
        confirmLabel="Apply link"
        requireValue
        onOpenChange={setLinkDialogOpen}
        onConfirm={confirmLink}
      />
    </>
  )
}

export default FloatingToolbarPlugin
