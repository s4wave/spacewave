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
import { $convertFromMarkdownString, TRANSFORMERS } from '@lexical/markdown'
import type { Transformer } from '@lexical/markdown'

import editorTheme from './editor/theme.js'
import {
  SpacewaveEmbedNode,
  SPACEWAVE_EMBED_TRANSFORMER,
} from './editor/SpacewaveEmbedNode.js'
import ToolbarPlugin from './editor/ToolbarPlugin.js'
import FloatingToolbarPlugin from './editor/FloatingToolbarPlugin.js'
import SlashCommandPlugin from './editor/SlashCommandPlugin.js'
import TabIndentPlugin from './editor/TabIndentPlugin.js'
import CodeHighlightPlugin from './editor/CodeHighlightPlugin.js'
import SavePlugin from './editor/SavePlugin.js'
import EditorCommandsPlugin from './editor/EditorCommandsPlugin.js'

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
  TableNode,
  TableCellNode,
  TableRowNode,
]

interface LexicalEditorProps {
  markdown: string
  onSave: (markdown: string) => void
  composerKey?: string
}

// LexicalEditor is the WYSIWYG markdown editor using Lexical.
// Markdown is the source of truth: flash-imported on mount, flash-exported on save.
function LexicalEditor({ markdown, onSave, composerKey }: LexicalEditorProps) {
  // Remount the composer when markdown source changes externally.
  // The key ensures a fresh Lexical instance.
  const key = composerKey ?? markdown

  const handleSave = useCallback(
    (body: string) => {
      onSave(body)
    },
    [onSave],
  )

  const initialConfig = useMemo(
    () => ({
      namespace: 'SpacewaveNotes',
      nodes: EDITOR_NODES,
      theme: editorTheme,
      onError: (error: Error) => console.error('[LexicalEditor]', error),
      editorState: () => {
        $convertFromMarkdownString(markdown, ALL_TRANSFORMERS, undefined, true)
      },
    }),
    [markdown],
  )

  return (
    <LexicalComposer key={key} initialConfig={initialConfig}>
      <ToolbarPlugin />
      <div className="relative flex-1 overflow-auto">
        <RichTextPlugin
          contentEditable={
            <ContentEditable className="text-editor-foreground text-ui min-h-full p-4 outline-none" />
          }
          ErrorBoundary={LexicalErrorBoundary}
        />
        <HistoryPlugin />
        <ListPlugin />
        <CheckListPlugin />
        <LinkPlugin validateUrl={validateLinkUrl} />
        <TablePlugin />
        <HorizontalRulePlugin />
        <MarkdownShortcutPlugin transformers={ALL_TRANSFORMERS} />
        <TabIndentPlugin />
        <CodeHighlightPlugin />
        <FloatingToolbarPlugin />
        <SlashCommandPlugin />
        <EditorCommandsPlugin />
        <SavePlugin transformers={ALL_TRANSFORMERS} onSave={handleSave} />
      </div>
    </LexicalComposer>
  )
}

export default LexicalEditor
