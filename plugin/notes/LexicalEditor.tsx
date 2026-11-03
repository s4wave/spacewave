import { useCallback, useMemo } from 'react'
import { LexicalComposer } from '@lexical/react/LexicalComposer'
import { RichTextPlugin } from '@lexical/react/LexicalRichTextPlugin'
import { ContentEditable } from '@lexical/react/LexicalContentEditable'
import { HistoryPlugin } from '@lexical/react/LexicalHistoryPlugin'
import { ListPlugin } from '@lexical/react/LexicalListPlugin'
import { CheckListPlugin } from '@lexical/react/LexicalCheckListPlugin'
import { LinkPlugin } from '@lexical/react/LexicalLinkPlugin'
import { MarkdownShortcutPlugin } from '@lexical/react/LexicalMarkdownShortcutPlugin'
import { TablePlugin } from '@lexical/react/LexicalTablePlugin'
import { HorizontalRulePlugin } from '@lexical/react/LexicalHorizontalRulePlugin'
import { LexicalErrorBoundary } from '@lexical/react/LexicalErrorBoundary'
import { HeadingNode, QuoteNode } from '@lexical/rich-text'
import { CodeNode, CodeHighlightNode } from '@lexical/code'
import { LinkNode, AutoLinkNode } from '@lexical/link'
import { ListNode, ListItemNode } from '@lexical/list'
import { HorizontalRuleNode } from '@lexical/react/LexicalHorizontalRuleNode'
import { TableNode, TableCellNode, TableRowNode } from '@lexical/table'
import {
  $convertFromMarkdownString,
  $convertToMarkdownString,
  TRANSFORMERS,
} from '@lexical/markdown'
import type { Transformer } from '@lexical/markdown'
import { CommandFocusContext } from '@s4wave/sdk/command/command.pb.js'

import { FocusContextProvider } from '@s4wave/web/command/FocusContext.js'

import editorTheme from './editor/theme.js'
import {
  SpacewaveEmbedNode,
  SPACEWAVE_EMBED_TRANSFORMER,
} from './editor/SpacewaveEmbedNode.js'
import { OrgPassthroughNode } from './editor/OrgPassthroughNode.js'
import ToolbarPlugin from './editor/ToolbarPlugin.js'
import FloatingToolbarPlugin from './editor/FloatingToolbarPlugin.js'
import SlashCommandPlugin from './editor/SlashCommandPlugin.js'
import TabIndentPlugin from './editor/TabIndentPlugin.js'
import CodeHighlightPlugin from './editor/CodeHighlightPlugin.js'
import SavePlugin from './editor/SavePlugin.js'
import EditorCommandsPlugin from './editor/EditorCommandsPlugin.js'
import { $convertFromOrgString, $convertToOrgString } from './org/lexical.js'
import type { NoteFileFormat } from './note-files.js'

import * as stylex from '@stylexjs/stylex'

import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    '--tw-outline-style': 'none',
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-brand'],
    },
    '--tw-ring-inset': { default: null, ':focus-visible': 'inset' },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    color: themeColors['--color-editor-foreground'],
    fontSize: themeLayout['--text-ui'],
    letterSpacing: `var(--tw-tracking, ${themeLayout['--text-ui--letter-spacing']})`,
    lineHeight: `var(--tw-leading, ${themeLayout['--text-ui--line-height']})`,
    outlineStyle: 'none',
    minHeight: '100%',
  },
})

const stylexStyles = stylex.create({
  rule1: { flex: 1, overflow: 'auto', position: 'relative', },
})

// validateLinkUrl rejects dangerous URL schemes (javascript:, data:, vbscript:).
function validateLinkUrl(url: string): boolean {
  const trimmed = url.trim()
  if (trimmed.length === 0) return false
  const lower = trimmed.toLowerCase()
  if (
    lower.startsWith('javascript:') ||
    lower.startsWith('data:') ||
    lower.startsWith('vbscript:')
  ) {
    return false
  }
  return true
}

const ALL_TRANSFORMERS: Transformer[] = [
  ...TRANSFORMERS,
  SPACEWAVE_EMBED_TRANSFORMER,
]

const EDITOR_NODES = [
  HeadingNode,
  QuoteNode,
  CodeNode,
  CodeHighlightNode,
  LinkNode,
  AutoLinkNode,
  ListNode,
  ListItemNode,
  HorizontalRuleNode,
  SpacewaveEmbedNode,
  OrgPassthroughNode,
  TableNode,
  TableCellNode,
  TableRowNode,
]

interface LexicalEditorProps {
  content: string
  format: NoteFileFormat
  onSave: (content: string) => void | Promise<void>
  onDraftChange?: (content: string) => void
  onDirty?: () => void
  composerKey?: string
}

// LexicalEditor is the WYSIWYG note editor using Lexical.
// The source file text is imported on mount and exported on save.
function LexicalEditor({
  content,
  format,
  onSave,
  onDraftChange,
  onDirty,
  composerKey,
}: LexicalEditorProps) {
  // Remount the composer when the source changes externally.
  // The key ensures a fresh Lexical instance.
  const key = composerKey ?? `${format}:${content}`

  const handleSave = useCallback((body: string) => onSave(body), [onSave])

  const exportContent = useCallback(() => {
    return format === 'org'
      ? $convertToOrgString()
      : $convertToMarkdownString(ALL_TRANSFORMERS, undefined, true)
  }, [format])

  const initialConfig = useMemo(
    () => ({
      namespace: 'SpacewaveNotes',
      nodes: EDITOR_NODES,
      theme: editorTheme,
      onError: (error: Error) => console.error('[LexicalEditor]', error),
      editorState: () => {
        if (format === 'org') {
          $convertFromOrgString(content)
          return
        }
        $convertFromMarkdownString(content, ALL_TRANSFORMERS, undefined, true)
      },
    }),
    [content, format],
  )

  return (
    <LexicalComposer key={key} initialConfig={initialConfig}>
      <ToolbarPlugin />
      <FocusContextProvider
        focusContext={CommandFocusContext.EDITOR}
        sx={stylexStyles.rule1}
      >
        <RichTextPlugin
          contentEditable={
            <ContentEditable
              aria-label="Note editor"
              {...stylex.props(stylexStyles2.rule1)}
            />
          }
          ErrorBoundary={LexicalErrorBoundary}
        />
        <HistoryPlugin />
        <ListPlugin />
        <CheckListPlugin />
        <LinkPlugin validateUrl={validateLinkUrl} />
        <TablePlugin />
        <HorizontalRulePlugin />
        {format === 'markdown' && (
          <MarkdownShortcutPlugin transformers={ALL_TRANSFORMERS} />
        )}
        <TabIndentPlugin />
        <CodeHighlightPlugin />
        <FloatingToolbarPlugin />
        <SlashCommandPlugin />
        <EditorCommandsPlugin />
        <SavePlugin
          savedContent={content}
          exportString={exportContent}
          onSave={handleSave}
          onDraftChange={onDraftChange}
          onDirty={onDirty}
        />
      </FocusContextProvider>
    </LexicalComposer>
  )
}

export default LexicalEditor
