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
  treeUploadConcurrency,
  type TreeUploadConflict,
} from '@s4wave/sdk/unixfs/upload-pool.js'

// UploadStatus, UploadItem, UploadEvent, and UploadManager form the durable
// contract this hook owns. Each drop holds its own handle on its destination
// folder, cloned at addFiles time, so the manager can outlive the folder view
// that started it (see SessionUploadManagerContext).

// UploadStatus represents the state of a single upload item. checking lists
// the destination, or waits on the answer for the dropped folder the item is
// in, before any byte is sent; asking waits for the person to answer the
// item's conflict; skipped is an item the person chose not to send.
export type UploadStatus =
  | 'checking'
  | 'queued'
  | 'uploading'
  | 'asking'
  | 'done'
  | 'skipped'
  | 'error'

// UploadHolder is what already holds a dropped name, as the folder listing
// names it.
export type UploadHolder = 'file' | 'folder' | 'link'

// UploadConflict is what holds an asking item's name, and for a file the
// version a read returned, which Replace expects.
export interface UploadConflict {
  holds: UploadHolder
  version: Uint8Array | null
}

// UploadAnswer is the person's answer to a conflict. replace sends a file
// expecting the version the question showed; merge sends a dropped folder
// into the folder of its name, its files asking as files do; keep sends the
// item under the first free "name (n).ext"; skip leaves the name alone.
export type UploadAnswer = 'replace' | 'merge' | 'keep' | 'skip'

// UploadQuestion is the conflict on screen: the first asking item of the
// oldest drop with one, and the other items of that drop still asking.
export interface UploadQuestion {
  item: UploadItem
  others: UploadItem[]
}

// UploadItem tracks the state of a single file or folder upload.
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
  // place is the display path of the folder the drop targets.
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
  // The manager clones handle for the batch and releases the clone once no
  // item of the batch can send or retry, so navigating away from the folder
  // neither aborts the write nor breaks an answer given later. A file or
  // top-level folder whose name the folder already holds asks before any of
  // its bytes are sent.
  addFiles: (
    handle: FSHandle,
    files: File[],
    directories?: string[],
    place?: string,
  ) => void
  // answer answers the asking item id, and with all every other item of its
  // drop asking the same question, as UploadQuestion.others lists.
  answer: (id: string, choice: UploadAnswer, all: boolean) => void
  // retryUpload sends a failed item again with what it last expected.
  retryUpload: (id: string) => void
  cancelUpload: (id: string) => void
  cancelAll: () => void
  clearDone: () => void
}

// answersFor returns the answers a conflict offers: Replace for a file over
// a file, Merge for a folder over a folder, and always Keep both and Skip.
export function answersFor(item: UploadItem): UploadAnswer[] {
  const holds = item.conflict?.holds
  if (item.kind === 'file' && holds === 'file') {
    return ['replace', 'keep', 'skip']
  }
  if (item.kind === 'directory' && holds === 'folder') {
    return ['merge', 'keep', 'skip']
  }
  return ['keep', 'skip']
}

// splitPath splits a path relative to the drop's folder into its parent
// folder and name.
function splitPath(path: string): [string, string] {
  const cut = path.lastIndexOf('/')
  return cut < 0 ? ['', path] : [path.slice(0, cut), path.slice(cut + 1)]
}

// topOf returns the top-level dropped folder a path is inside, or null for a
// path at the top of the drop.
function topOf(path: string): string | null {
  const cut = path.indexOf('/')
  return cut < 0 ? null : path.slice(0, cut)
}

