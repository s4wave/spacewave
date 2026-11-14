import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
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

// UploadStatus represents the state of a single upload item. waiting is an
// item inside a dropped folder that has not opened it yet: one being created,
// asking or failed; checking lists the item's folder before any byte is sent;
// asking waits for the person to answer the item's conflict; skipped is an
// item the person chose not to send; error is a failed listing or send, which
// Retry runs again.
export type UploadStatus =
  | 'waiting'
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

// UploadStep is the step an item last ran, which Retry runs again after it
// failed: check lists the item's folder and asks or sends, keep lists it and
// picks a free name, send commits with what the item expects, and open
// commits a folder and then sends what was dropped inside it. A failed
// listing never leads to a send.
export type UploadStep = 'check' | 'keep' | 'send' | 'open'

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
  // step is the step Retry runs again.
  step: UploadStep
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

// holderOf names what a listing entry is.
function holderOf(entry: DirEntry): UploadHolder {
  if (entry.isDir) return 'folder'
  if (entry.isSymlink) return 'link'
  return 'file'
}

// listFolder returns the entries of the folder at path below handle by name.
// It rejects when the folder cannot be listed; an empty listing would send
// into names it never saw.
async function listFolder(
  handle: FSHandle,
  path: string,
  signal: AbortSignal,
): Promise<Map<string, DirEntry>> {
  if (!path) return byName(await handle.readdirAll(undefined, signal))
  const { handle: folder } = await handle.lookupPath(path, signal)
  try {
    return byName(await folder.readdirAll(undefined, signal))
  } finally {
    folder.release()
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
// overwrite sends a file over whatever holds its name; a folder sent so
// merges into the folder of its name.
const overwrite: TreeUploadExpect = { kind: 'overwrite' }

// UploadGroup is what the manager holds for one drop: the handle it was
// dropped on, its clone of it, each folder's listing, read once and reused by
// every question of the drop, and the tree of its items.
interface UploadGroup {
  source: FSHandle
  held: Promise<FSHandle>
  // broken is set once held rejects; Retry then takes a new clone.
  broken: boolean
  // abort ends the drop's shared listings when the drop ends; no one item's
  // cancel ends a read its siblings wait on.
  abort: AbortController
  // listings holds each folder's listing read, by path below the drop's
  // folder. A failed read leaves it, so the next one reads again.
  listings: Map<string, Promise<Map<string, DirEntry>>>
  // children lists the items dropped directly inside each dropped folder,
  // by the folder's item id, and the top of the drop under ''. A folder's
  // items keep their names when Keep both renames it, so a walk builds each
  // path from its parent's.
  children: Map<string, UploadItem[]>
  // place is the display path of the drop's folder, which keys its claims.
  place: string
  // claimed lists the claims the drop holds, as folder key and name.
  claimed: [string, string][]
  // seen is set once the group's items are on the list, after which an
  // empty list releases it.
  seen: boolean
}

// hold clones the drop's handle into group, dropping listings read through
// an earlier clone.
function hold(group: UploadGroup) {
  const held = group.source.clone()
  group.held = held
  group.broken = false
  group.listings.clear()
  held.catch(() => {
    if (group.held === held) group.broken = true
  })
}

// Claims are the names the manager's drops send into each folder, keyed by
// the folder's display path, with how many drops claim each. Keep both picks
// past them, so no two drops into one folder pick the same name. A key two
// Spaces share only moves a pick past a number it could have taken.
type Claims = Map<string, Map<string, number>>

// folderKey returns the key of the folder at parent below the drop's place.
function folderKey(group: UploadGroup, parent: string): string {
  return parent ? `${group.place}/${parent}` : group.place
}

// release ends group's reads and drops its handle and its claims.
function release(group: UploadGroup, claims: Claims) {
  group.abort.abort()
  void group.held.then((handle) => handle.release()).catch(() => {})
  for (const [key, name] of group.claimed) {
    const names = claims.get(key)
    const count = names?.get(name) ?? 0
    if (count > 1) names?.set(name, count - 1)
    else names?.delete(name)
    if (names?.size === 0) claims.delete(key)
  }
  group.claimed = []
}

// useUploadManager projects the session-owned SDK upload pool into React state.
// A dropped file is sent only where its name is free, or over the version a
// Replace answer named; a dropped folder merges into one of its name only on
// a Merge answer. The drop lists its folder first and asks about each taken
// name before sending its bytes, and the store refuses at commit a name
// another writer took meanwhile, which asks the same question. A new folder
// is sent alone, and what was dropped inside it only once it commits, so no
// file of the drop creates it first.
export function useUploadManager(): UploadManager {
  const [items, setItems] = useState<UploadItem[]>([])
  const [lastEvent, setLastEvent] = useState<UploadEvent | null>(null)
  // itemsRef holds the manager's current items, with patchesRef the changes
  // not yet applied to them; items is the last list published to render.
  // Every press decides from these, never from a rendered list, so a second
  // press on a settled item finds it settled.
  const itemsRef = useRef(items)
  const patchesRef = useRef(new Map<string, Partial<UploadItem>>())

  // publish makes next the current items and renders it.
  const publish = useCallback((next: UploadItem[]) => {
    itemsRef.current = next
    setItems(next)
  }, [])

  // flush applies the waiting patches in one pass over the list.
  const flush = useCallback(() => {
    const patches = patchesRef.current
    if (patches.size === 0) return
    const batch = new Map(patches)
    patches.clear()
    publish(
      itemsRef.current.map((current) => {
        const next = batch.get(current.id)
        return next ? { ...current, ...next } : current
      }),
    )
  }, [publish])

  const nextIdRef = useRef(0)
  const nextGroupIdRef = useRef(0)
  const eventSeqRef = useRef(0)
  const wasInProgressRef = useRef(false)
  const poolRef = useRef<TreeUploadPool | null>(null)
  if (!poolRef.current) poolRef.current = new TreeUploadPool()
  const pool = poolRef.current
  const groupsRef = useRef(new Map<string, UploadGroup>())
  const claimsRef = useRef<Claims>(new Map())

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
      flush()
      publish(itemsRef.current.filter((item) => item.status === 'error'))
    }, 3000)
    return () => clearTimeout(timer)
  }, [items, flush, publish])

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
      release(group, claimsRef.current)
    }
  }, [items])

  // Release every held handle when the manager goes away.
  useEffect(() => {
    const groups = groupsRef.current
    const claims = claimsRef.current
    return () => {
      for (const group of groups.values()) release(group, claims)
      groups.clear()
    }
  }, [])

  // update merges patch into the item id. Patches made in one task apply
  // together in one pass over the list, so opening a folder of n items costs
  // one pass rather than n; a press flushes them first.
  const update = useCallback(
    (id: string, patch: Partial<UploadItem>) => {
      const patches = patchesRef.current
      if (patches.size === 0) queueMicrotask(flush)
      patches.set(id, { ...patches.get(id), ...patch })
    },
    [flush],
  )

  // flow holds the steps an item takes from its drop to its commit. They call
  // each other, since a refusal asks or sends again, and depend only on the
  // pool, update and the held groups, so they are built once.
  const flow = useMemo(() => {
    const groups = groupsRef.current
    const claims = claimsRef.current
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

    // listing returns the drop's listing of parent, read once per drop under
    // the drop's signal; a fresh listing reads it again. A failed read leaves
    // the cache, so the next asker reads again.
    const listing = (
      item: UploadItem,
      parent: string,
      fresh: boolean,
    ): Promise<Map<string, DirEntry>> => {
      const group = groups.get(item.groupId)
      if (!group) return Promise.reject(new Error('Upload was cancelled'))
      let listed = fresh ? undefined : group.listings.get(parent)
      if (!listed) {
        // Cache the promise before any await, so the drop's other items
        // reuse this read rather than start their own.
        const signal = group.abort.signal
        const read = group.held.then((handle) =>
          reads(() => listFolder(handle, parent, signal)),
        )
        group.listings.set(parent, read)
        read.catch(() => {
          if (group.listings.get(parent) === read) {
            group.listings.delete(parent)
          }
        })
        listed = read
      }
      return listed
    }

    // listed returns the listing of the item's parent, or null after failing
    // the item at step, which Retry runs again.
    const listed = async (
      item: UploadItem,
      parent: string,
      step: UploadStep,
      fresh = false,
    ): Promise<Map<string, DirEntry> | null> => {
      try {
        return await listing(item, parent, fresh)
      } catch (err) {
        if (!item.abortController.signal.aborted) {
          const why = err instanceof Error ? err.message : String(err)
          update(item.id, {
            status: 'error',
            error: `Could not list the folder: ${why}`,
            step,
          })
        }
        return null
      }
    }

    // childrenOf returns the items dropped directly inside the folder.
    const childrenOf = (folder: UploadItem): UploadItem[] =>
      groups.get(folder.groupId)?.children.get(folder.id) ?? []

    // below returns every item dropped inside the folder, from the drop's
    // tree of item ids, whatever path a Keep both gave the folder.
    const below = (folder: UploadItem): UploadItem[] => {
      const found: UploadItem[] = []
      const walk = (at: UploadItem) => {
        for (const child of childrenOf(at)) {
          found.push(child)
          walk(child)
        }
      }
      walk(folder)
      return found
    }

    // claim records that the item's drop sends name into parent.
    const claim = (item: UploadItem, parent: string, name: string) => {
      const group = groups.get(item.groupId)
      if (!group) return
      const key = folderKey(group, parent)
      let names = claims.get(key)
      if (!names) claims.set(key, (names = new Map()))
      names.set(name, (names.get(name) ?? 0) + 1)
      group.claimed.push([key, name])
    }

    // claimed returns the names every drop claims in the item's parent.
    const claimed = (item: UploadItem, parent: string): Iterable<string> => {
      const group = groups.get(item.groupId)
      if (!group) return []
      return claims.get(folderKey(group, parent))?.keys() ?? []
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

    // send commits the item at its path expecting expect, recording step for
    // Retry, and returns whether it committed. A folder expecting missing is
    // created only where nothing holds its path; any other folder merges into
    // the folder there. A refusal or failure settles the item before send
    // returns false, as does a cancel.
    const send = async (
      item: UploadItem,
      expect: TreeUploadExpect,
      step: UploadStep = 'send',
    ): Promise<boolean> => {
      const { id, file } = item
      const signal = item.abortController.signal
      if (signal.aborted) return false
      update(id, {
        status: 'queued',
        step,
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
        return false
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
        : {
            kind: 'directory',
            path: item.path,
            expectMissing: expect.kind === 'missing',
          }
      const sent = { ...item, expect }
      return new Promise<boolean>((resolve) => {
        // The pool drops a cancelled entry without a callback.
        const cancelled = () => resolve(false)
        const settle = (committed: boolean) => {
          signal.removeEventListener('abort', cancelled)
          resolve(committed)
        }
        signal.addEventListener('abort', cancelled, { once: true })
        pool.add(
          handle,
          entry,
          {
            onStart: () => update(id, { status: 'uploading' }),
            onComplete: () => {
              update(id, { status: 'done', bytesWritten: item.totalSize })
              settle(true)
            },
            onConflict: (conflict) => {
              settle(false)
              void refused(sent, conflict)
            },
            onError: (err) => {
              fail(id, err)
              settle(false)
            },
          },
          signal,
        )
      })
    }

    // place checks one file against its folder's listing: a free name is
    // sent as missing and a taken one asks.
    const place = async (item: UploadItem, fresh = false) => {
      const [parent, name] = splitPath(item.path)
      update(item.id, { status: 'checking', error: undefined })
      const entries = await listed(item, parent, 'check', fresh)
      if (!entries) return
      const entry = entries.get(name)
      if (!entry) {
        await send(item, missing)
        return
      }
      ask(item, await conflictOf(item, entry))
    }

    // inside returns the items dropped directly inside the folder, each at
    // its name in the folder's path.
    const inside = (folder: UploadItem): UploadItem[] =>
      childrenOf(folder).map((child) => ({
        ...child,
        path: `${folder.path}/${child.name}`,
      }))

    // open sends a dropped folder alone and, once it commits, what was
    // dropped inside it, which waits until then. A new folder expects
    // nothing at its path, so one another writer created after the listing
    // is refused and asks; everything below it then goes at once, files as
    // missing, so the store refuses a name another writer placed inside it
    // first, and folders merging, since the drop created the folder they
    // sit in: a subfolder another writer makes there after the folder
    // commits is merged into without asking. A folder merging into the folder of its name places what is
    // inside it against its folder's listing.
    const open = async (folder: UploadItem, fresh: boolean) => {
      if (!(await send(folder, fresh ? missing : overwrite, 'open'))) return
      if (fresh) {
        await fill(folder)
        return
      }
      await Promise.all(
        inside(folder).map((item) =>
          item.kind === 'file' ? place(item) : enter(item, true),
        ),
      )
    }

    // fill sends everything dropped inside a folder the drop created.
    const fill = async (folder: UploadItem): Promise<void> => {
      await Promise.all(
        inside(folder).map((item) =>
          item.kind === 'file'
            ? send(item, missing)
            : Promise.all([send(item, overwrite), fill(item)]),
        ),
      )
    }

    // enter checks a dropped folder against its parent's listing: a free
    // name opens as new; a folder of its name opens into it when the drop
    // already merged its parent, and otherwise asks; a file or link of its
    // name asks. What was dropped inside an asking or failed folder waits
    // on it.
    const enter = async (
      folder: UploadItem,
      merged: boolean,
      fresh = false,
    ) => {
      const [parent, name] = splitPath(folder.path)
      update(folder.id, { status: 'checking', error: undefined })
      const entries = await listed(folder, parent, 'check', fresh)
      if (!entries) return
      const entry = entries.get(name)
      if (!entry) {
        await open(folder, true)
        return
      }
      if (merged && entry.isDir) {
        await open(folder, false)
        return
      }
      ask(folder, await conflictOf(folder, entry))
    }

    // keep sends the item under the first free "name (n).ext" in its folder.
    // Names in the folder's listing and names any drop claims there are
    // taken; a refused name moves the next pick past its number, so a
    // listing that misses a taken name never picks it again. A kept folder
    // takes what was dropped inside it along to the new name.
    const keep = async (item: UploadItem, fresh = false) => {
      const signal = item.abortController.signal
      const from = item.kept?.from ?? item.path
      const [parent, original] = splitPath(from)
      update(item.id, { status: 'checking', conflict: null, error: undefined })
      const entries = await listed(item, parent, 'keep', fresh)
      if (!entries || signal.aborted) return
      const taken = new Set([...entries.keys(), ...claimed(item, parent)])
      const [name, n] = freeName(original, taken, item.kept?.next)
      claim(item, parent, name)
      const path = parent ? `${parent}/${name}` : name
      const kept = { ...item, name, path, kept: { from, next: n + 1 } }
      if (item.kind === 'directory') await open(kept, true)
      else await send(kept, missing)
    }

    // refused handles the store's refusal of a send: Keep both takes the
    // next free name, a new folder lists its parent again and asks, never
    // merging unasked, a refusal with a version asks showing it, a Replace
    // whose file is gone sends as missing, and any other refusal lists the
    // folder again to ask with what holds the name.
    const refused = async (item: UploadItem, conflict: TreeUploadConflict) => {
      if (item.kept) {
        await keep(item, true)
        return
      }
      if (item.kind === 'directory') {
        await enter(item, false, true)
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
      await place(item, true)
    }

    // check places a drop's top-level items against the destination's
    // listing. A top-level folder whose name is free opens at once; a taken
    // one asks, and what was dropped inside it waits on the answer.
    const check = async (group: UploadGroup) => {
      await Promise.all(
        (group.children.get('') ?? []).map((item) =>
          item.kind === 'file' ? place(item) : enter(item, false),
        ),
      )
    }

    // settle applies one answer to an asking item.
    const settle = async (item: UploadItem, choice: UploadAnswer) => {
      if (choice === 'skip') {
        for (const one of [item, ...below(item)]) {
          update(one.id, { status: 'skipped', conflict: null })
        }
        return
      }
      if (choice === 'keep') {
        await keep(item)
        return
      }
      if (choice === 'merge') {
        await open(item, false)
        return
      }
      const version = item.conflict?.version
      await send(item, version ? { kind: 'version', version } : missing)
    }

    // resume runs a failed item's step again, reading any listing again. A
    // folder below the top of the drop is only ever checked inside a merge.
    const resume = async (item: UploadItem) => {
      if (item.step === 'open')
        return open(item, item.expect.kind === 'missing')
      if (item.step === 'send') return send(item, item.expect)
      if (item.step === 'keep') return keep(item, true)
      if (item.kind === 'file') return place(item, true)
      return enter(item, splitPath(item.path)[0] !== '', true)
    }

    // retry runs a failed item's step again; when the drop's clone of its
    // handle failed, it takes a new clone and runs every failed item of the
    // drop again.
    const retry = (item: UploadItem) => {
      const group = groups.get(item.groupId)
      let failed = [item]
      if (group?.broken) {
        hold(group)
        failed = itemsRef.current.filter(
          (current) =>
            current.groupId === item.groupId && current.status === 'error',
        )
      }
      for (const one of failed) {
        void resume(one).catch((err: unknown) => fail(one.id, err))
      }
    }

    return { check, settle, retry, fail, below }
  }, [pool, update])
  const { check, settle, retry, fail, below } = flow

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
        step: 'check' as const,
        expect: missing,
        kept: null,
        conflict: null,
      }
      // An item at the top of the drop is checked at once; one inside a
      // dropped folder waits until the folder opens.
      const statusOf = (path: string): UploadStatus =>
        path.includes('/') ? 'waiting' : 'checking'
      const fileItems: UploadItem[] = files.map((file) => {
        const path =
          (file as File & { webkitRelativePath?: string }).webkitRelativePath ||
          file.name
        return {
          ...base,
          id: `upload-${++nextIdRef.current}`,
          kind: 'file',
          file,
          name: file.name,
          path,
          status: statusOf(path),
          totalSize: file.size,
          abortController: new AbortController(),
        }
      })
      // Every folder of the drop gets an item to ask about, even when only
      // the files inside it were picked.
      const folders = new Set<string>()
      for (const path of [
        ...(directories ?? []),
        ...fileItems.map((item) => splitPath(item.path)[0]),
      ]) {
        for (let at = path; at; at = splitPath(at)[0]) folders.add(at)
      }
      const dirItems: UploadItem[] = [...folders].map((directory) => ({
        ...base,
        id: `upload-${++nextIdRef.current}`,
        kind: 'directory',
        file: null,
        name: directory.split('/').at(-1) ?? directory,
        path: directory,
        status: statusOf(directory),
        totalSize: 0,
        abortController: new AbortController(),
      }))
      const newItems = [...fileItems, ...dirItems]
      if (newItems.length === 0) return

      // The drop holds its own handle on the destination until its last
      // item ends; the browsing handle goes when the person navigates.
      // The drop's tree indexes each item under its folder's item, read
      // once per drop.
      const byPath = new Map(newItems.map((item) => [item.path, item]))
      const children = new Map<string, UploadItem[]>()
      for (const item of newItems) {
        const parent = splitPath(item.path)[0]
        const key = parent ? (byPath.get(parent)?.id ?? '') : ''
        let list = children.get(key)
        if (!list) children.set(key, (list = []))
        list.push(item)
      }
      const group: UploadGroup = {
        source: handle,
        held: Promise.resolve(handle),
        broken: false,
        abort: new AbortController(),
        listings: new Map(),
        children,
        place: place ?? '',
        claimed: [],
        seen: false,
      }
      hold(group)
      groupsRef.current.set(groupId, group)
      const claims = claimsRef.current
      for (const item of newItems) {
        const [parent, name] = splitPath(item.path)
        const key = folderKey(group, parent)
        let names = claims.get(key)
        if (!names) claims.set(key, (names = new Map()))
        names.set(name, (names.get(name) ?? 0) + 1)
        group.claimed.push([key, name])
      }

      publish([...itemsRef.current, ...newItems])
      const fileCount = files.length || (directories?.length ?? 0)
      setLastEvent({
        id: ++eventSeqRef.current,
        kind: 'started',
        fileCount,
        errorCount: 0,
      })

      void check(group).catch((err: unknown) => {
        for (const item of newItems) fail(item.id, err)
      })
    },
    [check, fail, publish],
  )

  // answer applies only to items asking in the manager's current state; the
  // first step of every answer leaves asking at once, so a second press on
  // the same question finds nothing to answer.
  const answer = useCallback(
    (id: string, choice: UploadAnswer, all: boolean) => {
      flush()
      const cur = itemsRef.current
      const target = cur.find(
        (item) => item.id === id && item.status === 'asking',
      )
      if (!target || !answersFor(target).includes(choice)) return
      const answered = all ? alike(cur, target) : [target]
      for (const item of answered) {
        void settle(item, choice).catch((err: unknown) => fail(item.id, err))
      }
      flush()
    },
    [fail, settle, flush],
  )

  const retryUpload = useCallback(
    (id: string) => {
      flush()
      const item = itemsRef.current.find(
        (current) => current.id === id && current.status === 'error',
      )
      if (item) retry(item)
      flush()
    },
    [retry, flush],
  )

  // cancelUpload stops the item and, for a folder, everything dropped inside
  // it, whatever state each is in, and takes them off the list.
  const cancelUpload = useCallback(
    (id: string) => {
      flush()
      const prev = itemsRef.current
      const item = prev.find((current) => current.id === id)
      if (!item) return
      const ids = new Set([item, ...below(item)].map((one) => one.id))
      const gone = (current: UploadItem) => ids.has(current.id)
      for (const current of prev) {
        if (gone(current) && !finished(current)) {
          current.abortController.abort()
        }
      }
      publish(prev.filter((current) => !gone(current)))
    },
    [below, flush, publish],
  )

  const cancelAll = useCallback(() => {
    flush()
    for (const item of itemsRef.current) item.abortController.abort()
    publish([])
  }, [flush, publish])

  const clearDone = useCallback(() => {
    flush()
    publish(
      itemsRef.current.filter(
        (item) => item.status !== 'done' && item.status !== 'skipped',
      ),
    )
  }, [flush, publish])

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
