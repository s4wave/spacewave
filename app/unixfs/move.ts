import type { FSHandle } from '@s4wave/sdk/unixfs/handle.js'
import type { FileEntry } from '@s4wave/web/editors/file-browser/types.js'

export interface UnixFSMoveItem {
  id: string
  name: string
  path: string
  isDir: boolean
}

export interface UnixFSDirectoryOption {
  path: string
  name: string
  depth: number
}

export interface UnixFSMoveValidation {
  accepted: boolean
  reason:
    | 'empty'
    | 'missing-name'
    | 'self'
    | 'same-parent'
    | 'descendant'
    | 'duplicate-source'
    | null
}

// splitUnixFSPath splits a UnixFS path into non-empty segments.
export function splitUnixFSPath(path: string): string[] {
  return path.split('/').filter(Boolean)
}

// getUnixFSParentPath returns the normalized parent path for a UnixFS path.
export function getUnixFSParentPath(path: string): string {
  const parts = splitUnixFSPath(path)
  if (parts.length <= 1) return '/'
  return '/' + parts.slice(0, -1).join('/')
}

// getUnixFSBaseName returns the final path segment for a UnixFS path.
export function getUnixFSBaseName(path: string): string {
  const parts = splitUnixFSPath(path)
  return parts.at(-1) ?? ''
}

// normalizeUnixFSHandlePath normalizes a handle path for root-relative lookups.
export function normalizeUnixFSHandlePath(path: string): string {
  if (!path || path === '/' || path === '.') return ''
  return path.replace(/^\/+/, '')
}

// isSameOrChildUnixFSPath returns true when childPath is the same as or below parentPath.
export function isSameOrChildUnixFSPath(
  parentPath: string,
  childPath: string,
): boolean {
  const parentParts = splitUnixFSPath(parentPath)
  const childParts = splitUnixFSPath(childPath)
  if (childParts.length < parentParts.length) return false
  return parentParts.every((part, index) => childParts[index] === part)
}

// buildUnixFSMoveItems maps browser entries in one directory into move items.
export function buildUnixFSMoveItems(
  currentPath: string,
  entries: Pick<FileEntry, 'id' | 'name' | 'isDir'>[],
): UnixFSMoveItem[] {
  const normalizedCurrentPath =
    !currentPath || currentPath === '/' || currentPath === '.' ?
      ''
    : '/' + currentPath.replace(/^\/+|\/+$/g, '')
  return entries.map((entry) => ({
    id: entry.id,
    name: entry.name,
    isDir: entry.isDir ?? false,
    path:
      normalizedCurrentPath ?
        `${normalizedCurrentPath}/${entry.name}`
      : `/${entry.name}`,
  }))
}

// validateUnixFSMove checks whether the items can move into the destination path.
export function validateUnixFSMove(
  items: UnixFSMoveItem[],
  destinationPath: string,
): UnixFSMoveValidation {
  if (items.length === 0) {
    return { accepted: false, reason: 'empty' }
  }
  const seenPaths = new Set<string>()
  for (const item of items) {
    if (!item.name) {
      return { accepted: false, reason: 'missing-name' }
    }
    if (seenPaths.has(item.path)) {
      return { accepted: false, reason: 'duplicate-source' }
    }
    seenPaths.add(item.path)
    if (item.path === destinationPath) {
      return { accepted: false, reason: 'self' }
    }
    if (getUnixFSParentPath(item.path) === destinationPath) {
      return { accepted: false, reason: 'same-parent' }
    }
    if (item.isDir && isSameOrChildUnixFSPath(item.path, destinationPath)) {
      return { accepted: false, reason: 'descendant' }
    }
  }
  return { accepted: true, reason: null }
}

// describeUnixFSMoveValidation formats a validation result for move UI feedback.
export function describeUnixFSMoveValidation(
  validation: UnixFSMoveValidation,
): string | null {
  switch (validation.reason) {
    case null:
      return null
    case 'empty':
      return 'Choose at least one item to move.'
    case 'missing-name':
      return 'One of the selected items is missing a valid name.'
    case 'self':
      return 'Cannot move an item into itself.'
    case 'same-parent':
      return 'The selected items are already in that folder.'
    case 'descendant':
      return 'Cannot move a folder into one of its descendants.'
    case 'duplicate-source':
      return 'The move request contains duplicate source entries.'
  }
}

async function collectUnixFSDirectories(
  handle: FSHandle,
  currentPath: string,
  depth: number,
  dirs: UnixFSDirectoryOption[],
  abortSignal?: AbortSignal,
): Promise<void> {
  dirs.push({
    path: currentPath,
    name: currentPath === '/' ? 'Root' : getUnixFSBaseName(currentPath),
    depth,
  })

  const entries = (await handle.readdirAll(undefined, abortSignal))
    .filter((entry) => (entry.isDir ?? false) && !!entry.name)
    .sort((a, b) => (a.name ?? '').localeCompare(b.name ?? ''))

  for (const entry of entries) {
    const name = entry.name
    if (!name) continue
    const childPath =
      currentPath === '/' ? `/${name}` : `${currentPath}/${name}`
    using childHandle = await handle.lookup(name, abortSignal)
    await collectUnixFSDirectories(
      childHandle,
      childPath,
      depth + 1,
      dirs,
      abortSignal,
    )
  }
}

// listUnixFSDirectories returns the root and all descendant directories for move selection.
export async function listUnixFSDirectories(
  rootHandle: FSHandle,
  abortSignal?: AbortSignal,
): Promise<UnixFSDirectoryOption[]> {
  const dirs: UnixFSDirectoryOption[] = []
  using root = await rootHandle.clone(abortSignal)
  await collectUnixFSDirectories(root, '/', 0, dirs, abortSignal)
  return dirs
}

async function lookupUnixFSMoveHandle(
  rootHandle: FSHandle,
  path: string,
  abortSignal?: AbortSignal,
): Promise<FSHandle> {
  const normalizedPath = normalizeUnixFSHandlePath(path)
  if (!normalizedPath) {
    return rootHandle.clone(abortSignal)
  }
  const { handle } = await rootHandle.lookupPath(normalizedPath, abortSignal)
  return handle
}

// moveUnixFSItems moves the items into the destination directory within one UnixFS root.
export async function moveUnixFSItems(
  rootHandle: FSHandle,
  items: UnixFSMoveItem[],
  destinationPath: string,
  abortSignal?: AbortSignal,
): Promise<void> {
  const validation = validateUnixFSMove(items, destinationPath)
  if (!validation.accepted) {
    throw new Error(`invalid unixfs move: ${validation.reason}`)
  }

  using destinationHandle = await lookupUnixFSMoveHandle(
    rootHandle,
    destinationPath,
    abortSignal,
  )

  const itemsByParent = new Map<string, UnixFSMoveItem[]>()
  for (const item of items) {
    const parentPath = getUnixFSParentPath(item.path)
    const group = itemsByParent.get(parentPath)
    if (group) {
      group.push(item)
      continue
    }
    itemsByParent.set(parentPath, [item])
  }

  for (const [parentPath, parentItems] of itemsByParent) {
    using sourceParentHandle = await lookupUnixFSMoveHandle(
      rootHandle,
      parentPath,
      abortSignal,
    )
    for (const item of parentItems) {
      await sourceParentHandle.rename(
        item.name,
        item.name,
        destinationHandle.id,
        abortSignal,
      )
    }
  }
}
