import type { ObjectInfo } from '@s4wave/web/object/object.pb.js'

export const APP_DRAG_MIME = 'application/x-s4wave-app-drag+json'
export const APP_DRAG_VERSION = 1 as const

export interface AppDragEnvelope {
  version: typeof APP_DRAG_VERSION
  items: AppDragItem[]
}

let activeAppDragEnvelope: AppDragEnvelope | null = null

export interface AppDragItem {
  id: string
  label?: string
  capabilities: AppDragCapability[]
}

export type AppDragCapability =
  | AppDragOpenableCapability
  | AppDragMovableCapability

export interface AppDragOpenableCapability {
  kind: 'openable'
  value: AppDragOpenableValue
}

export interface AppDragMovableCapability {
  kind: 'movable'
  value: AppDragMovableValue
}

export type AppDragOpenableValue = AppDragOpenableObjectValue

export interface AppDragOpenableObjectValue {
  case: 'object'
  value: {
    objectInfo: ObjectInfo
    path: string
    routePath?: string
    componentId?: string
  }
}

export type AppDragMovableValue = AppDragMovableUnixFSEntryValue

export interface AppDragMovableUnixFSEntryValue {
  case: 'unixfs-entry'
  value: {
    unixfsId: string
    path: string
    isDir: boolean
  }
}

interface AppDragReadDataTransfer {
  getData: (format: string) => string
  types?: ArrayLike<string> | readonly string[]
}

interface AppDragWriteDataTransfer {
  setData: (format: string, data: string) => void
}

interface AppDragFileDetectDataTransfer {
  items?: ArrayLike<{ kind: string }>
  types?: ArrayLike<string> | readonly string[]
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function getDataTransferTypes(
  dataTransfer:
    | AppDragReadDataTransfer
    | AppDragFileDetectDataTransfer
    | null
    | undefined,
): string[] {
  return Array.from(dataTransfer?.types ?? [])
}

function isAppDragOpenableValue(value: unknown): value is AppDragOpenableValue {
  if (!isRecord(value)) return false
  if (value.case !== 'object') return false
  if (!isRecord(value.value)) return false
  if (!isRecord(value.value.objectInfo)) return false
  if (
    value.value.routePath !== undefined &&
    typeof value.value.routePath !== 'string'
  ) {
    return false
  }
  return typeof value.value.path === 'string'
}

function isAppDragMovableValue(value: unknown): value is AppDragMovableValue {
  if (!isRecord(value)) return false
  if (value.case !== 'unixfs-entry') return false
  if (!isRecord(value.value)) return false
  if (typeof value.value.unixfsId !== 'string') return false
  if (typeof value.value.path !== 'string') return false
  return typeof value.value.isDir === 'boolean'
}

function isAppDragCapability(value: unknown): value is AppDragCapability {
  if (!isRecord(value)) return false
  if (value.kind === 'openable') return isAppDragOpenableValue(value.value)
  if (value.kind === 'movable') return isAppDragMovableValue(value.value)
  return false
}

function isAppDragItem(value: unknown): value is AppDragItem {
  if (!isRecord(value)) return false
  if (typeof value.id !== 'string') return false
  if (value.label !== undefined && typeof value.label !== 'string') return false
  if (!Array.isArray(value.capabilities)) return false
  return value.capabilities.every(isAppDragCapability)
}

export function isAppDragEnvelope(value: unknown): value is AppDragEnvelope {
  if (!isRecord(value)) return false
  if (value.version !== APP_DRAG_VERSION) return false
  if (!Array.isArray(value.items)) return false
  return value.items.every(isAppDragItem)
}

export function writeAppDragEnvelope(
  dataTransfer: AppDragWriteDataTransfer,
  envelope: AppDragEnvelope,
): void {
  activeAppDragEnvelope = envelope
  dataTransfer.setData(APP_DRAG_MIME, JSON.stringify(envelope))
}

export function clearActiveAppDragEnvelope(): void {
  activeAppDragEnvelope = null
}

export function readAppDragEnvelope(
  dataTransfer: AppDragReadDataTransfer | null | undefined,
): AppDragEnvelope | null {
  const raw = dataTransfer?.getData(APP_DRAG_MIME)
  if (!raw) return null
  try {
    const parsed: unknown = JSON.parse(raw)
    if (isAppDragEnvelope(parsed)) return parsed
  } catch {
    return null
  }
  return null
}

function readActiveAppDragEnvelope(
  dataTransfer: AppDragReadDataTransfer | null | undefined,
): AppDragEnvelope | null {
  if (!activeAppDragEnvelope) return null
  const types = getDataTransferTypes(dataTransfer)
  if (!types.includes(APP_DRAG_MIME)) return null
  return activeAppDragEnvelope
}

export function readAppDragEnvelopeWithActiveFallback(
  dataTransfer: AppDragReadDataTransfer | null | undefined,
): AppDragEnvelope | null {
  return (
    readAppDragEnvelope(dataTransfer) ?? readActiveAppDragEnvelope(dataTransfer)
  )
}

export function hasAppDragEnvelope(
  dataTransfer: AppDragReadDataTransfer | null | undefined,
): boolean {
  const types = getDataTransferTypes(dataTransfer)
  if (types.includes(APP_DRAG_MIME)) return true
  return readAppDragEnvelopeWithActiveFallback(dataTransfer) !== null
}

export function hasNativeFileDrag(
  dataTransfer: AppDragFileDetectDataTransfer | null | undefined,
): boolean {
  const items = Array.from(dataTransfer?.items ?? [])
  if (items.length > 0) {
    return items.some((item) => item.kind === 'file')
  }
  return getDataTransferTypes(dataTransfer).includes('Files')
}
