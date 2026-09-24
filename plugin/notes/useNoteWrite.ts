import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useLatestRef } from '@aptre/bldr-react'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { FSHandle } from '@s4wave/sdk/unixfs/handle.js'

import { parseNote, reassembleNote } from './frontmatter.js'
import type { NoteFileFormat } from './note-files.js'
import { reassembleOrgMetadata, splitOrgMetadata } from './org/org.js'

interface UseNoteWriteOptions {
  fileHandle: Resource<FSHandle>
  filePath: string
  loadedContent: string
  editing: boolean
  noteFormat: NoteFileFormat
  onToggleEdit: () => void
  onContentSaved?: () => void
}

// useNoteWrite owns queued note writes, retained drafts, and mode transitions.
// Revisions and file paths guard asynchronous completions from stale writes.
export function useNoteWrite({
  fileHandle,
  filePath,
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
  const failedWrite = useRef<{ filePath: string; content: string } | null>(null)
  const currentFilePath = useLatestRef(filePath)
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
    savedContent?.filePath === filePath ? savedContent.content : loadedContent
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

  const writeFile = useCallback(
    (nextContent: string) => {
      const handle = fileHandle.value
      if (!handle) {
        const error = new Error('note file handle is not ready')
        failedWrite.current = { filePath, content: nextContent }
        saveTargetPath.current = filePath
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
        saveTargetPath.current = filePath
        setSaveState('saving')
      }
      const encoded = new TextEncoder().encode(nextContent)
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
          setSavedContent({ filePath, content: nextContent })
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
          failedWrite.current = { filePath, content: nextContent }
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
    [
      fileHandle.value,
      filePath,
      onContentSaved,
      currentFilePath,
      lastSettledContent,
    ],
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
    [filePath, noteFormat, rawMetadata, lastSettledContent],
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
  }, [editing, filePath, writeFile, currentFilePath])

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

  const handleSourceChange = useCallback(
    (nextContent: string) => {
      saveRevision.current += 1
      setSaveState((state) => (state === 'failed' ? state : 'idle'))
      sourceContentRef.current = nextContent
      setSourceContent(nextContent)
      if (failedWrite.current?.filePath === filePath) {
        failedWrite.current = { filePath, content: nextContent }
      }
    },
    [filePath],
  )

  return {
    sourceContent,
    sourceSaving,
    displayedSaveState,
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
