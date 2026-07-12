import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { FSHandle, TreeUploadEntry } from '@s4wave/sdk/unixfs/handle.js'

import { TreeUploadPool } from '@s4wave/sdk/unixfs/upload-pool.js'

// UploadStatus, UploadItem, UploadEvent, and UploadManager form the durable
// contract this hook owns. The manager holds no viewer or route identity: each
// upload group captures its own target handle at addFiles time, so the manager
// can outlive the folder view that started it (see SessionUploadManagerContext).

// UploadStatus represents the state of a single upload item.
export type UploadStatus = 'queued' | 'uploading' | 'done' | 'error'

// UploadItem tracks the state of a single file upload.
export interface UploadItem {
  id: string
  groupId: string
  kind: 'file' | 'directory'
  file: File | null
  name: string
  path: string
  totalSize: number
  bytesWritten: number
  status: UploadStatus
  error?: string
  abortController: AbortController
}

// UploadEvent is a transient upload-lifecycle notification for the UI: a burst
// starting (from addFiles) or every item in progress reaching a terminal state.
export interface UploadEvent {
  // id increments per event so a consumer reacts to each event even when the
  // kind repeats across successive bursts.
  id: number
  kind: 'started' | 'completed'
  // fileCount is the number of files in the started burst, or the number of
  // items that completed successfully at completion.
  fileCount: number
  // errorCount is the number of failed items at completion; always 0 for
  // 'started'.
  errorCount: number
}

// UploadManager provides the interface for managing file uploads.
export interface UploadManager {
  items: UploadItem[]
  activeCount: number
  // lastEvent is the most recent upload-lifecycle event, or null before any
  // upload has started this session. Consumers dedupe on lastEvent.id.
  lastEvent: UploadEvent | null
  // addFiles enqueues a batch against handle. The handle is captured into the
  // batch here, not held by the manager, so navigating away from the folder
  // that started the upload neither aborts the write nor drops the feedback.
  addFiles: (handle: FSHandle, files: File[], directories?: string[]) => void
  cancelUpload: (id: string) => void
  cancelAll: () => void
  clearDone: () => void
}

// useUploadManager projects the session-owned SDK upload pool into React state.
// Each addFiles call captures its target handle, so uploads can outlive the
// folder view that started them.
export function useUploadManager(): UploadManager {
  const [items, setItems] = useState<UploadItem[]>([])
  const [lastEvent, setLastEvent] = useState<UploadEvent | null>(null)

  const nextIdRef = useRef(0)
  const nextGroupIdRef = useRef(0)
  const eventSeqRef = useRef(0)
  const wasInProgressRef = useRef(false)
  const poolRef = useRef<TreeUploadPool | null>(null)
  if (!poolRef.current) poolRef.current = new TreeUploadPool()
  const pool = poolRef.current

  const activeCount = useMemo(
    () => items.filter((item) => item.status === 'uploading').length,
    [items],
  )

  // Emit a completion event on the transition from any in-progress items to
  // every item reaching a terminal state. Runs before the auto-clear below, so
  // items are still present and consumers can anchor feedback to the indicator.
  useEffect(() => {
    const inProgress = items.some(
      (item) => item.status === 'queued' || item.status === 'uploading',
    )
    if (inProgress) {
      wasInProgressRef.current = true
      return
    }
    if (!wasInProgressRef.current || items.length === 0) {
      wasInProgressRef.current = false
      return
    }
    wasInProgressRef.current = false
    const fileCount = items.filter((item) => item.status === 'done').length
    const errorCount = items.filter((item) => item.status === 'error').length
    setLastEvent({
      id: ++eventSeqRef.current,
      kind: 'completed',
      fileCount,
      errorCount,
    })
  }, [items])

  // Auto-clear completed uploads after a delay.
  useEffect(() => {
    if (items.length === 0) return
    const allFinished = items.every(
      (item) => item.status === 'done' || item.status === 'error',
    )
    if (!allFinished) return

    const timer = setTimeout(() => {
      setItems((prev) => prev.filter((item) => item.status !== 'done'))
    }, 3000)
    return () => clearTimeout(timer)
  }, [items])

  const addFiles = useCallback(
    (handle: FSHandle, files: File[], directories?: string[]) => {
      const groupId = `upload-group-${++nextGroupIdRef.current}`
      const newItems: UploadItem[] = files.map((file) => ({
        id: `upload-${++nextIdRef.current}`,
        groupId,
        kind: 'file',
        file,
        name: file.name,
        path:
          (file as File & { webkitRelativePath?: string }).webkitRelativePath ||
          file.name,
        totalSize: file.size,
        bytesWritten: 0,
        status: 'queued',
        abortController: new AbortController(),
      }))
      for (const directory of directories ?? []) {
        newItems.push({
          id: `upload-${++nextIdRef.current}`,
          groupId,
          kind: 'directory',
          file: null,
          name: directory.split('/').at(-1) ?? directory,
          path: directory,
          totalSize: 0,
          bytesWritten: 0,
          status: 'queued',
          abortController: new AbortController(),
        })
      }
      if (newItems.length === 0) return

      setItems((prev) => [...prev, ...newItems])
      const fileCount = files.length || (directories?.length ?? 0)
      setLastEvent({
        id: ++eventSeqRef.current,
        kind: 'started',
        fileCount,
        errorCount: 0,
      })

      for (const item of newItems) {
        const entry: TreeUploadEntry =
          item.kind === 'file' && item.file
            ? {
                kind: 'file',
                path: item.path,
                totalSize: BigInt(item.totalSize),
                stream: item.file.stream(),
                onProgress: (bytesWritten) => {
                  setItems((cur) =>
                    cur.map((current) =>
                      current.id === item.id
                        ? { ...current, bytesWritten: Number(bytesWritten) }
                        : current,
                    ),
                  )
                },
              }
            : { kind: 'directory', path: item.path }

        pool.add(
          handle,
          entry,
          {
            onStart: () => {
              setItems((cur) =>
                cur.map((current) =>
                  current.id === item.id
                    ? { ...current, status: 'uploading' }
                    : current,
                ),
              )
            },
            onComplete: () => {
              setItems((cur) =>
                cur.map((current) =>
                  current.id === item.id
                    ? {
                        ...current,
                        status: 'done',
                        bytesWritten: current.totalSize,
                      }
                    : current,
                ),
              )
            },
            onError: (err) => {
              const error = err instanceof Error ? err.message : 'Upload failed'
              setItems((cur) =>
                cur.map((current) =>
                  current.id === item.id
                    ? { ...current, status: 'error', error }
                    : current,
                ),
              )
            },
          },
          item.abortController.signal,
        )
      }
    },
    [pool],
  )

  const cancelUpload = useCallback((id: string) => {
    setItems((prev) => {
      const item = prev.find((current) => current.id === id)
      if (!item) return prev
      if (item.status === 'queued' || item.status === 'uploading') {
        item.abortController.abort()
      }
      return prev.filter((current) => current.id !== id)
    })
  }, [])

  const cancelAll = useCallback(() => {
    setItems((prev) => {
      for (const item of prev) item.abortController.abort()
      return []
    })
  }, [])

  const clearDone = useCallback(() => {
    setItems((prev) => prev.filter((item) => item.status !== 'done'))
  }, [])

  return useMemo(
    () => ({
      items,
      activeCount,
      lastEvent,
      addFiles,
      cancelUpload,
      cancelAll,
      clearDone,
    }),
    [
      items,
      activeCount,
      lastEvent,
      addFiles,
      cancelUpload,
      cancelAll,
      clearDone,
    ],
  )
}
