import { controlTargetStyles } from '@s4wave/web/style/ControlStylesProvider.js'
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
import { layout as themeLayout } from '@s4wave/web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
} from '@s4wave/web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    color: themeColors['--color-destructive'],
    display: 'flex',
    flexWrap: 'wrap',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    minWidth: 0,
  },
  rule2: { flex: 1, overflowWrap: 'break-word', minWidth: 0 },
  rule3: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 40%, transparent)`,
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    flexShrink: 0,
    fontWeight: themeFonts['--font-weight-medium'],
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
  },
  rule4: {
    borderColor: themeColors['--color-border'],
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule5: { color: themeColors['--color-success'] },
  rule6: { color: themeColors['--color-muted-foreground'] },
  rule7: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: themeColors['--color-destructive'],
    display: 'flex',
    flexDirection: 'column',
    fontSize: themeFonts['--text-xs'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    height: '100%',
  },
  rule8: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    wordBreak: 'break-all',
  },
  rule9: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderColor: themeColors['--color-border'],
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    color: themeColors['--color-foreground'],
    fontWeight: themeFonts['--font-weight-medium'],
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
  },
  rule10: { flex: 1, overflow: 'auto' },
  rule11: {
    '--tw-border-style': 'none',
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
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    borderStyle: 'none',
    backgroundColor: themeColors['--color-background-primary'],
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    color: themeColors['--color-editor-foreground'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    outlineStyle: 'none',
    resize: 'none',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    height: '100%',
    width: '100%',
  },
  rule12: {
    borderColor: themeColors['--color-border'],
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 0,
      },
    },
    minWidth: 0,
  },
  rule13: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    minWidth: 0,
  },
  rule14: {
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-brand'],
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    borderRadius: '.25rem',
    gap: themeLayout['--spacing'],
    paddingBlock: {
      default: null,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': `calc(${themeLayout['--spacing']} * .5)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-list-hover-background'],
      },
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    display: 'flex',
    flexShrink: 0,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 0,
      },
    },
  },
  rule15: { color: themeColors['--color-brand'] },
  rule16: { color: themeColors['--color-foreground-alt'] },
  rule17: {
    flex: 1,
    overflow: 'hidden',
    display: 'flex',
    flexDirection: 'column',
  },
  rule18: { display: 'flex', flexDirection: 'column', height: '100%' },
  rule19: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderColor: themeColors['--color-border'],
    overflow: 'hidden',
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule20: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    color: themeColors['--color-destructive'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    height: '100%',
  },
  rule21: {
    alignItems: 'center',
    color: themeColors['--color-muted-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    height: '100%',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
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
        <button
          type="button"
          sx={[stylexStyles2.rule3, controlTargetStyles.minimum]}
          onClick={onRetry}
        >
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
      <button
        type="button"
        sx={[stylexStyles2.rule9, controlTargetStyles.minimum]}
        onClick={onRetry}
      >
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
          controlTargetStyles.minimum,
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
