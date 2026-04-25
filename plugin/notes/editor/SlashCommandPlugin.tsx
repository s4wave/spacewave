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
import { cn } from '@s4wave/web/style/utils.js'

type SlashMenuItem = {
  title: string
  description: string
  icon: string
  action: () => void
}

// SlashCommandPlugin provides a / menu for inserting block elements.
function SlashCommandPlugin() {
  const [editor] = useLexicalComposerContext()
  const [isOpen, setIsOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [selectedIndex, setSelectedIndex] = useState(0)
  const [position, setPosition] = useState({ top: 0, left: 0 })
  const menuRef = useRef<HTMLDivElement>(null)

  const menuItems: SlashMenuItem[] = useMemo(
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
          const path = window.prompt('Enter object path:')
          if (path) {
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
          }
        },
      },
    ],
    [editor],
  )

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

  if (!isOpen || filteredItems.length === 0) return null

  return createPortal(
    <div
      ref={menuRef}
      className="bg-popover border-border fixed z-[200] min-w-[220px] rounded-lg border py-1 shadow-lg"
      style={{
        top: position.top,
        left: position.left,
      }}
    >
      {filteredItems.map((item, index) => (
        <button
          key={item.title}
          type="button"
          className={cn(
            'flex w-full items-center gap-3 px-3 py-1.5 text-left',
            index === selectedIndex && 'bg-list-active-selection-background',
          )}
          onMouseDown={(e) => {
            e.preventDefault()
            executeItem(index)
          }}
          onMouseEnter={() => setSelectedIndex(index)}
        >
          <span className="text-muted-foreground flex h-6 w-6 items-center justify-center text-xs font-medium">
            {item.icon}
          </span>
          <span className="flex flex-col">
            <span className="text-foreground text-xs">{item.title}</span>
            <span className="text-muted-foreground text-xs">
              {item.description}
            </span>
          </span>
        </button>
      ))}
    </div>,
    document.body,
  )
}

export default SlashCommandPlugin
