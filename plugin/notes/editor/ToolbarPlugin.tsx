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
} from '@s4wave/web/images/icons.js'

import { TextInputDialog } from '../NoteDialogs.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
  radii as themeRadii,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    minWidth: 0,
    flexWrap: {
      default: 'wrap',
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 'nowrap',
      },
    },
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * .5)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: themeColors['--color-border'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
  },
  rule2: {
    display: 'flex',
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 0,
      },
    },
    minWidth: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 0,
      },
    },
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '.25rem',
    padding: themeLayout['--spacing'],
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    opacity: { default: 0.3, ':disabled': 0.3 },
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-list-hover-background'],
      },
    },
  },
  rule3: {
    display: 'flex',
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 0,
      },
    },
    minWidth: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 0,
      },
    },
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '.25rem',
    padding: themeLayout['--spacing'],
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-list-hover-background'],
      },
    },
    opacity: { default: null, ':disabled': 0.3 },
  },
  rule4: {
    marginInline: themeLayout['--spacing'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: '1px',
    backgroundColor: themeColors['--color-border'],
  },
  rule5: { position: 'relative' },
  rule6: {
    gap: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule7: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule8: {
    position: 'absolute',
    top: '100%',
    left: 0,
    zIndex: 50,
    marginTop: themeLayout['--spacing'],
    minWidth: `calc(${themeLayout['--spacing']} * 35)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: themeColors['--color-border'],
    backgroundColor: themeColors['--color-popover'],
    paddingBlock: themeLayout['--spacing'],
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    maxHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 24)`,
    },
    overflowY: { default: null, '@media (pointer: coarse)': 'auto' },
  },
  rule9: {
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 0,
      },
    },
    width: '100%',
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: themeLayout['--spacing'],
    textAlign: 'left',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-list-hover-background'],
      },
    },
  },
  rule10: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-brand'],
  },
  rule11: {
    display: 'flex',
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 0,
      },
    },
    minWidth: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 0,
      },
    },
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
    padding: themeLayout['--spacing'],
    color: {
      default: themeColors['--color-brand'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    opacity: { default: null, ':disabled': 0.3 },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

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

/** ToolbarPlugin renders the editor toolbar with formatting controls. */
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
  const [linkDialogOpen, setLinkDialogOpen] = useState(false)

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
      setLinkDialogOpen(true)
    }
  }, [editor, isLink])

  const confirmLink = useCallback(
    (url: string) => {
      if (!/^https?:\/\//i.test(url)) return
      editor.dispatchCommand(TOGGLE_LINK_COMMAND, url)
      setLinkDialogOpen(false)
    },
    [editor],
  )

  const blockTypeOptions = useMemo(
    () =>
      (Object.keys(BLOCK_TYPE_LABELS) as BlockType[]).filter(
        (t) => t !== 'bullet' && t !== 'number' && t !== 'check',
      ),
    [],
  )

  return (
    <>
      <div sx={stylexStyles2.rule1}>
        <button
          type="button"
          sx={!canUndo ? stylexStyles2.rule2 : stylexStyles2.rule3}
          onClick={() => editor.dispatchCommand(UNDO_COMMAND, undefined)}
          disabled={!canUndo}
          title="Undo"
        >
          <LuUndo2 sx={stylexStyles.rule1} />
        </button>
        <button
          type="button"
          sx={!canRedo ? stylexStyles2.rule2 : stylexStyles2.rule3}
          onClick={() => editor.dispatchCommand(REDO_COMMAND, undefined)}
          disabled={!canRedo}
          title="Redo"
        >
          <LuRedo2 sx={stylexStyles.rule1} />
        </button>

        <div sx={stylexStyles2.rule4} />

        <div sx={stylexStyles2.rule5}>
          <button
            type="button"
            sx={[stylexStyles2.rule3, stylexStyles2.rule6]}
            onClick={() => setShowBlockMenu(!showBlockMenu)}
            title="Block type"
          >
            <span sx={stylexStyles2.rule7}>{BLOCK_TYPE_LABELS[blockType]}</span>
            <LuChevronDown sx={stylexStyles.rule2} />
          </button>
          {showBlockMenu && (
            <div sx={stylexStyles2.rule8}>
              {blockTypeOptions.map((type) => (
                <button
                  key={type}
                  type="button"
                  sx={[
                    stylexStyles2.rule9,
                    blockType === type && stylexStyles2.rule10,
                  ]}
                  onClick={() => formatBlock(type)}
                >
                  {BLOCK_TYPE_LABELS[type]}
                </button>
              ))}
            </div>
          )}
        </div>

        <div sx={stylexStyles2.rule4} />

        <button
          type="button"
          sx={isBold ? stylexStyles2.rule11 : stylexStyles2.rule3}
          onClick={() => formatText('bold')}
          title="Bold (Ctrl+B)"
        >
          <LuBold sx={stylexStyles.rule1} />
        </button>
        <button
          type="button"
          sx={isItalic ? stylexStyles2.rule11 : stylexStyles2.rule3}
          onClick={() => formatText('italic')}
          title="Italic (Ctrl+I)"
        >
          <LuItalic sx={stylexStyles.rule1} />
        </button>
        <button
          type="button"
          sx={isStrikethrough ? stylexStyles2.rule11 : stylexStyles2.rule3}
          onClick={() => formatText('strikethrough')}
          title="Strikethrough"
        >
          <LuStrikethrough sx={stylexStyles.rule1} />
        </button>
        <button
          type="button"
          sx={isCode ? stylexStyles2.rule11 : stylexStyles2.rule3}
          onClick={() => formatText('code')}
          title="Inline Code"
        >
          <LuCode sx={stylexStyles.rule1} />
        </button>

        <div sx={stylexStyles2.rule4} />

        <button
          type="button"
          sx={
            blockType === 'bullet' ? stylexStyles2.rule11 : stylexStyles2.rule3
          }
          onClick={() => formatBlock('bullet')}
          title="Bullet List"
        >
          <LuList sx={stylexStyles.rule1} />
        </button>
        <button
          type="button"
          sx={
            blockType === 'number' ? stylexStyles2.rule11 : stylexStyles2.rule3
          }
          onClick={() => formatBlock('number')}
          title="Numbered List"
        >
          <LuListOrdered sx={stylexStyles.rule1} />
        </button>
        <button
          type="button"
          sx={
            blockType === 'check' ? stylexStyles2.rule11 : stylexStyles2.rule3
          }
          onClick={() => formatBlock('check')}
          title="Check List"
        >
          <LuListChecks sx={stylexStyles.rule1} />
        </button>

        <div sx={stylexStyles2.rule4} />

        <button
          type="button"
          sx={isLink ? stylexStyles2.rule11 : stylexStyles2.rule3}
          onClick={insertLink}
          title="Insert Link"
        >
          <LuLink sx={stylexStyles.rule1} />
        </button>
      </div>
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

export default ToolbarPlugin
