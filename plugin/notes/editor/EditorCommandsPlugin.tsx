import { useCallback } from 'react'
import { useLexicalComposerContext } from '@lexical/react/LexicalComposerContext'
import {
  $getSelection,
  $isRangeSelection,
} from 'lexical'
import { $setBlocksType } from '@lexical/selection'
import { $createHeadingNode, $createQuoteNode } from '@lexical/rich-text'
import { $createCodeNode } from '@lexical/code'
import {
  INSERT_UNORDERED_LIST_COMMAND,
  INSERT_ORDERED_LIST_COMMAND,
  INSERT_CHECK_LIST_COMMAND,
} from '@lexical/list'
import { TOGGLE_LINK_COMMAND } from '@lexical/link'
import { INSERT_TABLE_COMMAND } from '@lexical/table'
import { INSERT_HORIZONTAL_RULE_COMMAND } from '@lexical/react/LexicalHorizontalRuleNode'

import { useCommand } from '@s4wave/web/command/useCommand.js'
import { useIsTabActive } from '@s4wave/web/contexts/TabActiveContext.js'

// EditorCommandsPlugin bridges Lexical formatting operations to the
// app command system. Active when a notebook tab is focused.
export default function EditorCommandsPlugin() {
  const [editor] = useLexicalComposerContext()
  const isTabActive = useIsTabActive()

  // Headings H1-H4.
  useCommand({
    commandId: 'notes.format.heading-1',
    label: 'Heading 1',
    menuPath: 'Edit/Heading/H1',
    menuGroup: 50,
    menuOrder: 1,
    active: isTabActive,
    handler: useCallback(() => {
      editor.update(() => {
        const selection = $getSelection()
        if ($isRangeSelection(selection)) {
          $setBlocksType(selection, () => $createHeadingNode('h1'))
        }
      })
    }, [editor]),
  })

  useCommand({
    commandId: 'notes.format.heading-2',
    label: 'Heading 2',
    menuPath: 'Edit/Heading/H2',
    menuGroup: 50,
    menuOrder: 2,
    active: isTabActive,
    handler: useCallback(() => {
      editor.update(() => {
        const selection = $getSelection()
        if ($isRangeSelection(selection)) {
          $setBlocksType(selection, () => $createHeadingNode('h2'))
        }
      })
    }, [editor]),
  })

  useCommand({
    commandId: 'notes.format.heading-3',
    label: 'Heading 3',
    menuPath: 'Edit/Heading/H3',
    menuGroup: 50,
    menuOrder: 3,
    active: isTabActive,
    handler: useCallback(() => {
      editor.update(() => {
        const selection = $getSelection()
        if ($isRangeSelection(selection)) {
          $setBlocksType(selection, () => $createHeadingNode('h3'))
        }
      })
    }, [editor]),
  })

  useCommand({
    commandId: 'notes.format.heading-4',
    label: 'Heading 4',
    menuPath: 'Edit/Heading/H4',
    menuGroup: 50,
    menuOrder: 4,
    active: isTabActive,
    handler: useCallback(() => {
      editor.update(() => {
        const selection = $getSelection()
        if ($isRangeSelection(selection)) {
          $setBlocksType(selection, () => $createHeadingNode('h4'))
        }
      })
    }, [editor]),
  })

  // Lists.
  useCommand({
    commandId: 'notes.format.bullet-list',
    label: 'Bullet List',
    menuPath: 'Edit/List/Bullet List',
    menuGroup: 51,
    menuOrder: 1,
    active: isTabActive,
    handler: useCallback(() => {
      editor.dispatchCommand(INSERT_UNORDERED_LIST_COMMAND, undefined)
    }, [editor]),
  })

  useCommand({
    commandId: 'notes.format.numbered-list',
    label: 'Numbered List',
    menuPath: 'Edit/List/Numbered List',
    menuGroup: 51,
    menuOrder: 2,
    active: isTabActive,
    handler: useCallback(() => {
      editor.dispatchCommand(INSERT_ORDERED_LIST_COMMAND, undefined)
    }, [editor]),
  })

  useCommand({
    commandId: 'notes.format.check-list',
    label: 'Check List',
    menuPath: 'Edit/List/Check List',
    menuGroup: 51,
    menuOrder: 3,
    active: isTabActive,
    handler: useCallback(() => {
      editor.dispatchCommand(INSERT_CHECK_LIST_COMMAND, undefined)
    }, [editor]),
  })

  // Block formats.
  useCommand({
    commandId: 'notes.format.code-block',
    label: 'Code Block',
    menuPath: 'Edit/Code Block',
    menuGroup: 52,
    menuOrder: 1,
    active: isTabActive,
    handler: useCallback(() => {
      editor.update(() => {
        const selection = $getSelection()
        if ($isRangeSelection(selection)) {
          $setBlocksType(selection, () => $createCodeNode())
        }
      })
    }, [editor]),
  })

  useCommand({
    commandId: 'notes.format.quote',
    label: 'Quote',
    menuPath: 'Edit/Quote',
    menuGroup: 52,
    menuOrder: 2,
    active: isTabActive,
    handler: useCallback(() => {
      editor.update(() => {
        const selection = $getSelection()
        if ($isRangeSelection(selection)) {
          $setBlocksType(selection, () => $createQuoteNode())
        }
      })
    }, [editor]),
  })

  // Insert commands.
  useCommand({
    commandId: 'notes.insert.link',
    label: 'Insert Link',
    keybinding: 'CmdOrCtrl+K',
    menuPath: 'Edit/Insert Link',
    menuGroup: 53,
    menuOrder: 1,
    active: isTabActive,
    handler: useCallback(() => {
      editor.dispatchCommand(TOGGLE_LINK_COMMAND, 'https://')
    }, [editor]),
  })

  useCommand({
    commandId: 'notes.insert.table',
    label: 'Insert Table',
    menuPath: 'Edit/Insert Table',
    menuGroup: 53,
    menuOrder: 2,
    active: isTabActive,
    handler: useCallback(() => {
      editor.dispatchCommand(INSERT_TABLE_COMMAND, {
        columns: '3',
        rows: '3',
        includeHeaders: true,
      })
    }, [editor]),
  })

  useCommand({
    commandId: 'notes.insert.horizontal-rule',
    label: 'Horizontal Rule',
    menuPath: 'Edit/Horizontal Rule',
    menuGroup: 53,
    menuOrder: 3,
    active: isTabActive,
    handler: useCallback(() => {
      editor.dispatchCommand(INSERT_HORIZONTAL_RULE_COMMAND, undefined)
    }, [editor]),
  })

  return null
}
