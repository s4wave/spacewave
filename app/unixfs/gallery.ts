import {
  getUnixFSDirEntryKind,
  getUnixFSFileInfoKind,
} from '@s4wave/sdk/unixfs/file-kind.js'
import { FSHandle } from '@s4wave/sdk/unixfs/handle.js'
import type { DirEntry } from '@s4wave/sdk/unixfs/handle.pb.js'
import {
  getUnixFSParentPath,
  getUnixFSRelativePath,
  joinUnixFSDisplayPath,
  normalizeUnixFSLookupPath,
} from '@s4wave/sdk/unixfs/path.js'
import { getMimeType } from '@s4wave/web/hooks/useUnixFSHandle.js'

// UnixFSGalleryCandidate describes one discovered gallery item.
export interface UnixFSGalleryCandidate {
  path: string
  name: string
  label: string
  mimeType: string
}

// UnixFSGalleryDiscoveryState tracks discovered gallery items plus non-fatal
// traversal errors encountered while walking the subtree.
export interface UnixFSGalleryDiscoveryState {
  scopePath: string
  items: UnixFSGalleryCandidate[]
  errors: string[]
  complete: boolean
}

const galleryMimeTypes = new Set<string>([
  'image/png',
  'image/jpeg',
  'image/gif',
  'image/webp',
  'image/svg+xml',
  'image/bmp',
  'image/tiff',
  'image/avif',
])

function getScopeRelativeLabel(scopeRoot: string, entryPath: string): string {
  return getUnixFSRelativePath(scopeRoot, entryPath)
}

function sortGalleryCandidates(
  candidates: UnixFSGalleryCandidate[],
): UnixFSGalleryCandidate[] {
  return candidates.toSorted((a, b) => a.label.localeCompare(b.label))
}

function buildDiscoveryState(
  scopePath: string,
  items: UnixFSGalleryCandidate[],
  errors: string[],
  complete: boolean,
): UnixFSGalleryDiscoveryState {
  return {
    scopePath,
    items: sortGalleryCandidates(items),
    errors: [...errors],
    complete,
  }
}

function formatDiscoveryError(scopePath: string, err: unknown): string {
  const msg = err instanceof Error ? err.message : String(err)
  return `${scopePath}: ${msg}`
}

function normalizeQueueError(err: unknown): Error {
  return err instanceof Error ? err : new Error(String(err))
}

class AsyncQueue<T> implements AsyncIterable<T> {
  private readonly values: T[] = []
  private readonly waiters: Array<(value: IteratorResult<T>) => void> = []
  private closed = false
  private err: unknown = null

  push(value: T): void {
    if (this.closed) return
    const waiter = this.waiters.shift()
    if (waiter) {
      waiter({ value, done: false })
      return
    }
    this.values.push(value)
  }

  fail(err: unknown): void {
    if (this.closed) return
    this.err = err
    this.close()
  }

  close(): void {
    if (this.closed) return
    this.closed = true
    for (const waiter of this.waiters.splice(0)) {
      waiter({ value: undefined, done: true })
    }
  }

  async *[Symbol.asyncIterator](): AsyncIterator<T> {
    for (;;) {
      if (this.values.length > 0) {
        yield this.values.shift() as T
        continue
      }
      if (this.closed) {
        if (this.err) throw normalizeQueueError(this.err)
        return
      }
      const next = await new Promise<IteratorResult<T>>((resolve) => {
        this.waiters.push(resolve)
      })
      if (next.done) {
        if (this.err) throw normalizeQueueError(this.err)
        return
      }
      yield next.value
    }
  }
}

async function resolveGalleryScope(
  rootHandle: FSHandle,
  requestedScopePath: string,
  signal: AbortSignal,
): Promise<{
  handle: FSHandle
  scopePath: string
}> {
  const normalizedRequestedPath = normalizeUnixFSLookupPath(requestedScopePath)
  const requestedHandle = normalizedRequestedPath
    ? (await rootHandle.lookupPath(normalizedRequestedPath, signal)).handle
    : await rootHandle.clone(signal)
  const info = await requestedHandle.getFileInfo(signal)
  if (getUnixFSFileInfoKind(info) === 'directory') {
    return {
      handle: requestedHandle,
      scopePath: requestedScopePath || '/',
    }
  }
  requestedHandle[Symbol.dispose]()

  const scopePath = getUnixFSParentPath(requestedScopePath || '/')
  const normalizedScopePath = normalizeUnixFSLookupPath(scopePath)
  const handle = normalizedScopePath
    ? (await rootHandle.lookupPath(normalizedScopePath, signal)).handle
    : await rootHandle.clone(signal)
  return {
    handle,
    scopePath,
  }
}

