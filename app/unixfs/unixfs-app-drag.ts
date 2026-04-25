import type { FileEntry } from '@s4wave/web/editors/file-browser/types.js'
import {
  type AppDragEnvelope,
  APP_DRAG_VERSION,
  readAppDragEnvelopeWithActiveFallback,
  type AppDragMovableUnixFSEntryValue,
} from '@s4wave/web/dnd/app-drag.js'
import {
  type ObjectInfo,
  type UnixfsObjectInfo,
} from '@s4wave/web/object/object.pb.js'

function joinPath(base: string, name: string): string {
  if (base.endsWith('/')) return base + name
  return base + '/' + name
}

function buildSpaceObjectPath(objectKey: string, objectPath: string): string {
  const strippedObjectPath = objectPath.replace(/^\/+/, '')
  if (!strippedObjectPath) return objectKey
  return `${objectKey}/-/${strippedObjectPath}`
}

export interface BuildUnixFSEntryAppDragParams {
  entry: FileEntry
  currentPath: string
  sessionIndex: number | null
  spaceId: string | null
  unixfsId: string
}

export interface BuildUnixFSSelectionAppDragParams {
  entries: FileEntry[]
  currentPath: string
  sessionIndex: number | null
  spaceId: string | null
  unixfsId: string
  movableEntryIds?: string[]
}

export interface UnixFSMovableAppDragItem {
  id: string
  label?: string
  value: AppDragMovableUnixFSEntryValue['value']
}

function buildUnixFSEntryAppDragItem(
  entry: FileEntry,
  currentPath: string,
  sessionIndex: number | null,
  spaceId: string | null,
  unixfsId: string,
  includeMovable: boolean,
): AppDragEnvelope['items'][number] {
  const entryPath = joinPath(currentPath, entry.name)
  const capabilities: AppDragEnvelope['items'][number]['capabilities'] = []
  if (includeMovable) {
    capabilities.push({
      kind: 'movable',
      value: {
        case: 'unixfs-entry',
        value: {
          unixfsId,
          path: entryPath,
          isDir: entry.isDir ?? false,
        },
      },
    })
  }

  if (sessionIndex != null && spaceId) {
    const objectInfo: ObjectInfo = {
      info: {
        case: 'unixfsObjectInfo',
        value: {
          unixfsId,
          path: entryPath,
        } satisfies UnixfsObjectInfo,
      },
    }
    const routePath = `/u/${sessionIndex}/so/${spaceId}/-/${buildSpaceObjectPath(unixfsId, entryPath)}`
    capabilities.unshift({
      kind: 'openable',
      value: {
        case: 'object',
        value: {
          objectInfo,
          path: '',
          routePath,
        },
      },
    })
  }

  return {
    id: entry.id,
    label: entry.name,
    capabilities,
  }
}

export function buildUnixFSSelectionAppDragEnvelope(
  params: BuildUnixFSSelectionAppDragParams,
): AppDragEnvelope | null {
  const {
    entries,
    currentPath,
    sessionIndex,
    spaceId,
    unixfsId,
    movableEntryIds = entries.map((entry) => entry.id),
  } = params
  if (entries.length === 0) return null
  const movableEntryIdSet = new Set(movableEntryIds)

  const items = entries.map((entry) =>
    buildUnixFSEntryAppDragItem(
      entry,
      currentPath,
      sessionIndex,
      spaceId,
      unixfsId,
      movableEntryIdSet.has(entry.id),
    ),
  )

  return {
    version: APP_DRAG_VERSION,
    items,
  }
}

export function buildUnixFSEntryAppDragEnvelope(
  params: BuildUnixFSEntryAppDragParams,
): AppDragEnvelope | null {
  return buildUnixFSSelectionAppDragEnvelope({
    entries: [params.entry],
    currentPath: params.currentPath,
    sessionIndex: params.sessionIndex,
    spaceId: params.spaceId,
    unixfsId: params.unixfsId,
    movableEntryIds: [params.entry.id],
  })
}

export function readUnixFSMovableAppDragItem(
  dataTransfer: Pick<DataTransfer, 'getData' | 'types'> | null | undefined,
): UnixFSMovableAppDragItem | null {
  return readUnixFSMovableAppDragItems(dataTransfer)[0] ?? null
}

export function readUnixFSMovableAppDragItems(
  dataTransfer: Pick<DataTransfer, 'getData' | 'types'> | null | undefined,
): UnixFSMovableAppDragItem[] {
  const envelope = readAppDragEnvelopeWithActiveFallback(dataTransfer)
  if (!envelope) return []

  const items: UnixFSMovableAppDragItem[] = []

  for (const item of envelope.items) {
    for (const capability of item.capabilities) {
      if (capability.kind !== 'movable') continue
      if (capability.value.case !== 'unixfs-entry') continue
      items.push({
        id: item.id,
        label: item.label,
        value: capability.value.value,
      })
    }
  }

  return items
}