// holderOf names what a listing entry is.
function holderOf(entry: DirEntry): UploadHolder {
  if (entry.isDir) return 'folder'
  if (entry.isSymlink) return 'link'
  return 'file'
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

// freeName returns the first "name (n).ext", and its n, that taken lacks,
// from n = from, or from one past the number name already carries.
export function freeName(
  name: string,
  taken: Set<string>,
  from = 2,
): [string, number] {
  const dot = name.lastIndexOf('.')
  let [stem, ext] = dot > 0 ? [name.slice(0, dot), name.slice(dot)] : [name, '']
  const numbered = /^(.+) \((\d+)\)$/.exec(stem)
  if (numbered) {
    stem = numbered[1]
    from = Math.max(from, Number(numbered[2]) + 1)
  }
  for (let n = from; ; n++) {
    const next = `${stem} (${n})${ext}`
    if (!taken.has(next)) return [next, n]
  }
}

// gate returns a function that runs at most limit of its calls at once.
function gate(limit: number) {
  let active = 0
  const waiting: (() => void)[] = []
  return async <T>(run: () => Promise<T>): Promise<T> => {
    if (active >= limit) {
      await new Promise<void>((resolve) => waiting.push(resolve))
    }
    active++
    try {
      return await run()
    } finally {
      active--
      waiting.shift()?.()
    }
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

// alike returns the items of item's drop asking the same question as it:
// the same answers, so one answer can apply to all of them.
function alike(items: UploadItem[], item: UploadItem): UploadItem[] {
  const answers = answersFor(item).join()
  return items.filter(
    (current) =>
      current.groupId === item.groupId &&
      current.status === 'asking' &&
      answersFor(current).join() === answers,
  )
}

// questionOf returns the first asking item of the oldest drop with one.
function questionOf(items: UploadItem[]): UploadQuestion | null {
  const item = items.find((current) => current.status === 'asking')
  if (!item) return null
  const others = alike(items, item).filter((current) => current !== item)
  return { item, others }
}

const missing: TreeUploadExpect = { kind: 'missing' }

// UploadGroup is what the manager holds for one drop: its clone of the
// destination handle, each folder's listing, read once and reused by every
// question of the drop, and the names the drop claims in each folder.
interface UploadGroup {
  held: Promise<FSHandle>
  listings: Map<string, Promise<Map<string, DirEntry>>>
  claims: Map<string, Set<string>>
  // seen is set once the group's items are on the list, after which an
  // empty list releases it.
  seen: boolean
}

// useUploadManager projects the session-owned SDK upload pool into React state.
// A dropped file is sent only where its name is free, or over the version a
// Replace answer named; a dropped folder merges into one of its name only on
// a Merge answer. The drop lists its folder first and asks about each taken
// name before sending its bytes, and the store refuses at commit a file name
// another writer took meanwhile, which asks the same question.
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
  const groupsRef = useRef(new Map<string, UploadGroup>())

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

  // Release each drop's handle once none of its items can send or retry: an
  // item done, skipped or removed from the list never sends again.
  useEffect(() => {
    for (const [id, group] of groupsRef.current) {
      const live = items.some(
        (item) =>
          item.groupId === id &&
          item.status !== 'done' &&
          item.status !== 'skipped',
      )
      if (live) {
        group.seen = true
        continue
      }
      if (!group.seen) continue
      groupsRef.current.delete(id)
      void group.held.then((handle) => handle.release()).catch(() => {})
    }
  }, [items])

  // Release every held handle when the manager goes away.
  useEffect(() => {
    const groups = groupsRef.current
    return () => {
      for (const group of groups.values()) {
        void group.held.then((handle) => handle.release()).catch(() => {})
      }
      groups.clear()
    }
  }, [])

  // update merges patch into the item id.
  const update = useCallback((id: string, patch: Partial<UploadItem>) => {
    setItems((cur) =>
      cur.map((current) =>
        current.id === id ? { ...current, ...patch } : current,
      ),
    )
  }, [])

  // flow holds the steps an item takes from its drop to its commit. They call
  // each other, since a refusal asks or sends again, and depend only on the
  // pool, update and the held groups, so they are built once.
  const flow = useMemo(() => {
    const groups = groupsRef.current
    // reads bounds the listing and version reads to the pool's limit.
    const reads = gate(treeUploadConcurrency)

    // fail shows err on the item.
    const fail = (id: string, err: unknown) =>
      update(id, {
        status: 'error',
        error: err instanceof Error ? err.message : 'Upload failed',
      })

    // handleOf returns the item's drop's held handle.
    const handleOf = (item: UploadItem): Promise<FSHandle> => {
      const group = groups.get(item.groupId)
      if (!group) return Promise.reject(new Error('Upload was cancelled'))
      return group.held
    }

    // listing returns the drop's listing of parent, read once per drop; a
    // fresh listing reads it again.
    const listing = async (
      item: UploadItem,
      parent: string,
      fresh = false,
    ): Promise<Map<string, DirEntry>> => {
      const group = groups.get(item.groupId)
      if (!group) return new Map()
      let listed = fresh ? undefined : group.listings.get(parent)
      if (!listed) {
        // Cache the promise before any await, so the drop's other items
        // reuse this read rather than start their own.
        const signal = item.abortController.signal
        listed = group.held.then((handle) =>
          reads(() => listFolder(handle, parent, signal)),
        )
        group.listings.set(parent, listed)
      }
      return listed
    }

    // claim records that the drop sends name into parent.
    const claim = (item: UploadItem, parent: string, name: string) => {
      const claims = groups.get(item.groupId)?.claims
      if (!claims) return
      let names = claims.get(parent)
      if (!names) claims.set(parent, (names = new Set()))
      names.add(name)
    }

    // conflictOf returns what holds a taken name: its kind from the listing,
    // and for a file the version a one byte read returns.
    const conflictOf = async (
      item: UploadItem,
      entry: DirEntry,
    ): Promise<UploadConflict> => {
      const holds = holderOf(entry)
      if (holds !== 'file') return { holds, version: null }
      const handle = await handleOf(item)
      const signal = item.abortController.signal
      const version = await reads(() => readVersion(handle, item.path, signal))
      return { holds, version }
    }

    // ask puts the item in front of the person with what holds its name.
    const ask = (item: UploadItem, conflict: UploadConflict) => {
      if (item.abortController.signal.aborted) return
      update(item.id, { status: 'asking', conflict, bytesWritten: 0 })
    }

    // send queues one commit of the item at its path. A file expects expect;
    // a folder merges into whatever folder holds its path.
    const send = async (item: UploadItem, expect: TreeUploadExpect) => {
      const { id, file } = item
      if (item.abortController.signal.aborted) return
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
      let handle: FSHandle
      try {
        handle = await handleOf(item)
      } catch (err) {
        fail(id, err)
        return
      }
      const entry: TreeUploadEntry = file
        ? {
            kind: 'file',
            path: item.path,
            expect,
            totalSize: BigInt(item.totalSize),
            stream: file.stream(),
            onProgress: (bytesWritten) =>
              update(id, { bytesWritten: Number(bytesWritten) }),
          }
        : { kind: 'directory', path: item.path }
      const sent = { ...item, expect }
      pool.add(
        handle,
        entry,
        {
          onStart: () => update(id, { status: 'uploading' }),
          onComplete: () =>
            update(id, { status: 'done', bytesWritten: item.totalSize }),
          onConflict: (conflict) => void refused(sent, conflict),
          onError: (err) => fail(id, err),
        },
        item.abortController.signal,
      )
    }

    // place checks one file against its folder's listing: a free name is
    // sent as missing and a taken one asks.
    const place = async (item: UploadItem) => {
      const [parent, name] = splitPath(item.path)
      const entry = (await listing(item, parent)).get(name)
      if (!entry) {
        await send(item, missing)
        return
      }
      ask(item, await conflictOf(item, entry))
    }

    // inside returns the drop's waiting items inside the top-level folder.
    const inside = (top: UploadItem): UploadItem[] =>
      itemsRef.current.filter(
        (item) =>
          item.groupId === top.groupId &&
          item.status === 'checking' &&
          item !== top &&
          topOf(item.path) === top.path,
      )

    // open sends a top-level folder and what was dropped inside it. Into a
    // folder that held the name, each file is placed against its folder's
    // listing; into a new folder, each is sent as missing, and the store
    // refuses one another writer placed first.
    const open = async (
      top: UploadItem,
      into: UploadItem[],
      fresh: boolean,
    ) => {
      await send(top, missing)
      await Promise.all(
        into.map((item) =>
          item.kind === 'directory'
            ? send(item, missing)
            : fresh
              ? send(item, missing)
              : place(item),
        ),
      )
    }

    // keep sends the item under the first free "name (n).ext" in its folder.
    // Names in the folder's listing and names the drop claims are taken; a
    // refused name moves the next pick past its number, so a listing that
    // misses a taken name never picks it again. A kept folder takes what
    // was dropped inside it along to the new name.
    const keep = async (item: UploadItem, fresh = false) => {
      const signal = item.abortController.signal
      const from = item.kept?.from ?? item.path
      const [parent, original] = splitPath(from)
      update(item.id, { status: 'checking', conflict: null })
      const into = item.kind === 'directory' ? inside(item) : []
      const listed = await listing(item, parent, fresh)
      if (signal.aborted) return
      const claimed = groups.get(item.groupId)?.claims.get(parent)
      const taken = new Set([...listed.keys(), ...(claimed ?? [])])
      const [name, n] = freeName(original, taken, item.kept?.next)
      claim(item, parent, name)
      const path = parent ? `${parent}/${name}` : name
      const kept = { ...item, name, path, kept: { from, next: n + 1 } }
      if (item.kind === 'file') {
        await send(kept, missing)
        return
      }
      const moved = into.map((child) => {
        const childPath = path + child.path.slice(item.path.length)
        update(child.id, { path: childPath })
        return { ...child, path: childPath }
      })
      await open(kept, moved, true)
    }

    // refused handles the store's refusal of a file's send: Keep both takes
    // the next free name, a refusal with a version asks showing it, a
    // Replace whose file is gone sends as missing, and any other refusal
    // lists the folder again to ask with what holds the name.
    const refused = async (item: UploadItem, conflict: TreeUploadConflict) => {
      if (item.kept) {
        await keep(item, true)
        return
      }
      if (conflict.version) {
        ask(item, { holds: 'file', version: conflict.version })
        return
      }
      if (item.expect.kind === 'version') {
        await send(item, missing)
        return
      }
      const [parent, name] = splitPath(item.path)
      const entry = (await listing(item, parent, true)).get(name)
      if (!entry) {
        await send(item, missing)
        return
      }
      ask(item, await conflictOf(item, entry))
    }

    // check places a drop's top-level items against the destination's
    // listing. A top-level folder whose name is free opens at once; a taken
    // one asks, and what was dropped inside it waits on the answer.
    const check = async (dropped: UploadItem[]) => {
      const tops = dropped.filter((item) => topOf(item.path) === null)
      await Promise.all(
        tops.map(async (item) => {
          if (item.kind === 'file') {
            await place(item)
            return
          }
          const entry = (await listing(item, '')).get(item.path)
          const into = dropped.filter(
            (child) => child !== item && topOf(child.path) === item.path,
          )
          if (!entry) {
            await open(item, into, true)
            return
          }
          ask(item, await conflictOf(item, entry))
        }),
      )
    }

    // settle applies one answer to an asking item.
    const settle = async (item: UploadItem, choice: UploadAnswer) => {
      const into = item.kind === 'directory' ? inside(item) : []
      if (choice === 'skip') {
        for (const one of [item, ...into]) {
          update(one.id, { status: 'skipped', conflict: null })
        }
        return
      }
      if (choice === 'keep') {
        await keep(item)
        return
      }
      if (choice === 'merge') {
        await open(item, into, false)
        return
      }
      const version = item.conflict?.version
      await send(item, version ? { kind: 'version', version } : missing)
    }

    return { check, settle, send, fail }
  }, [pool, update])
  const { check, settle, send, fail } = flow

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
        place: place ?? '',
        expect: missing,
        kept: null,
        conflict: null,
        status: 'checking' as const,
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
        abortController: new AbortController(),
      }))
      // Every top-level folder of the drop gets an item to ask about, even
      // when only the files inside it were picked.
      const folders = new Set(directories ?? [])
      for (const item of fileItems) {
        const top = topOf(item.path)
        if (top) folders.add(top)
      }
      const dirItems: UploadItem[] = [...folders].map((directory) => ({
        ...base,
        id: `upload-${++nextIdRef.current}`,
        kind: 'directory',
        file: null,
        name: directory.split('/').at(-1) ?? directory,
        path: directory,
        totalSize: 0,
        abortController: new AbortController(),
      }))
      const newItems = [...fileItems, ...dirItems]
      if (newItems.length === 0) return

      // The drop holds its own handle on the destination until its last
      // item ends; the browsing handle goes when the person navigates.
      const held = handle.clone()
      held.catch(() => {})
      const claims = new Map<string, Set<string>>()
      for (const item of newItems) {
        const [parent, name] = splitPath(item.path)
        let names = claims.get(parent)
        if (!names) claims.set(parent, (names = new Set()))
        names.add(name)
      }
      groupsRef.current.set(groupId, {
        held,
        listings: new Map(),
        claims,
        seen: false,
      })

      itemsRef.current = [...itemsRef.current, ...newItems]
      setItems((prev) => [...prev, ...newItems])
      const fileCount = files.length || (directories?.length ?? 0)
      setLastEvent({
        id: ++eventSeqRef.current,
        kind: 'started',
        fileCount,
        errorCount: 0,
      })

      void check(newItems).catch((err: unknown) => {
        for (const item of newItems) fail(item.id, err)
      })
    },
    [check, fail],
  )

  const answer = useCallback(
    (id: string, choice: UploadAnswer, all: boolean) => {
      const cur = itemsRef.current
      const target = cur.find(
        (item) => item.id === id && item.status === 'asking',
      )
      if (!target || !answersFor(target).includes(choice)) return
      const answered = all ? alike(cur, target) : [target]
      for (const item of answered) {
        void settle(item, choice).catch((err: unknown) => fail(item.id, err))
      }
    },
    [fail, settle],
  )

  const retryUpload = useCallback(
    (id: string) => {
      const item = itemsRef.current.find(
        (current) => current.id === id && current.status === 'error',
      )
      if (item) void send(item, item.expect)
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