async function walkGalleryScope(
  handle: FSHandle,
  scopeRoot: string,
  scopePath: string,
  signal: AbortSignal,
): Promise<UnixFSGalleryCandidate[]> {
  const info = await handle.getFileInfo(signal)
  if (getUnixFSFileInfoKind(info) !== 'directory') {
    return []
  }

  const entries = (await handle.readdirAll(0n, signal)).toSorted((a, b) =>
    (a.name ?? '').localeCompare(b.name ?? ''),
  )
  const images: UnixFSGalleryCandidate[] = []
  for (const entry of entries) {
    if (!entry.name) {
      continue
    }

    const entryPath = joinUnixFSDisplayPath(scopePath || '/', entry.name)
    // eslint-disable-next-line react-doctor/async-await-in-loop -- recursive walk keeps one scoped child ref live at a time.
    using child = await handle.lookup(entry.name, signal)
    if (getUnixFSDirEntryKind(entry) === 'directory') {
      images.push(
        ...(await walkGalleryScope(child, scopeRoot, entryPath, signal)),
      )
      continue
    }

    const mimeType = getMimeType(entry.name)
    if (!galleryMimeTypes.has(mimeType)) {
      continue
    }
    images.push({
      path: entryPath,
      name: entry.name,
      label: getScopeRelativeLabel(scopeRoot, entryPath),
      mimeType,
    })
  }
  return sortGalleryCandidates(images)
}

function makeChildAbortController(parent: AbortSignal): AbortController {
  const ctrl = new AbortController()
  if (parent.aborted) {
    ctrl.abort()
    return ctrl
  }
  parent.addEventListener('abort', () => ctrl.abort(), { once: true })
  return ctrl
}

class LiveGalleryScope {
  private readonly queue = new AsyncQueue<UnixFSGalleryDiscoveryState>()
  private readonly dirs = new Set<WatchedGalleryDir>()
  private readonly errors: string[] = []
  private lastStateKey = ''
  private rootDir: WatchedGalleryDir | null = null
  private stopped = false

  constructor(
    readonly scopeRoot: string,
    private readonly rootHandle: FSHandle,
    private readonly signal: AbortSignal,
  ) {
    signal.addEventListener('abort', () => this.stop(), { once: true })
  }

  stream(): AsyncIterable<UnixFSGalleryDiscoveryState> {
    return this.queue
  }

  async start(): Promise<void> {
    this.emit(false)
    this.rootDir = this.createDir(this.rootHandle, this.scopeRoot)
    await this.rootDir.start()
    this.emit(true)
  }

  stop(): void {
    if (this.stopped) return
    this.stopped = true
    this.rootDir?.stop()
    this.queue.close()
  }

  private async startDir(
    handle: FSHandle,
    path: string,
  ): Promise<WatchedGalleryDir> {
    const dir = this.createDir(handle, path)
    await dir.start()
    return dir
  }

  private createDir(handle: FSHandle, path: string): WatchedGalleryDir {
    const dir = new WatchedGalleryDir(
      handle,
      path,
      this.scopeRoot,
      this.signal,
      this,
    )
    this.dirs.add(dir)
    return dir
  }

  removeDir(dir: WatchedGalleryDir): void {
    this.dirs.delete(dir)
  }

  async watchChildDir(
    parent: WatchedGalleryDir,
    name: string,
    path: string,
  ): Promise<WatchedGalleryDir> {
    const handle = await parent.handle.lookup(name, parent.signal)
    return this.startDir(handle, path)
  }

  addError(path: string, err: unknown): void {
    this.errors.push(formatDiscoveryError(path, err))
  }

  emit(complete: boolean): void {
    const state = buildDiscoveryState(
      this.scopeRoot,
      [...this.dirs].flatMap((dir) => [...dir.images.values()]),
      this.errors,
      complete,
    )
    const stateKey = JSON.stringify(state)
    if (stateKey === this.lastStateKey) {
      return
    }
    this.lastStateKey = stateKey
    this.queue.push(state)
  }

  fail(err: unknown): void {
    this.queue.fail(err)
  }
}

class WatchedGalleryDir {
  readonly abort: AbortController
  readonly children = new Map<string, WatchedGalleryDir>()
  readonly images = new Map<string, UnixFSGalleryCandidate>()

