import { useMemo } from 'react'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { parseObjectUri } from '@s4wave/sdk/space/object-uri.js'
import {
  useUnixFSRootHandle,
  useUnixFSHandle,
  useUnixFSHandleTextContent,
} from '@s4wave/web/hooks/useUnixFSHandle.js'
import { cn } from '@s4wave/web/style/utils.js'
import { LuCode, LuPenLine } from 'react-icons/lu'

import FrontmatterDisplay from './FrontmatterDisplay.js'
import LexicalEditor from './LexicalEditor.js'
import { getNoteFileFormat, stripNoteFileExtension } from './note-files.js'
import { useNoteWrite } from './useNoteWrite.js'
import type { parseNote } from './frontmatter.js'

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
      <div
        className="border-destructive/30 text-destructive flex min-w-0 flex-wrap items-center gap-2 border-b px-3 py-1 text-xs"
        role="alert"
      >
        <span className="min-w-0 flex-1 break-words">
          Failed to save note: {error?.message}
        </span>
        <button
          type="button"
          className="border-destructive/40 shrink-0 rounded border px-2 py-0.5 font-medium"
          onClick={onRetry}
        >
          Retry
        </button>
      </div>
    )
  }
  return (
    <div
      className={cn(
        'border-border border-b px-3 py-1 text-xs',
        state === 'saved' ? 'text-success' : 'text-muted-foreground',
      )}
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
    <div
      className="text-destructive flex h-full flex-col items-center justify-center gap-2 p-4 text-xs"
      role="alert"
    >
      <span>Failed to load note</span>
      <span className="text-foreground-alt/50 text-xs break-all">
        {error.message}
      </span>
      <button
        type="button"
        className="border-border text-foreground rounded border px-2 py-1 font-medium"
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
    <div className="flex-1 overflow-auto">
      <textarea
        aria-label="Note source"
        className="bg-background-primary text-editor-foreground focus-visible:ring-brand h-full w-full resize-none border-none p-4 font-mono text-xs outline-none focus-visible:ring-2 focus-visible:ring-inset"
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
    <div className="border-border flex items-center justify-between border-b px-3 py-1.5">
      <span className="text-xs font-medium">
        {stripNoteFileExtension(noteName.split('/').pop() ?? noteName)}
      </span>
      <button
        type="button"
        className={cn(
          'flex items-center gap-1 rounded px-2 py-0.5 text-xs',
          'hover:bg-list-hover-background focus-visible:ring-brand focus-visible:ring-2',
          editing ? 'text-brand' : 'text-foreground-alt',
        )}
        onClick={onToggle}
        onPointerDown={onTogglePointerDown}
        disabled={disabled}
        data-testid="notes-source-toggle"
        title={editing ? 'Switch to WYSIWYG' : 'Switch to source'}
      >
        {editing ? (
          <>
            <LuPenLine className="size-3" />
            WYSIWYG
          </>
        ) : (
          <>
            <LuCode className="size-3" />
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
      <div className="flex flex-1 flex-col overflow-hidden">
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

// NoteContentView displays a note with WYSIWYG (Lexical) or source (textarea) mode.
function NoteContentView({
  worldState,
  sourceRef,
  noteName,
  editing,
  onToggleEdit,
  onFilterTag,
  onFilterStatus,
  onContentSaved,
}: NoteContentViewProps) {
  const parsed = useMemo(() => parseObjectUri(sourceRef), [sourceRef])
  const filePath = useMemo(() => {
    const base = parsed.path
    return base ? `${base}/${noteName}` : noteName
  }, [parsed.path, noteName])
  const noteFormat = getNoteFileFormat(noteName) ?? 'markdown'

  const rootHandle = useUnixFSRootHandle(worldState, parsed.objectKey)
  const fileHandle = useUnixFSHandle(rootHandle, filePath)
  const textResource = useUnixFSHandleTextContent(fileHandle)
  const noteWrite = useNoteWrite({
    fileHandle,
    filePath,
    loadedContent: textResource.value ?? '',
    editing,
    noteFormat,
    onToggleEdit,
    onContentSaved,
  })

  if (!noteName) {
    return (
      <div className="text-muted-foreground flex h-full items-center justify-center text-xs">
        Select a note to view
      </div>
    )
  }

  if (textResource.loading) {
    return (
      <div className="text-muted-foreground flex h-full items-center justify-center text-xs">
        Loading…
      </div>
    )
  }

  if (textResource.error) {
    return (
      <NoteReadError error={textResource.error} onRetry={textResource.retry} />
    )
  }

  return (
    <div className="flex h-full flex-col" data-testid="notes-content-view">
      <NoteHeader
        noteName={noteName}
        editing={editing}
        disabled={noteWrite.sourceSaving || !fileHandle.value}
        onToggle={noteWrite.handleToggle}
        onTogglePointerDown={noteWrite.handleTogglePointerDown}
      />
      <SaveStatus
        state={noteWrite.displayedSaveState}
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

export default NoteContentView
