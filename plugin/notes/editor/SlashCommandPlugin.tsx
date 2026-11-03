import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { useLexicalComposerContext } from '@lexical/react/LexicalComposerContext'
import {
  $getSelection,
  $isRangeSelection,
  $isTextNode,
  $createParagraphNode,
  COMMAND_PRIORITY_HIGH,
  KEY_ARROW_DOWN_COMMAND,
  KEY_ARROW_UP_COMMAND,
  KEY_ENTER_COMMAND,
  KEY_ESCAPE_COMMAND,
  KEY_TAB_COMMAND,
  type LexicalEditor,
} from 'lexical'
import { $setBlocksType } from '@lexical/selection'
import { $createHeadingNode, $createQuoteNode } from '@lexical/rich-text'
import { $createCodeNode } from '@lexical/code'
import {
  INSERT_ORDERED_LIST_COMMAND,
  INSERT_UNORDERED_LIST_COMMAND,
  INSERT_CHECK_LIST_COMMAND,
} from '@lexical/list'
import { $createHorizontalRuleNode } from '@lexical/react/LexicalHorizontalRuleNode'
import { INSERT_TABLE_COMMAND } from '@lexical/table'
import { SpacewaveEmbedNode } from './SpacewaveEmbedNode.js'

import { TextInputDialog } from '../NoteDialogs.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  radii as themeRadii,
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    borderColor: themeColors['--color-border'],
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: themeLayout['--spacing'],
    backgroundColor: themeColors['--color-popover'],
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    position: 'fixed',
    zIndex: 200,
    left: 'var(--slash-command-left)',
    minWidth: `calc(${themeLayout['--spacing']} * 55)`,
    top: 'var(--slash-command-top)',
  },
  rule2: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    textAlign: 'left',
    width: '100%',
  },
  rule3: {
    backgroundColor: themeColors['--color-list-active-selection-background'],
  },
  rule4: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    alignItems: 'center',
    color: themeColors['--color-muted-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    height: `calc(${themeLayout['--spacing']} * 6)`,
    width: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule5: { display: 'flex', flexDirection: 'column' },
  rule6: {
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule7: {
    color: themeColors['--color-muted-foreground'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
})

type SlashMenuItem = {
  title: string
  description: string
  icon: string
  action: () => void
}

// useSlashMenuItems defines the editor mutations represented by the slash menu.
function useSlashMenuItems(
  editor: LexicalEditor,
  onInsertEmbed: () => void,
): SlashMenuItem[] {
  return useMemo(
    () => [
      {
        title: 'Heading 1',
        description: 'Large section heading',
        icon: 'H1',
        action: () => {
          editor.update(() => {
            const selection = $getSelection()
            if ($isRangeSelection(selection)) {
              $setBlocksType(selection, () => $createHeadingNode('h1'))
            }
          })
        },
      },
      {
        title: 'Heading 2',
        description: 'Medium section heading',
        icon: 'H2',
        action: () => {
          editor.update(() => {
            const selection = $getSelection()
            if ($isRangeSelection(selection)) {
              $setBlocksType(selection, () => $createHeadingNode('h2'))
            }
          })
        },
      },
      {
        title: 'Heading 3',
        description: 'Small section heading',
        icon: 'H3',
        action: () => {
          editor.update(() => {
            const selection = $getSelection()
            if ($isRangeSelection(selection)) {
              $setBlocksType(selection, () => $createHeadingNode('h3'))
            }
          })
        },
      },
      {
        title: 'Bullet List',
        description: 'Unordered list with bullets',
        icon: '\u2022',
        action: () => {
          editor.dispatchCommand(INSERT_UNORDERED_LIST_COMMAND, undefined)
        },
      },
      {
        title: 'Numbered List',
        description: 'Ordered list with numbers',
        icon: '1.',
        action: () => {
          editor.dispatchCommand(INSERT_ORDERED_LIST_COMMAND, undefined)
        },
      },
      {
        title: 'Check List',
        description: 'To-do list with checkboxes',
        icon: '\u2611',
        action: () => {
          editor.dispatchCommand(INSERT_CHECK_LIST_COMMAND, undefined)
        },
      },
      {
        title: 'Quote',
        description: 'Block quotation',
        icon: '\u201C',
        action: () => {
          editor.update(() => {
            const selection = $getSelection()
            if ($isRangeSelection(selection)) {
              $setBlocksType(selection, $createQuoteNode)
            }
          })
        },
      },
      {
        title: 'Code Block',
        description: 'Fenced code block',
        icon: '{}',
        action: () => {
          editor.update(() => {
            const selection = $getSelection()
            if ($isRangeSelection(selection)) {
              $setBlocksType(selection, $createCodeNode)
            }
          })
        },
      },
      {
        title: 'Horizontal Rule',
        description: 'Divider line',
        icon: '\u2500',
        action: () => {
          editor.update(() => {
            const selection = $getSelection()
            if ($isRangeSelection(selection)) {
              const node = selection.anchor.getNode()
              const topLevel = node.getTopLevelElementOrThrow()
              const hrNode = $createHorizontalRuleNode()
              topLevel.insertAfter(hrNode)
              const paragraph = $createParagraphNode()
              hrNode.insertAfter(paragraph)
              paragraph.selectStart()
            }
          })
        },
      },
      {
        title: 'Table',
        description: 'Insert a table (3x3)',
        icon: '\u229E',
        action: () => {
          editor.dispatchCommand(INSERT_TABLE_COMMAND, {
            columns: '3',
            rows: '3',
            includeHeaders: true,
          })
        },
      },
      {
        title: 'Spacewave Embed',
        description: 'Embed a Space object',
        icon: '\u25C6',
        action: () => {
          onInsertEmbed()
        },
      },
    ],
    [editor, onInsertEmbed],
  )
}

// SlashCommandPlugin provides a / menu for inserting block elements.
function SlashCommandPlugin() {
  const [editor] = useLexicalComposerContext()
  const [isOpen, setIsOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [selectedIndex, setSelectedIndex] = useState(0)
  const [position, setPosition] = useState({ top: 0, left: 0 })
  const [embedDialogOpen, setEmbedDialogOpen] = useState(false)
  const menuRef = useRef<HTMLDivElement>(null)

  const insertEmbed = useCallback(
    (path: string) => {
      if (!path) return
      editor.update(() => {
        const selection = $getSelection()
        if ($isRangeSelection(selection)) {
          const node = selection.anchor.getNode()
          const topLevel = node.getTopLevelElementOrThrow()
          const embedNode = new SpacewaveEmbedNode(path)
          topLevel.insertAfter(embedNode)
          const paragraph = $createParagraphNode()
          embedNode.insertAfter(paragraph)
          paragraph.selectStart()
        }
      })
      setEmbedDialogOpen(false)
    },
    [editor],
  )

  const openEmbedDialog = useCallback(() => {
    setEmbedDialogOpen(true)
  }, [])
  const menuItems = useSlashMenuItems(editor, openEmbedDialog)

  const filteredItems = useMemo(() => {
    if (!query) return menuItems
    const lower = query.toLowerCase()
    return menuItems.filter(
      (item) =>
        item.title.toLowerCase().includes(lower) ||
        item.description.toLowerCase().includes(lower),
    )
  }, [menuItems, query])

  const close = useCallback(() => {
    setIsOpen(false)
    setQuery('')
    setSelectedIndex(0)
  }, [])

  const executeItem = useCallback(
    (index: number) => {
      const item = filteredItems[index]
      if (!item) return

      editor.update(() => {
        const selection = $getSelection()
        if ($isRangeSelection(selection)) {
          const node = selection.anchor.getNode()
          const textContent = node.getTextContent()

          if (textContent.startsWith('/') && $isTextNode(node)) {
            const parent = node.getParent()
            if (
              parent &&
              parent.getChildrenSize() === 1 &&
              textContent.trimEnd() === '/' + query
            ) {
              node.setTextContent('')
            }
          }
        }
      })

      close()
      item.action()
    },
    [editor, filteredItems, query, close],
  )

  useEffect(() => {
    return editor.registerUpdateListener(({ editorState }) => {
      editorState.read(() => {
        const selection = $getSelection()
        if (!$isRangeSelection(selection) || !selection.isCollapsed()) {
          if (isOpen) close()
          return
        }

        const node = selection.anchor.getNode()
        const textContent = node.getTextContent()
        const offset = selection.anchor.offset

        const textUpToCursor = textContent.slice(0, offset)
        const slashIndex = textUpToCursor.lastIndexOf('/')

        if (
          slashIndex === -1 ||
          (slashIndex > 0 &&
            textUpToCursor[slashIndex - 1] !== ' ' &&
            textUpToCursor[slashIndex - 1] !== '\n')
        ) {
          if (slashIndex !== 0) {
            if (isOpen) close()
            return
          }
        }

        const queryText = textUpToCursor.slice(slashIndex + 1)

        if (queryText.includes(' ') && queryText.length > 20) {
          if (isOpen) close()
          return
        }

        setQuery(queryText)
        setSelectedIndex(0)

        const nativeSelection = window.getSelection()
        if (nativeSelection && nativeSelection.rangeCount > 0) {
          const range = nativeSelection.getRangeAt(0)
          const rect = range.getBoundingClientRect()
          setPosition({
            top: rect.bottom + 4 + window.scrollY,
            left: rect.left + window.scrollX,
          })
        }

        setIsOpen(true)
      })
    })
  }, [editor, isOpen, close])

  useEffect(() => {
    if (!isOpen) return

    const removeDown = editor.registerCommand(
      KEY_ARROW_DOWN_COMMAND,
      (event) => {
        event.preventDefault()
        setSelectedIndex((prev) => (prev + 1) % filteredItems.length)
        return true
      },
      COMMAND_PRIORITY_HIGH,
    )

    const removeUp = editor.registerCommand(
      KEY_ARROW_UP_COMMAND,
      (event) => {
        event.preventDefault()
        setSelectedIndex(
          (prev) => (prev - 1 + filteredItems.length) % filteredItems.length,
        )
        return true
      },
      COMMAND_PRIORITY_HIGH,
    )

    const removeEnter = editor.registerCommand(
      KEY_ENTER_COMMAND,
      (event) => {
        if (event) event.preventDefault()
        executeItem(selectedIndex)
        return true
      },
      COMMAND_PRIORITY_HIGH,
    )

    const removeTab = editor.registerCommand(
      KEY_TAB_COMMAND,
      (event) => {
        event.preventDefault()
        executeItem(selectedIndex)
        return true
      },
      COMMAND_PRIORITY_HIGH,
    )

    const removeEscape = editor.registerCommand(
      KEY_ESCAPE_COMMAND,
      () => {
        close()
        return true
      },
      COMMAND_PRIORITY_HIGH,
    )

    return () => {
      removeDown()
      removeUp()
      removeEnter()
      removeTab()
      removeEscape()
    }
  }, [editor, isOpen, filteredItems.length, selectedIndex, executeItem, close])

  if ((!isOpen || filteredItems.length === 0) && !embedDialogOpen) return null

  return (
    <>
      {isOpen && filteredItems.length > 0
        ? createPortal(
            <div
              ref={menuRef}
              sx={stylexStyles.rule1}
              style={{
                '--slash-command-top': `${position.top}px`,
                '--slash-command-left': `${position.left}px`,
              }}
            >
              {filteredItems.map((item, index) => (
                <button
                  key={item.title}
                  type="button"
                  sx={[
                    stylexStyles.rule2,
                    index === selectedIndex && stylexStyles.rule3,
                  ]}
                  onMouseDown={(e) => {
                    e.preventDefault()
                    executeItem(index)
                  }}
                  onMouseEnter={() => setSelectedIndex(index)}
                >
                  <span sx={stylexStyles.rule4}>{item.icon}</span>
                  <span sx={stylexStyles.rule5}>
                    <span sx={stylexStyles.rule6}>{item.title}</span>
                    <span sx={stylexStyles.rule7}>{item.description}</span>
                  </span>
                </button>
              ))}
            </div>,
            document.body,
          )
        : null}
      <TextInputDialog
        open={embedDialogOpen}
        title="Insert Spacewave embed"
        label="Object path"
        placeholder="object/key"
        confirmLabel="Insert embed"
        requireValue
        onOpenChange={setEmbedDialogOpen}
        onConfirm={insertEmbed}
      />
    </>
  )
}

export default SlashCommandPlugin