  constructor(
    readonly handle: FSHandle,
    readonly path: string,
    private readonly scopeRoot: string,
    private readonly parentSignal: AbortSignal,
    private readonly scope: LiveGalleryScope,
  ) {
    this.abort = makeChildAbortController(parentSignal)
  }

  get signal(): AbortSignal {
    return this.abort.signal
  }

  async start(): Promise<void> {
    try {
      const iter = this.handle.watchReaddir(this.signal)[Symbol.asyncIterator]()
      const first = await iter.next()
      if (!first.done) {
        await this.applyEntries(first.value ?? [])
      }
      void this.watch(iter)
    } catch (err) {
      if (!this.signal.aborted) {
        this.scope.addError(this.path, err)
        this.scope.emit(true)
      }
    }
  }

  stop(): void {
    this.abort.abort()
    for (const child of this.children.values()) {
      child.stop()
    }
    this.children.clear()
    this.images.clear()
    this.scope.removeDir(this)
    this.handle[Symbol.dispose]()
  }

  private async watch(iter: AsyncIterator<DirEntry[]>): Promise<void> {
    try {
      for (;;) {
        const next = await iter.next()
        if (next.done) return
        await this.applyEntries(next.value ?? [])
        this.scope.emit(true)
      }
    } catch (err) {
      if (!this.signal.aborted) {
        this.scope.addError(this.path, err)
        this.scope.emit(true)
      }
    } finally {
      await iter.return?.()
    }
  }

  private async applyEntries(entries: DirEntry[]): Promise<void> {
    const nextChildren = new Set<string>()
    const nextImages = new Map<string, UnixFSGalleryCandidate>()
    const sortedEntries = entries.toSorted((a, b) =>
      (a.name ?? '').localeCompare(b.name ?? ''),
    )

    for (const entry of sortedEntries) {
      if (!entry.name) {
        continue
      }

      const entryPath = joinUnixFSDisplayPath(this.path || '/', entry.name)
      if (getUnixFSDirEntryKind(entry) === 'directory') {
        nextChildren.add(entry.name)
        if (!this.children.has(entry.name)) {
          try {
            this.children.set(
              entry.name,
              // eslint-disable-next-line react-doctor/async-await-in-loop -- directory watchers are attached sequentially to preserve child owner order.
              await this.scope.watchChildDir(this, entry.name, entryPath),
            )
          } catch (err) {
            this.scope.addError(entryPath, err)
          }
        }
        continue
      }

      const mimeType = getMimeType(entry.name)
      if (!galleryMimeTypes.has(mimeType)) {
        continue
      }
      nextImages.set(entry.name, {
        path: entryPath,
        name: entry.name,
        label: getScopeRelativeLabel(this.scopeRoot, entryPath),
        mimeType,
      })
    }

    for (const [name, child] of this.children) {
      if (!nextChildren.has(name)) {
        child.stop()
        this.children.delete(name)
      }
    }
    this.images.clear()
    for (const [name, image] of nextImages) {
      this.images.set(name, image)
    }
  }
}

// collectUnixFSGalleryCandidates walks the scoped UnixFS subtree and returns
// image-like file candidates discovered under that path.
export async function collectUnixFSGalleryCandidates(
  rootHandle: FSHandle,
  scopePath: string,
  signal: AbortSignal,
): Promise<UnixFSGalleryCandidate[]> {
  const resolvedScope = await resolveGalleryScope(
    rootHandle,
    scopePath || '/',
    signal,
  )
  const scopeRoot = resolvedScope.scopePath
  using scopeHandle = resolvedScope.handle
  return walkGalleryScope(scopeHandle, scopeRoot, scopeRoot, signal)
}

// streamUnixFSGalleryCandidates watches the scoped UnixFS subtree and streams
// the complete discovered gallery state whenever a watched directory changes.
export async function* streamUnixFSGalleryCandidates(
  rootHandle: FSHandle,
  scopePath: string,
  signal: AbortSignal,
): AsyncIterable<UnixFSGalleryDiscoveryState> {
  const resolvedScope = await resolveGalleryScope(
    rootHandle,
    scopePath || '/',
    signal,
  )
  const liveScope = new LiveGalleryScope(
    resolvedScope.scopePath,
    resolvedScope.handle,
    signal,
  )
  try {
    void liveScope.start().catch((err: unknown) => {
      if (!signal.aborted) {
        liveScope.fail(err)
      }
    })
    yield* liveScope.stream()
  } finally {
    liveScope.stop()
  }
}
