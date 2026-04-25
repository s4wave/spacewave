import { useCallback, useEffect, useMemo, useState } from 'react'
import { useLexicalComposerContext } from '@lexical/react/LexicalComposerContext'
import {
  $getSelection,
  $isRangeSelection,
  $createParagraphNode,
  FORMAT_TEXT_COMMAND,
  UNDO_COMMAND,
  REDO_COMMAND,
  CAN_UNDO_COMMAND,
  CAN_REDO_COMMAND,
  COMMAND_PRIORITY_CRITICAL,
  type RangeSelection,
  type TextFormatType,
} from 'lexical'
import { $setBlocksType } from '@lexical/selection'
import {
  $createHeadingNode,
  $createQuoteNode,
  $isHeadingNode,
  type HeadingTagType,
} from '@lexical/rich-text'
import { $createCodeNode, $isCodeNode } from '@lexical/code'
import {
  INSERT_ORDERED_LIST_COMMAND,
  INSERT_UNORDERED_LIST_COMMAND,
  INSERT_CHECK_LIST_COMMAND,
  $isListNode,
  ListNode,
} from '@lexical/list'
import { $isLinkNode, TOGGLE_LINK_COMMAND } from '@lexical/link'
import { $findMatchingParent } from '@lexical/utils'
import { cn } from '@s4wave/web/style/utils.js'
import {
  LuUndo2,
  LuRedo2,
  LuBold,
  LuItalic,
  LuStrikethrough,
  LuCode,
  LuList,
  LuListOrdered,
  LuListChecks,
  LuLink,
  LuChevronDown,
} from 'react-icons/lu'

type BlockType =
  | 'paragraph'
  | 'h1'
  | 'h2'
  | 'h3'
  | 'h4'
  | 'quote'
  | 'code'
  | 'bullet'
  | 'number'
  | 'check'

const BLOCK_TYPE_LABELS: Record<BlockType, string> = {
  paragraph: 'Paragraph',
  h1: 'Heading 1',
  h2: 'Heading 2',
  h3: 'Heading 3',
  h4: 'Heading 4',
  quote: 'Quote',
  code: 'Code Block',
  bullet: 'Bullet List',
  number: 'Numbered List',
  check: 'Check List',
}

function getSelectedBlockType(selection: RangeSelection): BlockType {
  const node = selection.anchor.getNode()
  const parent = node.getParent()
  const topLevel = node.getTopLevelElementOrThrow()

  if ($isHeadingNode(topLevel)) {
    return topLevel.getTag() as BlockType
  }
  if ($isCodeNode(topLevel)) {
    return 'code'
  }
  if ($isListNode(topLevel)) {
    const listType = topLevel.getListType()
    if (listType === 'bullet') return 'bullet'
    if (listType === 'number') return 'number'
    if (listType === 'check') return 'check'
  }
  if (parent && $isListNode(parent)) {
    const listType = parent.getListType()
    if (listType === 'bullet') return 'bullet'
    if (listType === 'number') return 'number'
    if (listType === 'check') return 'check'
  }

  const listParent = $findMatchingParent(node, $isListNode)
  if (listParent instanceof ListNode) {
    const listType = listParent.getListType()
    if (listType === 'bullet') return 'bullet'
    if (listType === 'number') return 'number'
    if (listType === 'check') return 'check'
  }

  if (topLevel.getType() === 'quote') return 'quote'

  return 'paragraph'
}

