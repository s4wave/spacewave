import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from 'react'
import type { DirEntry } from '@s4wave/sdk/unixfs/handle.pb.js'
import type {
  FSHandle,
  TreeUploadEntry,
  TreeUploadExpect,
} from '@s4wave/sdk/unixfs/handle.js'

import {
  TreeUploadPool,
  type TreeUploadConflict,
} from '@s4wave/sdk/unixfs/upload-pool.js'

// UploadStatus, UploadItem, UploadEvent, and UploadManager form the durable
// contract this hook owns. The manager holds no viewer or route identity: each
// upload group captures its own target handle at addFiles time, so the manager
// can outlive the folder view that started it (see SessionUploadManagerContext).

// UploadStatus represents the state of a single upload item. checking lists
// the destination before any byte is sent; asking waits for the person to
// answer the item's conflict; skipped is a file the person chose not to send.
export type UploadStatus =
  | 'checking'
  | 'queued'
  | 'uploading'
  | 'asking'
  | 'done'
  | 'skipped'
  | 'error'

// UploadConflict is what holds an asking item's name: a file at version, or,
// with version null, a folder or link the file cannot replace.
export interface UploadConflict {
  version: Uint8Array | null
}

// UploadAnswer is the person's answer to a conflict. replace sends the file
// expecting the version the question showed; keep sends it under the first
// free "name (n).ext"; skip leaves the name alone.
export type UploadAnswer = 'replace' | 'keep' | 'skip'

// UploadQuestion is the conflict on screen: the first asking item of the
// oldest drop with one, and the other items of that drop still asking.
export interface UploadQuestion {
  item: UploadItem
  others: UploadItem[]
}

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
  // handle is the folder the drop targets, and place its display path.
  handle: FSHandle
  place: string
  // expect is what the item's last send expected, which Retry sends again.
  expect: TreeUploadExpect
  // kept is set after a Keep both answer: the dropped path, and the number
  // a refusal of the name sent starts the next free name from.
  kept: { from: string; next: number } | null
  // conflict is set while the item is asking.
  conflict: UploadConflict | null
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
  // question is the conflict waiting for an answer, or null.
  question: UploadQuestion | null
  // addFiles enqueues a batch against handle, whose display path is place.
  // The handle is captured into the batch here, not held by the manager, so
  // navigating away from the folder that started the upload neither aborts
  // the write nor drops the feedback. A file whose name the folder already
  // holds asks before any of its bytes are sent.
  addFiles: (
    handle: FSHandle,
    files: File[],
    directories?: string[],
    place?: string,
  ) => void
  // answer answers the asking item id, and with all every other item of its
  // drop that is asking; replace applies only to items whose name holds a
  // file.
  answer: (id: string, choice: UploadAnswer, all: boolean) => void
  // retryUpload sends a failed item again with what it last expected.
  retryUpload: (id: string) => void
  cancelUpload: (id: string) => void
  cancelAll: () => void
  clearDone: () => void
}

// splitPath splits a path relative to the drop's folder into its parent
// folder and name.
function splitPath(path: string): [string, string] {
  const cut = path.lastIndexOf('/')
  return cut < 0 ? ['', path] : [path.slice(0, cut), path.slice(cut + 1)]
}

// listFolder returns the entries of the folder at path below handle by name,
// or none when it cannot be listed, such as a folder the drop will create.
// The store's check at commit stays the authority, so a missed entry only
// moves its question after the send.
async function listFolder(
  handle: FSHandle,
  path: string,
  signal: AbortSignal,
): Promise<Map<string, DirEntry>> {
  try {
    if (!path) return byName(await handle.readdirAll(undefined, signal))
    const { handle: folder } = await handle.lookupPath(path, signal)
    try {
      return byName(await folder.readdirAll(undefined, signal))
    } finally {
      folder.release()
    }
  } catch {
    return new Map()
  }
}

function byName(entries: DirEntry[]): Map<string, DirEntry> {
  return new Map(entries.map((entry) => [entry.name ?? '', entry]))
}

// readVersion returns the version of the file at path below handle from a one
// byte read, or null when it cannot be read; the store then reports the
// version at commit.
async function readVersion(
  handle: FSHandle,
  path: string,
  signal: AbortSignal,
): Promise<Uint8Array | null> {
  try {
    const { handle: file } = await handle.lookupPath(path, signal)
    try {
      const { version } = await file.readAt(0n, 1n, signal)
      return version.byteLength ? version : null
    } finally {
      file.release()
    }
  } catch {
    return null
  }
}

// freeName returns the first "name (n).ext", and its n, from n = from that
// taken lacks.
export function freeName(
  name: string,
  taken: Set<string>,
  from = 2,
): [string, number] {
  const dot = name.lastIndexOf('.')
  const [stem, ext] =
    dot > 0 ? [name.slice(0, dot), name.slice(dot)] : [name, '']
  for (let n = from; ; n++) {
    const next = `${stem} (${n})${ext}`
    if (!taken.has(next)) return [next, n]
  }
}

