import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

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

import { parseNote, reassembleNote } from './frontmatter.js'
import FrontmatterDisplay from './FrontmatterDisplay.js'
import LexicalEditor from './LexicalEditor.js'
import { getNoteFileFormat, stripNoteFileExtension } from './note-files.js'
import type { NoteFileFormat } from './note-files.js'
import { reassembleOrgMetadata, splitOrgMetadata } from './org/org.js'

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

function useNoteMetadata(content: string, noteFormat: NoteFileFormat) {
  const parsedNote = useMemo(() => {
    if (!content || noteFormat !== 'markdown') return null
    return parseNote(content)
  }, [content, noteFormat])
  const orgNote = useMemo(() => {
    if (noteFormat !== 'org') return null
    return splitOrgMetadata(content)
  }, [content, noteFormat])
  const rawMetadata =
    noteFormat === 'org'
      ? (orgNote?.metadata ?? '')
      : (parsedNote?.rawFrontmatter ?? '')
  const editorContent =
    noteFormat === 'org' ? (orgNote?.body ?? '') : (parsedNote?.body ?? '')
  return { editorContent, parsedNote, rawMetadata }
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

  // Source mode edit state.
  const [sourceContent, setSourceContent] = useState<string | null>(null)
  const sourceContentRef = useRef<string | null>(null)
  const [sourceSaving, setSourceSaving] = useState(false)
  const [saveState, setSaveState] = useState<
    'idle' | 'saving' | 'saved' | 'failed'
  >('idle')
  const [writeError, setWriteError] = useState<Error | null>(null)
  const failedWrite = useRef<{ filePath: string; content: string } | null>(null)
  const currentFilePath = useRef(filePath)
  currentFilePath.current = filePath
  const saveRevision = useRef(0)
  const saveTargetPath = useRef(filePath)
  const writeTails = useRef(new Map<string, Promise<void>>())
  const mounted = useRef(true)
  const [savedContent, setSavedContent] = useState<{
    filePath: string
    content: string
  } | null>(null)
  const skipNextSourceBlurSave = useRef(false)
  const content =
    savedContent?.filePath === filePath
      ? savedContent.content
      : (textResource.value ?? '')
  // Full note text of the last completed write or initial load. Editor
  // updates that re-export this text are not edits.
  const lastSettledContent = useRef('')
  lastSettledContent.current = content
  const displayedSaveState =
    saveTargetPath.current === filePath ? saveState : 'idle'

  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
    }
  }, [])

  useEffect(() => {
    saveTargetPath.current = filePath
    failedWrite.current = null
    setWriteError(null)
    setSaveState('idle')
    sourceContentRef.current = null
    setSourceContent(null)
  }, [filePath])

  const { editorContent, parsedNote, rawMetadata } = useNoteMetadata(
    content,
    noteFormat,
  )

  const writeFile = useCallback(
    (content: string) => {
      const handle = fileHandle.value
      if (!handle) {
        const error = new Error('note file handle is not ready')
        failedWrite.current = { filePath, content }
        saveTargetPath.current = filePath
        setWriteError(error)
        setSaveState('failed')
        return Promise.reject(error)
      }

      const isReexport = content === lastSettledContent.current
      const revision = isReexport
        ? saveRevision.current
        : saveRevision.current + 1
      if (!isReexport) {
        saveRevision.current = revision
        saveTargetPath.current = filePath
        setSaveState('saving')
      }
      const encoded = new TextEncoder().encode(content)
      const prior = writeTails.current.get(filePath) ?? Promise.resolve()
      const operation = prior
        .catch(() => {})
        .then(async () => {
          await handle.writeAt(0n, encoded)
          await handle.truncate(BigInt(encoded.byteLength))
        })
      writeTails.current.set(filePath, operation)

      void operation.then(
        () => {
          if (
            !mounted.current ||
            revision !== saveRevision.current ||
            currentFilePath.current !== filePath
          ) {
            return
          }
          setSavedContent({ filePath, content })
          setWriteError(null)
          failedWrite.current = null
          setSaveState('saved')
          onContentSaved?.()
        },
        (error: unknown) => {
          if (
            !mounted.current ||
            revision !== saveRevision.current ||
            currentFilePath.current !== filePath
          ) {
            return
          }
          const nextError =
            error instanceof Error ? error : new Error(String(error))
          setWriteError(nextError)
          failedWrite.current = { filePath, content }
          setSaveState('failed')
        },
      )
      void operation
        .finally(() => {
          if (writeTails.current.get(filePath) === operation) {
            writeTails.current.delete(filePath)
          }
        })
        .catch(() => {})
      return operation
    },
    [fileHandle.value, filePath, onContentSaved],
  )

  const handleWysiwygDraftChange = useCallback(
    (body: string) => {
      const full =
        noteFormat === 'org'
          ? reassembleOrgMetadata(rawMetadata, body)
          : reassembleNote(rawMetadata, body)
      if (full !== lastSettledContent.current) {
        // Real edit: supersede in-flight completions and drop stale status.
        saveRevision.current += 1
        setSaveState((state) => (state === 'failed' ? state : 'idle'))
      }
      if (failedWrite.current?.filePath !== filePath) return
      failedWrite.current = { filePath, content: full }
    },
    [filePath, noteFormat, rawMetadata],
  )

  // WYSIWYG save: re-assemble format metadata + exported body, then write.
  const handleWysiwygSave = useCallback(
    async (body: string) => {
      const full =
        noteFormat === 'org'
          ? reassembleOrgMetadata(rawMetadata, body)
          : reassembleNote(rawMetadata, body)
      await writeFile(full)
    },
    [noteFormat, rawMetadata, writeFile],
  )

  // Source mode blur: write the raw content.
  const handleRetrySave = useCallback(() => {
    const failed = failedWrite.current
    if (failed === null || failed.filePath !== filePath) return
    void (async () => {
      try {
        await writeFile(failed.content)
        if (
          editing &&
          currentFilePath.current === filePath &&
          sourceContentRef.current === failed.content
        ) {
          sourceContentRef.current = null
          setSourceContent(null)
        }
      } catch {
        // writeFile keeps the failed draft and error available for another retry.
      }
    })()
  }, [editing, filePath, writeFile])

  const handleSourceBlur = useCallback(() => {
    if (skipNextSourceBlurSave.current) return
    if (sourceContent !== null) {
      void writeFile(sourceContent).catch(() => {
        // writeFile already surfaced the error in component state.
      })
    }
  }, [sourceContent, writeFile])

  const handleToggle = useCallback(() => {
    if (editing) {
      // Switching from source to WYSIWYG.
      if (sourceContent !== null) {
        void (async () => {
          setSourceSaving(true)
          const savingContent = sourceContent
          try {
            await writeFile(savingContent)
            if (sourceContentRef.current === savingContent) {
              sourceContentRef.current = null
              setSourceContent(null)
              onToggleEdit()
            }
          } catch {
            // writeFile already surfaced the error in component state.
          } finally {
            skipNextSourceBlurSave.current = false
            setSourceSaving(false)
          }
        })()
        return
      }
      skipNextSourceBlurSave.current = false
    } else {
      // Switching from WYSIWYG to source.
      skipNextSourceBlurSave.current = false
      sourceContentRef.current = content
      setSourceContent(content)
    }
    onToggleEdit()
  }, [editing, sourceContent, content, onToggleEdit, writeFile])

  const handleTogglePointerDown = useCallback(() => {
    if (editing) {
      skipNextSourceBlurSave.current = true
    }
  }, [editing])

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
        disabled={sourceSaving || !fileHandle.value}
        onToggle={handleToggle}
        onTogglePointerDown={handleTogglePointerDown}
      />
      <SaveStatus
        state={displayedSaveState}
        error={writeError}
        onRetry={handleRetrySave}
      />
      <NoteBody
        editing={editing}
        sourceContent={sourceContent ?? content}
        onSourceChange={(nextContent) => {
          saveRevision.current += 1
          setSaveState((state) => (state === 'failed' ? state : 'idle'))
          sourceContentRef.current = nextContent
          setSourceContent(nextContent)
          if (failedWrite.current?.filePath === filePath) {
            failedWrite.current = { filePath, content: nextContent }
          }
        }}
        onSourceBlur={handleSourceBlur}
        noteFormat={noteFormat}
        parsedNote={parsedNote}
        editorContent={editorContent}
        composerKey={`${filePath}:${noteFormat}`}
        onSave={handleWysiwygSave}
        onDraftChange={handleWysiwygDraftChange}
        onFilterTag={onFilterTag}
        onFilterStatus={onFilterStatus}
      />
    </div>
  )
}

export default NoteContentView
