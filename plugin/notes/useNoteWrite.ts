import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useLatestRef } from '@aptre/bldr-react'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { FSHandle } from '@s4wave/sdk/unixfs/handle.js'

import { parseNote, reassembleNote } from './frontmatter.js'
import type { NoteFileFormat } from './note-files.js'
import { reassembleOrgMetadata, splitOrgMetadata } from './org/org.js'

interface UseNoteWriteOptions {
  fileHandle: Resource<FSHandle>
  loadedContent: string
  editing: boolean
  noteFormat: NoteFileFormat
  onToggleEdit: () => void
  onContentSaved?: () => void
}

// useNoteWrite owns queued note writes, retained drafts, and mode transitions
// for one note file. Callers mount it per file, keyed by path, so a different
// file starts from fresh state. Revisions guard asynchronous completions from
// stale writes.
export function useNoteWrite({
  fileHandle,
  loadedContent,
  editing,
  noteFormat,
  onToggleEdit,
  onContentSaved,
}: UseNoteWriteOptions) {
  const [sourceContent, setSourceContent] = useState<string | null>(null)
  const sourceContentRef = useRef<string | null>(null)
  const [sourceSaving, setSourceSaving] = useState(false)
  const [saveState, setSaveState] = useState<
    'idle' | 'saving' | 'saved' | 'failed'
  >('idle')
  const [writeError, setWriteError] = useState<Error | null>(null)
  const failedWrite = useRef<string | null>(null)
  const saveRevision = useRef(0)
  const writeTail = useRef<Promise<void> | null>(null)
  const mounted = useRef(true)
  const [savedContent, setSavedContent] = useState<string | null>(null)
  const skipNextSourceBlurSave = useRef(false)
  const content = savedContent ?? loadedContent
  // Full note text of the last completed write or initial load. Editor
  // updates that re-export this text are not edits.
  const lastSettledContent = useLatestRef(content)
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

  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
    }
  }, [])

  const writeFile = useCallback(
    (nextContent: string) => {
      const handle = fileHandle.value
      if (!handle) {
        const error = new Error('note file handle is not ready')
        failedWrite.current = nextContent
        setWriteError(error)
        setSaveState('failed')
        return Promise.reject(error)
      }

      const isReexport = nextContent === lastSettledContent.current
      const revision = isReexport
        ? saveRevision.current
        : saveRevision.current + 1
      if (!isReexport) {
        saveRevision.current = revision
        setSaveState('saving')
      }
      const encoded = new TextEncoder().encode(nextContent)
      const operation = (writeTail.current ?? Promise.resolve())
        .catch(() => {})
        .then(async () => {
          await handle.writeAt(0n, encoded)
          await handle.truncate(BigInt(encoded.byteLength))
        })
      writeTail.current = operation

      void operation.then(
        () => {
          if (!mounted.current || revision !== saveRevision.current) return
          setSavedContent(nextContent)
          setWriteError(null)
          failedWrite.current = null
          setSaveState('saved')
          onContentSaved?.()
        },
        (error: unknown) => {
          if (!mounted.current || revision !== saveRevision.current) return
          const nextError =
            error instanceof Error ? error : new Error(String(error))
          setWriteError(nextError)
          failedWrite.current = nextContent
          setSaveState('failed')
        },
      )
      return operation
    },
    [fileHandle.value, onContentSaved, lastSettledContent],
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
      if (failedWrite.current === null) return
      failedWrite.current = full
    },
    [noteFormat, rawMetadata, lastSettledContent],
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
  const handleSourceBlur = useCallback(() => {
    if (skipNextSourceBlurSave.current) return
    if (sourceContent !== null) {
      void writeFile(sourceContent).catch(() => {
        // writeFile already surfaced the error in component state.
      })
    }
  }, [sourceContent, writeFile])

  const handleRetrySave = useCallback(() => {
    const failed = failedWrite.current
    if (failed === null) return
    void (async () => {
      try {
        await writeFile(failed)
        if (editing && sourceContentRef.current === failed) {
          sourceContentRef.current = null
          setSourceContent(null)
        }
      } catch {
        // writeFile keeps the failed draft and error available for another retry.
      }
    })()
  }, [editing, writeFile])

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
  }, [content, editing, onToggleEdit, sourceContent, writeFile])

  const handleTogglePointerDown = useCallback(() => {
    if (editing) {
      skipNextSourceBlurSave.current = true
    }
  }, [editing])

  const handleSourceChange = useCallback((nextContent: string) => {
    saveRevision.current += 1
    setSaveState((state) => (state === 'failed' ? state : 'idle'))
    sourceContentRef.current = nextContent
    setSourceContent(nextContent)
    if (failedWrite.current !== null) {
      failedWrite.current = nextContent
    }
  }, [])

  return {
    sourceContent,
    sourceSaving,
    saveState,
    writeError,
    handleRetrySave,
    handleSourceBlur,
    handleToggle,
    handleTogglePointerDown,
    handleSourceChange,
    handleWysiwygSave,
    handleWysiwygDraftChange,
    editorContent,
    parsedNote,
    content,
  }
}
