import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { FSHandle, TreeUploadEntry } from '@s4wave/sdk/unixfs/handle.js'

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
  addFiles: (files: File[], directories?: string[]) => void
  cancelUpload: (id: string) => void
  cancelAll: () => void
  clearDone: () => void
}

// useUploadManager manages concurrent file uploads to a UnixFS handle.
export function useUploadManager(
  handle: FSHandle | null,
  concurrency = 1,
): UploadManager {
  const [items, setItems] = useState<UploadItem[]>([])
  const [lastEvent, setLastEvent] = useState<UploadEvent | null>(null)
  const handleRef = useRef(handle)
  useEffect(() => {
    handleRef.current = handle
  }, [handle])

  const nextIdRef = useRef(0)
  const nextGroupIdRef = useRef(0)
  const eventSeqRef = useRef(0)
  const wasInProgressRef = useRef(false)
  const startedRef = useRef(new Set<string>())
  const groupDirsRef = useRef(new Map<string, string[]>())

  const activeCount = useMemo(
    () => items.filter((i) => i.status === 'uploading').length,
    [items],
  )

  // processQueue marks queued items as uploading (state-only, no side effects).
  const processQueue = useCallback(() => {
    setItems((prev) => {
      const active = new Set(
        prev.flatMap((i) => (i.status === 'uploading' ? [i.groupId] : [])),
      ).size
      if (active >= concurrency) return prev

      const slots = concurrency - active
      const nextGroupIds: string[] = []
      const nextGroupIdSet = new Set<string>()
      for (const item of prev) {
        if (item.status !== 'queued') continue
        if (nextGroupIdSet.has(item.groupId)) continue
        nextGroupIds.push(item.groupId)
        nextGroupIdSet.add(item.groupId)
        if (nextGroupIds.length >= slots) break
      }
      if (nextGroupIds.length === 0) return prev

      const next = prev.map((item) =>
        item.status === 'queued' && nextGroupIds.includes(item.groupId)
          ? { ...item, status: 'uploading' as const }
          : item,
      )

      return next
    })
  }, [concurrency])

  // Process queue when items change.
  useEffect(() => {
    const queued = items.some((i) => i.status === 'queued')
    const active = new Set(
      items.flatMap((i) => (i.status === 'uploading' ? [i.groupId] : [])),
    ).size
    if (queued && active < concurrency) {
      queueMicrotask(processQueue)
    }
  }, [items, concurrency, processQueue])

  // Start uploads for groups marked uploading but not yet started.
  useEffect(() => {
    const groups = new Map<string, UploadItem[]>()
    for (const item of items) {
      if (item.status !== 'uploading') continue
      const group = groups.get(item.groupId)
      if (group) {
        group.push(item)
        continue
      }
      groups.set(item.groupId, [item])
    }

    for (const [groupId, groupItems] of groups) {
      if (startedRef.current.has(groupId)) continue
      startedRef.current.add(groupId)

      const h = handleRef.current
      if (!h) {
        setItems((cur) =>
          cur.map((c) =>
            c.groupId === groupId
              ? { ...c, status: 'error' as const, error: 'No handle' }
              : c,
          ),
        )
        continue
      }

      const entries: TreeUploadEntry[] = []
      const dirs = groupDirsRef.current.get(groupId) ?? []
      entries.push(
        ...dirs.map((dirPath) => ({
          kind: 'directory' as const,
          path: dirPath,
        })),
      )
      entries.push(
        ...groupItems.flatMap((item) =>
          item.kind === 'file' && item.file !== null
            ? [
                {
                  kind: 'file' as const,
                  path: item.path,
                  totalSize: BigInt(item.totalSize),
                  stream: item.file.stream(),
                  onProgress: (bytesWritten: bigint) => {
                    setItems((cur) =>
                      cur.map((c) =>
                        c.id === item.id
                          ? { ...c, bytesWritten: Number(bytesWritten) }
                          : c,
                      ),
                    )
                  },
                },
              ]
            : [],
        ),
      )

      h.uploadTree(entries, undefined, groupItems[0].abortController.signal)
        .then(() => {
          groupDirsRef.current.delete(groupId)
          setItems((cur) =>
            cur.map((c) =>
              c.groupId === groupId
                ? { ...c, status: 'done' as const, bytesWritten: c.totalSize }
                : c,
            ),
          )
        })
        .catch((err: unknown) => {
          groupDirsRef.current.delete(groupId)
          if (groupItems[0].abortController.signal.aborted) return
          const msg = err instanceof Error ? err.message : 'Upload failed'
          setItems((cur) =>
            cur.map((c) =>
              c.groupId === groupId
                ? { ...c, status: 'error' as const, error: msg }
                : c,
            ),
          )
        })
    }
  }, [items])

  // Emit a completion event on the transition from any in-progress items to
  // every item reaching a terminal state. Runs before the auto-clear below, so
  // items are still present and consumers can anchor feedback to the indicator.
  useEffect(() => {
    const inProgress = items.some(
      (i) => i.status === 'queued' || i.status === 'uploading',
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
    const fileCount = items.filter((i) => i.status === 'done').length
    const errorCount = items.filter((i) => i.status === 'error').length
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
      (i) => i.status === 'done' || i.status === 'error',
    )
    if (!allFinished) return

    const timer = setTimeout(() => {
      setItems((prev) => {
        const kept = prev.filter((i) => i.status !== 'done')
        for (const item of prev) {
          if (item.status === 'done') {
            startedRef.current.delete(item.groupId)
          }
        }
        return kept
      })
    }, 3000)
    return () => clearTimeout(timer)
  }, [items])

  const addFiles = useCallback((files: File[], directories?: string[]) => {
    const groupId = `upload-group-${++nextGroupIdRef.current}`
    const abortController = new AbortController()
    groupDirsRef.current.set(groupId, directories ?? [])
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
      status: 'queued' as const,
      abortController,
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
        status: 'queued' as const,
        abortController,
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
  }, [])

  const cancelUpload = useCallback((id: string) => {
    setItems((prev) => {
      const item = prev.find((i) => i.id === id)
      if (!item) return prev
      if (item.status === 'queued' || item.status === 'uploading') {
        item.abortController.abort()
      }
      startedRef.current.delete(item.groupId)
      groupDirsRef.current.delete(item.groupId)
      return prev.filter((i) => i.groupId !== item.groupId)
    })
  }, [])

  const cancelAll = useCallback(() => {
    startedRef.current.clear()
    groupDirsRef.current.clear()
    setItems((prev) => {
      const abortControllers = new Set<AbortController>()
      for (const item of prev) {
        abortControllers.add(item.abortController)
      }
      for (const abortController of abortControllers) {
        abortController.abort()
      }
      return []
    })
  }, [])

  const clearDone = useCallback(() => {
    setItems((prev) => {
      const kept = prev.filter((i) => i.status !== 'done')
      for (const item of prev) {
        if (item.status === 'done') {
          startedRef.current.delete(item.groupId)
        }
      }
      return kept
    })
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
