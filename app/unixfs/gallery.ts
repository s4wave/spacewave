import { FSHandle } from '@s4wave/sdk/unixfs/handle.js'
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

function normalizeScopePath(path: string): string {
  if (!path || path === '/') {
    return ''
  }
  return path.split('/').filter(Boolean).join('/')
}

function joinPath(base: string, name: string): string {
  if (!base || base === '/') {
    return '/' + name
  }
  if (base.endsWith('/')) {
    return base + name
  }
  return `${base}/${name}`
}

function splitPath(path: string): string[] {
  return path.split('/').filter(Boolean)
}

function getParentPath(path: string): string {
  const parts = splitPath(path)
  if (parts.length <= 1) {
    return '/'
  }
  return '/' + parts.slice(0, -1).join('/')
}

function getScopeRelativeLabel(scopeRoot: string, entryPath: string): string {
  const normalizedScopeRoot = normalizeScopePath(scopeRoot)
  const normalizedEntryPath = normalizeScopePath(entryPath)
  if (!normalizedScopeRoot) {
    return normalizedEntryPath
  }
  if (!normalizedEntryPath.startsWith(normalizedScopeRoot + '/')) {
    return normalizedEntryPath
  }
  return normalizedEntryPath.slice(normalizedScopeRoot.length + 1)
}

function sortGalleryCandidates(
  candidates: UnixFSGalleryCandidate[],
): UnixFSGalleryCandidate[] {
  return [...candidates].sort((a, b) => a.label.localeCompare(b.label))
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

async function resolveGalleryScope(
  rootHandle: FSHandle,
  requestedScopePath: string,
  signal: AbortSignal,
): Promise<{
  handle: FSHandle
  scopePath: string
}> {
  const normalizedRequestedPath = normalizeScopePath(requestedScopePath)
  const requestedHandle =
    normalizedRequestedPath ?
      (await rootHandle.lookupPath(normalizedRequestedPath, signal)).handle
    : await rootHandle.clone(signal)
  const info = await requestedHandle.getFileInfo(signal)
  if (info.isDir ?? false) {
    return {
      handle: requestedHandle,
      scopePath: requestedScopePath || '/',
    }
  }
  requestedHandle[Symbol.dispose]()

  const scopePath = getParentPath(requestedScopePath || '/')
  const normalizedScopePath = normalizeScopePath(scopePath)
  const handle =
    normalizedScopePath ?
      (await rootHandle.lookupPath(normalizedScopePath, signal)).handle
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
  if (!(info.isDir ?? false)) {
    return []
  }

  const entries = [...(await handle.readdirAll(0n, signal))].sort((a, b) =>
    (a.name ?? '').localeCompare(b.name ?? ''),
  )
  const images: UnixFSGalleryCandidate[] = []
  for (const entry of entries) {
    if (!entry.name) {
      continue
    }

    const entryPath = joinPath(scopePath || '/', entry.name)
    using child = await handle.lookup(entry.name, signal)
    if (entry.isDir ?? false) {
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

async function* streamGalleryScope(
  handle: FSHandle,
  scopeRoot: string,
  scopePath: string,
  signal: AbortSignal,
  discovered: UnixFSGalleryCandidate[],
  errors: string[],
): AsyncIterable<UnixFSGalleryDiscoveryState> {
  const info = await handle.getFileInfo(signal)
  if (!(info.isDir ?? false)) {
    return
  }

  const entries = [...(await handle.readdirAll(0n, signal))].sort((a, b) =>
    (a.name ?? '').localeCompare(b.name ?? ''),
  )
  for (const entry of entries) {
    if (!entry.name) {
      continue
    }

    const entryPath = joinPath(scopePath || '/', entry.name)
    try {
      using child = await handle.lookup(entry.name, signal)
      if (entry.isDir ?? false) {
        yield* streamGalleryScope(
          child,
          scopeRoot,
          entryPath,
          signal,
          discovered,
          errors,
        )
        continue
      }

      const mimeType = getMimeType(entry.name)
      if (!galleryMimeTypes.has(mimeType)) {
        continue
      }
      discovered.push({
        path: entryPath,
        name: entry.name,
        label: getScopeRelativeLabel(scopeRoot, entryPath),
        mimeType,
      })
      yield buildDiscoveryState(scopeRoot, discovered, errors, false)
    } catch (err) {
      errors.push(formatDiscoveryError(entryPath, err))
      yield buildDiscoveryState(scopeRoot, discovered, errors, false)
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

// streamUnixFSGalleryCandidates streams discovered gallery items as the scoped
// subtree walk progresses.
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
  const scopeRoot = resolvedScope.scopePath
  const discovered: UnixFSGalleryCandidate[] = []
  const errors: string[] = []
  yield buildDiscoveryState(scopeRoot, discovered, errors, false)
  using scopeHandle = resolvedScope.handle
  yield* streamGalleryScope(
    scopeHandle,
    scopeRoot,
    scopeRoot,
    signal,
    discovered,
    errors,
  )
  yield buildDiscoveryState(scopeRoot, discovered, errors, true)
}