// finished reports an item that waits on nothing.
function finished(item: UploadItem): boolean {
  return (
    item.status === 'done' ||
    item.status === 'skipped' ||
    item.status === 'error'
  )
}

// questionOf returns the first asking item of the oldest drop with one.
function questionOf(items: UploadItem[]): UploadQuestion | null {
  const item = items.find((current) => current.status === 'asking')
  if (!item) return null
  const others = items.filter(
    (current) =>
      current !== item &&
      current.groupId === item.groupId &&
      current.status === 'asking',
  )
  return { item, others }
}

const missing: TreeUploadExpect = { kind: 'missing' }

// useUploadManager projects the session-owned SDK upload pool into React state.
// Each addFiles call captures its target handle, so uploads can outlive the
// folder view that started them. A dropped file is sent only where its name
// is free, or over the version a Replace answer named: the drop lists its
// folder first and asks about each taken name before sending its bytes, and
// the store refuses at commit a name another writer took meanwhile, which
// asks the same question.
export function useUploadManager(): UploadManager {
  const [items, setItems] = useState<UploadItem[]>([])
  const [lastEvent, setLastEvent] = useState<UploadEvent | null>(null)
  // itemsRef holds the committed items for answer and retryUpload.
  const itemsRef = useRef(items)
  useLayoutEffect(() => {
    itemsRef.current = items
  }, [items])

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
  const question = useMemo(() => questionOf(items), [items])

  // Emit a completion event on the transition from any in-progress items to
  // every item reaching a terminal state. Runs before the auto-clear below, so
  // items are still present and consumers can anchor feedback to the indicator.
  useEffect(() => {
    const inProgress = items.some((item) => !finished(item))
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

  // Auto-clear completed and skipped uploads after a delay.
  useEffect(() => {
    if (items.length === 0) return
    if (!items.every(finished)) return

    const timer = setTimeout(() => {
      setItems((prev) => prev.filter((item) => item.status === 'error'))
    }, 3000)
    return () => clearTimeout(timer)
  }, [items])

  // update merges patch into the item id.
  const update = useCallback((id: string, patch: Partial<UploadItem>) => {
    setItems((cur) =>
      cur.map((current) =>
        current.id === id ? { ...current, ...patch } : current,
      ),
    )
  }, [])

  // flow holds the steps a file takes from its drop to its commit. They call
  // each other, since a refusal asks or sends again, and depend only on the
  // pool and update, so they are built once.
  const flow = useMemo(() => {
    // ask puts the item in front of the person with what holds its name.
    const ask = (item: UploadItem, version: Uint8Array | null) => {
      if (item.abortController.signal.aborted) return
      update(item.id, {
        status: 'asking',
        conflict: { version },
        bytesWritten: 0,
      })
    }

    // send queues one commit of the item's file at its path, expecting expect.
    const send = (item: UploadItem, expect: TreeUploadExpect) => {
      const { id, file } = item
      if (!file || item.abortController.signal.aborted) return
      const sent = { ...item, expect }
      update(id, {
        status: 'queued',
        path: item.path,
        name: item.name,
        expect,
        kept: item.kept,
        conflict: null,
        bytesWritten: 0,
        error: undefined,
      })
      const entry: TreeUploadEntry = {
        kind: 'file',
        path: item.path,
        expect,
        totalSize: BigInt(item.totalSize),
        stream: file.stream(),
        onProgress: (bytesWritten) =>
          update(id, { bytesWritten: Number(bytesWritten) }),
      }
      pool.add(
        item.handle,
        entry,
        {
          onStart: () => update(id, { status: 'uploading' }),
          onComplete: () =>
            update(id, { status: 'done', bytesWritten: item.totalSize }),
          onConflict: (conflict) => refused(sent, conflict),
          onError: (err) =>
            update(id, {
              status: 'error',
              error: err instanceof Error ? err.message : 'Upload failed',
            }),
        },
        item.abortController.signal,
      )
    }

    // keep sends the item under the first free "name (n).ext" in its
    // folder. A refused name moves the next pick past its number, so a
    // listing that misses a taken name never picks it again.
    const keep = async (item: UploadItem) => {
      const signal = item.abortController.signal
      const from = item.kept?.from ?? item.path
      const [parent, original] = splitPath(from)
      update(item.id, { status: 'checking', conflict: null })
      const taken = new Set(
        (await listFolder(item.handle, parent, signal)).keys(),
      )
      if (signal.aborted) return
      const [name, n] = freeName(original, taken, item.kept?.next)
      const path = parent ? `${parent}/${name}` : name
      send({ ...item, name, path, kept: { from, next: n + 1 } }, missing)
    }

    // refused handles the store's refusal of a send: a taken name asks, a
    // Replace whose file is gone sends as missing, and Keep both takes the
    // next free name.
    const refused = (item: UploadItem, conflict: TreeUploadConflict) => {
      if (item.kept) {
        void keep(item)
        return
      }
      if (conflict.version) {
        ask(item, conflict.version)
        return
      }
      if (item.expect.kind === 'version') {
        send(item, missing)
        return
      }
      ask(item, null)
    }

    // check lists the folders of a drop's files and asks about each taken
    // name; every other file is sent as missing. The listings stop only
    // once every file of the drop is cancelled.
    const check = async (handle: FSHandle, files: UploadItem[]) => {
      const listings = new Map<string, Promise<Map<string, DirEntry>>>()
      const stop = new AbortController()
      const signals = files.map((item) => item.abortController.signal)
      for (const signal of signals) {
        signal.addEventListener(
          'abort',
          () => {
            if (signals.every((current) => current.aborted)) stop.abort()
          },
          { once: true },
        )
      }
      await Promise.all(
        files.map(async (item) => {
          const [parent, name] = splitPath(item.path)
          let listing = listings.get(parent)
          if (!listing) {
            listing = listFolder(handle, parent, stop.signal)
            listings.set(parent, listing)
          }
          const entry = (await listing).get(name)
          if (!entry) {
            send(item, missing)
            return
          }
          if (entry.isDir || entry.isSymlink) {
            ask(item, null)
            return
          }
          const signal = item.abortController.signal
          const version = await readVersion(handle, item.path, signal)
          if (version) ask(item, version)
          else send(item, missing)
        }),
      )
    }

    return { send, keep, check }
  }, [pool, update])
  const { send, keep, check } = flow

  const addFiles = useCallback(
    (
      handle: FSHandle,
      files: File[],
      directories?: string[],
      place?: string,
    ) => {
      const groupId = `upload-group-${++nextGroupIdRef.current}`
      const base = {
        groupId,
        bytesWritten: 0,
        handle,
        place: place ?? '',
        expect: missing,
        kept: null,
        conflict: null,
      }
      const fileItems: UploadItem[] = files.map((file) => ({
        ...base,
        id: `upload-${++nextIdRef.current}`,
        kind: 'file',
        file,
        name: file.name,
        path:
          (file as File & { webkitRelativePath?: string }).webkitRelativePath ||
          file.name,
        totalSize: file.size,
        status: 'checking',
        abortController: new AbortController(),
      }))
      const dirItems: UploadItem[] = (directories ?? []).map((directory) => ({
        ...base,
        id: `upload-${++nextIdRef.current}`,
        kind: 'directory',
        file: null,
        name: directory.split('/').at(-1) ?? directory,
        path: directory,
        totalSize: 0,
        status: 'queued',
        abortController: new AbortController(),
      }))
      const newItems = [...fileItems, ...dirItems]
      if (newItems.length === 0) return

      setItems((prev) => [...prev, ...newItems])
      const fileCount = files.length || (directories?.length ?? 0)
      setLastEvent({
        id: ++eventSeqRef.current,
        kind: 'started',
        fileCount,
        errorCount: 0,
      })

      // A folder merges into one of the same name, so it is sent at once.
      for (const item of dirItems) {
        pool.add(
          handle,
          { kind: 'directory', path: item.path },
          {
            onStart: () => update(item.id, { status: 'uploading' }),
            onComplete: () => update(item.id, { status: 'done' }),
            onError: (err) =>
              update(item.id, {
                status: 'error',
                error: err instanceof Error ? err.message : 'Upload failed',
              }),
          },
          item.abortController.signal,
        )
      }
      if (fileItems.length) void check(handle, fileItems)
    },
    [check, pool, update],
  )

  const answer = useCallback(
    (id: string, choice: UploadAnswer, all: boolean) => {
      const cur = itemsRef.current
      const target = cur.find(
        (item) => item.id === id && item.status === 'asking',
      )
      if (!target) return
      if (choice === 'replace' && !target.conflict?.version) return
      const answered = all
        ? cur.filter(
            (item) =>
              item.groupId === target.groupId && item.status === 'asking',
          )
        : [target]
      for (const item of answered) {
        if (choice === 'skip') {
          update(item.id, { status: 'skipped', conflict: null })
        } else if (choice === 'keep') {
          void keep(item)
        } else if (item.conflict?.version) {
          send(item, { kind: 'version', version: item.conflict.version })
        }
      }
    },
    [keep, send, update],
  )

  const retryUpload = useCallback(
    (id: string) => {
      const item = itemsRef.current.find(
        (current) =>
          current.id === id &&
          current.status === 'error' &&
          current.kind === 'file',
      )
      if (item) send(item, item.expect)
    },
    [send],
  )

  const cancelUpload = useCallback((id: string) => {
    setItems((prev) => {
      const item = prev.find((current) => current.id === id)
      if (!item) return prev
      if (!finished(item)) item.abortController.abort()
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
    setItems((prev) =>
      prev.filter(
        (item) => item.status !== 'done' && item.status !== 'skipped',
      ),
    )
  }, [])

  return useMemo(
    () => ({
      items,
      activeCount,
      lastEvent,
      question,
      addFiles,
      answer,
      retryUpload,
      cancelUpload,
      cancelAll,
      clearDone,
    }),
    [
      items,
      activeCount,
      lastEvent,
      question,
      addFiles,
      answer,
      retryUpload,
      cancelUpload,
      cancelAll,
      clearDone,
    ],
  )
}
