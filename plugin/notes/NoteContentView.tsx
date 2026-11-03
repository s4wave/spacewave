import { Component, useMemo, type ErrorInfo, type ReactNode } from 'react'
import { LuCode, LuPenLine } from '@s4wave/web/images/icons.js'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { parseObjectUri } from '@s4wave/sdk/space/object-uri.js'
import {
  useUnixFSRootHandle,
  useUnixFSHandle,
  useUnixFSHandleTextContent,
} from '@s4wave/web/hooks/useUnixFSHandle.js'

import FrontmatterDisplay from './FrontmatterDisplay.js'
import LexicalEditor from './LexicalEditor.js'
import { getNoteFileFormat, stripNoteFileExtension } from './note-files.js'
import { useNoteWrite } from './useNoteWrite.js'
import type { parseNote } from './frontmatter.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    minWidth: 0,
    flexWrap: 'wrap',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule2: { minWidth: 0, flex: 1, overflowWrap: 'break-word' },
  rule3: {
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
    flexShrink: 0,
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule4: {
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: themeColors['--color-border'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule5: { color: themeColors['--color-success'] },
  rule6: { color: themeColors['--color-muted-foreground'] },
  rule7: {
    display: 'flex',
    height: '100%',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule8: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    wordBreak: 'break-all',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule9: {
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
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: themeColors['--color-border'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule10: { flex: 1, overflow: 'auto' },
  rule11: {
    height: '100%',
    width: '100%',
    resize: 'none',
    '--tw-border-style': 'none',
    borderStyle: 'none',
    backgroundColor: themeColors['--color-background-primary'],
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-editor-foreground'],
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-brand'],
    },
    '--tw-ring-inset': { default: null, ':focus-visible': 'inset' },
  },
  rule12: {
    display: 'flex',
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 0,
      },
    },
    minWidth: 0,
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: themeColors['--color-border'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule13: {
    minWidth: 0,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule14: {
    display: 'flex',
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 0,
      },
    },
    flexShrink: 0,
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    borderRadius: '.25rem',
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-list-hover-background'],
      },
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-brand'],
    },
    paddingBlock: {
      default: null,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': `calc(${themeLayout['--spacing']} * .5)`,
      },
    },
  },
  rule15: { color: themeColors['--color-brand'] },
  rule16: { color: themeColors['--color-foreground-alt'] },
  rule17: {
    display: 'flex',
    flex: 1,
    flexDirection: 'column',
    overflow: 'hidden',
  },
  rule18: { display: 'flex', height: '100%', flexDirection: 'column' },
  rule19: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: themeColors['--color-border'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule20: {
    display: 'flex',
    height: '100%',
    alignItems: 'center',
    justifyContent: 'center',
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule21: {
    display: 'flex',
    height: '100%',
    alignItems: 'center',
    justifyContent: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-muted-foreground'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

interface NoteContentViewProps {
  worldState: Resource<IWorldState>
  sourceRef: string
  noteName: string
  editing: boolean
  onToggleEdit: () => void
  onFilterTag?: (tag: string | undefined) => void
  onFilterStatus?: (status: string | undefined) => void
  onContentSaved?: () => void
}

interface SaveStatusProps {
  state: 'idle' | 'saving' | 'saved' | 'failed'
  error: Error | null
  onRetry: () => void
}

// SaveStatus presents note write progress and the explicit failure action.
function SaveStatus({ state, error, onRetry }: SaveStatusProps) {
  if (state === 'idle') return null
  if (state === 'failed') {
    return (
      <div sx={stylexStyles2.rule1} role="alert">
        <span sx={stylexStyles2.rule2}>
          Failed to save note: {error?.message}
        </span>
        <button type="button" sx={stylexStyles2.rule3} onClick={onRetry}>
          Retry
        </button>
      </div>
    )
  }
  return (
    <div
      sx={[
        stylexStyles2.rule4,
        state === 'saved' ? stylexStyles2.rule5 : stylexStyles2.rule6,
      ]}
      role="status"
      aria-live="polite"
    >
      {state === 'saving' ? 'Saving…' : 'Saved'}
    </div>
  )
}

interface NoteReadErrorProps {
  error: Error
  onRetry: () => void
}

// NoteReadError presents a failed note read and its Resource retry action.
function NoteReadError({ error, onRetry }: NoteReadErrorProps) {
  return (
    <div sx={stylexStyles2.rule7} role="alert">
      <span>Failed to load note</span>
      <span sx={stylexStyles2.rule8}>{error.message}</span>
      <button type="button" sx={stylexStyles2.rule9} onClick={onRetry}>
        Retry
      </button>
    </div>
  )
}

interface SourceEditorProps {
  value: string
  onChange: (content: string) => void
  onBlur: () => void
}

// SourceEditor presents the raw note body with an explicit accessible name.
function SourceEditor({ value, onChange, onBlur }: SourceEditorProps) {
  return (
    <div sx={stylexStyles2.rule10}>
      <textarea
        aria-label="Note source"
        sx={stylexStyles2.rule11}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        onBlur={onBlur}
      />
    </div>
  )
}

interface NoteHeaderProps {
  noteName: string
  editing: boolean
  disabled: boolean
  onToggle: () => void
  onTogglePointerDown: () => void
}

// NoteHeader presents the note title and editor-mode action.
function NoteHeader({
  noteName,
  editing,
  disabled,
  onToggle,
  onTogglePointerDown,
}: NoteHeaderProps) {
  return (
    <div sx={stylexStyles2.rule12}>
      <span sx={stylexStyles2.rule13}>
        {stripNoteFileExtension(noteName.split('/').pop() ?? noteName)}
      </span>
      <button
        type="button"
        sx={[
          stylexStyles2.rule14,
          editing ? stylexStyles2.rule15 : stylexStyles2.rule16,
        ]}
        onClick={onToggle}
        onPointerDown={onTogglePointerDown}
        disabled={disabled}
        data-testid="notes-source-toggle"
        title={editing ? 'Switch to WYSIWYG' : 'Switch to source'}
      >
        {editing ? (
          <>
            <LuPenLine sx={stylexStyles.rule1} />
            WYSIWYG
          </>
        ) : (
          <>
            <LuCode sx={stylexStyles.rule1} />
            Source
          </>
        )}
      </button>
    </div>
  )
}

interface NoteBodyProps {
  editing: boolean
  sourceContent: string
  onSourceChange: (content: string) => void
  onSourceBlur: () => void
  noteFormat: ReturnType<typeof getNoteFileFormat>
  parsedNote: ReturnType<typeof parseNote> | null
  editorContent: string
  composerKey: string
  onSave: (content: string) => Promise<void>
  onDraftChange: (content: string) => void
  onFilterTag?: (tag: string | undefined) => void
  onFilterStatus?: (status: string | undefined) => void
}

// NoteBody presents source or WYSIWYG editing for the selected note.
function NoteBody({
  editing,
  sourceContent,
  onSourceChange,
  onSourceBlur,
  noteFormat,
  parsedNote,
  editorContent,
  composerKey,
  onSave,
  onDraftChange,
  onFilterTag,
  onFilterStatus,
}: NoteBodyProps) {
  if (editing) {
    return (
      <SourceEditor
        value={sourceContent}
        onChange={onSourceChange}
        onBlur={onSourceBlur}
      />
    )
  }
  return (
    <>
      {noteFormat === 'markdown' && parsedNote && (
        <FrontmatterDisplay
          frontmatter={parsedNote.frontmatter}
          onTagClick={onFilterTag}
          onStatusClick={onFilterStatus}
        />
      )}
      <div sx={stylexStyles2.rule17}>
        <LexicalEditor
          content={editorContent}
          format={noteFormat ?? 'markdown'}
          onSave={onSave}
          onDraftChange={onDraftChange}
          composerKey={composerKey}
        />
      </div>
    </>
  )
}

interface NoteFileViewProps extends Omit<NoteContentViewProps, 'sourceRef'> {
  objectKey: string
  filePath: string
}

interface NoteErrorBoundaryProps {
  noteName: string
  children: ReactNode
}

/** NoteErrorBoundary contains note parsing and editor failures within one file view. */
class NoteErrorBoundary extends Component<
  NoteErrorBoundaryProps,
  { failed: boolean }
> {
  state = { failed: false }

  /** getDerivedStateFromError replaces only the selected note's content. */
  static getDerivedStateFromError() {
    return { failed: true }
  }

  /** componentDidCatch keeps parser details in the developer console. */
  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('Failed to open note', this.props.noteName, error, info)
  }

  /** render leaves notebook navigation available after a note fails. */
  render() {
    if (!this.state.failed) {
      return this.props.children
    }
    return (
      <div sx={stylexStyles2.rule18} data-testid="notes-content-view">
        <div sx={stylexStyles2.rule19}>
          {stripNoteFileExtension(
            this.props.noteName.split('/').pop() ?? this.props.noteName,
          )}
        </div>
        <div sx={stylexStyles2.rule20} role="alert">
          Could not open this note. Select another note.
        </div>
      </div>
    )
  }
}

/**
 * NoteFileView displays one note file. Its write state belongs to that file,
 * so NoteContentView keys its boundary by object key and file path.
 */
function NoteFileView({
  worldState,
  objectKey,
  filePath,
  noteName,
  editing,
  onToggleEdit,
  onFilterTag,
  onFilterStatus,
  onContentSaved,
}: NoteFileViewProps) {
  // Resolve the format for the selected file.
  const noteFormat = getNoteFileFormat(noteName) ?? 'markdown'

  // Read the selected file and attach its write state.
  const rootHandle = useUnixFSRootHandle(worldState, objectKey)
  const fileHandle = useUnixFSHandle(rootHandle, filePath)
  const textResource = useUnixFSHandleTextContent(fileHandle)
  const noteWrite = useNoteWrite({
    fileHandle,
    loadedContent: textResource.value ?? '',
    editing,
    noteFormat,
    onToggleEdit,
    onContentSaved,
  })

  // Keep an empty selection separate from file loading.
  if (!noteName) {
    return <div sx={stylexStyles2.rule21}>Select a note to view</div>
  }

  // Show the selected file's pending read.
  if (textResource.loading) {
    return <div sx={stylexStyles2.rule21}>Loading…</div>
  }

  // Keep a failed read retryable through its resource.
  if (textResource.error) {
    return (
      <NoteReadError error={textResource.error} onRetry={textResource.retry} />
    )
  }

  return (
    <div sx={stylexStyles2.rule18} data-testid="notes-content-view">
      <NoteHeader
        noteName={noteName}
        editing={editing}
        disabled={noteWrite.sourceSaving || !fileHandle.value}
        onToggle={noteWrite.handleToggle}
        onTogglePointerDown={noteWrite.handleTogglePointerDown}
      />
      <SaveStatus
        state={noteWrite.saveState}
        error={noteWrite.writeError}
        onRetry={noteWrite.handleRetrySave}
      />
      <NoteBody
        editing={editing}
        sourceContent={noteWrite.sourceContent ?? noteWrite.content}
        onSourceChange={noteWrite.handleSourceChange}
        onSourceBlur={noteWrite.handleSourceBlur}
        noteFormat={noteFormat}
        parsedNote={noteWrite.parsedNote}
        editorContent={noteWrite.editorContent}
        composerKey={`${filePath}:${noteFormat}`}
        onSave={noteWrite.handleWysiwygSave}
        onDraftChange={noteWrite.handleWysiwygDraftChange}
        onFilterTag={onFilterTag}
        onFilterStatus={onFilterStatus}
      />
    </div>
  )
}

/** NoteContentView displays a note with WYSIWYG (Lexical) or source (textarea) mode. */
function NoteContentView({ sourceRef, ...props }: NoteContentViewProps) {
  const parsed = useMemo(() => parseObjectUri(sourceRef), [sourceRef])
  const filePath = parsed.path
    ? `${parsed.path}/${props.noteName}`
    : props.noteName

  return (
    <NoteErrorBoundary
      key={`${parsed.objectKey}:${filePath}`}
      noteName={props.noteName}
    >
      <NoteFileView
        objectKey={parsed.objectKey}
        filePath={filePath}
        {...props}
      />
    </NoteErrorBoundary>
  )
}

export default NoteContentView