// ToolbarPlugin renders the editor toolbar with formatting controls.
function ToolbarPlugin() {
  const [editor] = useLexicalComposerContext()
  const [isBold, setIsBold] = useState(false)
  const [isItalic, setIsItalic] = useState(false)
  const [isStrikethrough, setIsStrikethrough] = useState(false)
  const [isCode, setIsCode] = useState(false)
  const [isLink, setIsLink] = useState(false)
  const [blockType, setBlockType] = useState<BlockType>('paragraph')
  const [canUndo, setCanUndo] = useState(false)
  const [canRedo, setCanRedo] = useState(false)
  const [showBlockMenu, setShowBlockMenu] = useState(false)

  const updateToolbar = useCallback(() => {
    editor.getEditorState().read(() => {
      const selection = $getSelection()
      if (!$isRangeSelection(selection)) return

      setIsBold(selection.hasFormat('bold'))
      setIsItalic(selection.hasFormat('italic'))
      setIsStrikethrough(selection.hasFormat('strikethrough'))
      setIsCode(selection.hasFormat('code'))

      const node = selection.anchor.getNode()
      const parent = node.getParent()
      setIsLink($isLinkNode(parent) || $isLinkNode(node))

      setBlockType(getSelectedBlockType(selection))
    })
  }, [editor])

  useEffect(() => {
    return editor.registerUpdateListener(({ editorState }) => {
      editorState.read(() => {
        updateToolbar()
      })
    })
  }, [editor, updateToolbar])

  useEffect(() => {
    return editor.registerCommand(
      CAN_UNDO_COMMAND,
      (payload) => {
        setCanUndo(payload)
        return false
      },
      COMMAND_PRIORITY_CRITICAL,
    )
  }, [editor])

  useEffect(() => {
    return editor.registerCommand(
      CAN_REDO_COMMAND,
      (payload) => {
        setCanRedo(payload)
        return false
      },
      COMMAND_PRIORITY_CRITICAL,
    )
  }, [editor])

  const formatText = useCallback(
    (format: TextFormatType) => {
      editor.dispatchCommand(FORMAT_TEXT_COMMAND, format)
    },
    [editor],
  )

  const formatBlock = useCallback(
    (type: BlockType) => {
      editor.update(() => {
        const selection = $getSelection()
        if (!$isRangeSelection(selection)) return

        if (type === 'paragraph') {
          $setBlocksType(selection, $createParagraphNode)
        } else if (
          type === 'h1' ||
          type === 'h2' ||
          type === 'h3' ||
          type === 'h4'
        ) {
          $setBlocksType(selection, () =>
            $createHeadingNode(type as HeadingTagType),
          )
        } else if (type === 'quote') {
          $setBlocksType(selection, $createQuoteNode)
        } else if (type === 'code') {
          $setBlocksType(selection, $createCodeNode)
        } else if (type === 'bullet') {
          editor.dispatchCommand(INSERT_UNORDERED_LIST_COMMAND, undefined)
        } else if (type === 'number') {
          editor.dispatchCommand(INSERT_ORDERED_LIST_COMMAND, undefined)
        } else if (type === 'check') {
          editor.dispatchCommand(INSERT_CHECK_LIST_COMMAND, undefined)
        }
      })
      setShowBlockMenu(false)
    },
    [editor],
  )

  const insertLink = useCallback(() => {
    if (isLink) {
      editor.dispatchCommand(TOGGLE_LINK_COMMAND, null)
    } else {
      const url = window.prompt('Enter URL:')
      if (url && /^https?:\/\//i.test(url.trim())) {
        editor.dispatchCommand(TOGGLE_LINK_COMMAND, url.trim())
      }
    }
  }, [editor, isLink])

  const blockTypeOptions = useMemo(
    () =>
      (Object.keys(BLOCK_TYPE_LABELS) as BlockType[]).filter(
        (t) => t !== 'bullet' && t !== 'number' && t !== 'check',
      ),
    [],
  )

  const btnClass =
    'flex items-center justify-center rounded p-1 text-foreground-alt hover:bg-list-hover-background hover:text-foreground disabled:opacity-30'
  const activeClass = 'text-brand bg-brand/10'

  return (
    <div className="flex items-center gap-0.5 border-b border-border px-2 py-1">
      <button
        type="button"
        className={cn(btnClass, !canUndo && 'opacity-30')}
        onClick={() => editor.dispatchCommand(UNDO_COMMAND, undefined)}
        disabled={!canUndo}
        title="Undo"
      >
        <LuUndo2 className="h-3.5 w-3.5" />
      </button>
      <button
        type="button"
        className={cn(btnClass, !canRedo && 'opacity-30')}
        onClick={() => editor.dispatchCommand(REDO_COMMAND, undefined)}
        disabled={!canRedo}
        title="Redo"
      >
        <LuRedo2 className="h-3.5 w-3.5" />
      </button>

      <div className="mx-1 h-4 w-px bg-border" />

      <div className="relative">
        <button
          type="button"
          className={cn(btnClass, 'gap-1 px-2')}
          onClick={() => setShowBlockMenu(!showBlockMenu)}
          title="Block type"
        >
          <span className="text-foreground-alt/50 text-xs">
            {BLOCK_TYPE_LABELS[blockType]}
          </span>
          <LuChevronDown className="h-3 w-3" />
        </button>
        {showBlockMenu && (
          <div className="bg-popover border-border absolute left-0 top-full z-50 mt-1 min-w-[140px] rounded-lg border py-1 shadow-lg">
            {blockTypeOptions.map((type) => (
              <button
                key={type}
                type="button"
                className={cn(
                  'w-full px-3 py-1 text-left text-xs hover:bg-list-hover-background',
                  blockType === type && 'text-brand font-medium',
                )}
                onClick={() => formatBlock(type)}
              >
                {BLOCK_TYPE_LABELS[type]}
              </button>
            ))}
          </div>
        )}
      </div>

      <div className="mx-1 h-4 w-px bg-border" />

      <button
        type="button"
        className={cn(btnClass, isBold && activeClass)}
        onClick={() => formatText('bold')}
        title="Bold (Ctrl+B)"
      >
        <LuBold className="h-3.5 w-3.5" />
      </button>
      <button
        type="button"
        className={cn(btnClass, isItalic && activeClass)}
        onClick={() => formatText('italic')}
        title="Italic (Ctrl+I)"
      >
        <LuItalic className="h-3.5 w-3.5" />
      </button>
      <button
        type="button"
        className={cn(btnClass, isStrikethrough && activeClass)}
        onClick={() => formatText('strikethrough')}
        title="Strikethrough"
      >
        <LuStrikethrough className="h-3.5 w-3.5" />
      </button>
      <button
        type="button"
        className={cn(btnClass, isCode && activeClass)}
        onClick={() => formatText('code')}
        title="Inline Code"
      >
        <LuCode className="h-3.5 w-3.5" />
      </button>

      <div className="mx-1 h-4 w-px bg-border" />

      <button
        type="button"
        className={cn(btnClass, blockType === 'bullet' && activeClass)}
        onClick={() => formatBlock('bullet')}
        title="Bullet List"
      >
        <LuList className="h-3.5 w-3.5" />
      </button>
      <button
        type="button"
        className={cn(btnClass, blockType === 'number' && activeClass)}
        onClick={() => formatBlock('number')}
        title="Numbered List"
      >
        <LuListOrdered className="h-3.5 w-3.5" />
      </button>
      <button
        type="button"
        className={cn(btnClass, blockType === 'check' && activeClass)}
        onClick={() => formatBlock('check')}
        title="Check List"
      >
        <LuListChecks className="h-3.5 w-3.5" />
      </button>

      <div className="mx-1 h-4 w-px bg-border" />

      <button
        type="button"
        className={cn(btnClass, isLink && activeClass)}
        onClick={insertLink}
        title="Insert Link"
      >
        <LuLink className="h-3.5 w-3.5" />
      </button>
    </div>
  )
}

export default ToolbarPlugin
