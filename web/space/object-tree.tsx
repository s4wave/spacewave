import React from 'react'
import {
  LuBookOpen,
  LuBox,
  LuBot,
  LuBrainCircuit,
  LuCheck,
  LuClipboardList,
  LuDatabase,
  LuFile,
  LuFileQuestion,
  LuFileText,
  LuFolder,
  LuGitBranch,
  LuGauge,
  LuLayoutGrid,
  LuListChecks,
  LuListFilter,
  LuMessageSquareText,
  LuNetwork,
  LuPaintbrush,
  LuScale,
  LuTable,
  LuWrench,
} from 'react-icons/lu'

import {
  SPACE_SETTINGS_BLOCK_TYPE,
  SPACE_SETTINGS_OBJECT_KEY,
} from '@s4wave/core/space/world/world.js'
import type { TreeNode } from '@s4wave/web/ui/tree/TreeNode.js'
import type { WorldContentsObject } from '@s4wave/core/space/world/world.pb.js'
import {
  ObjectTypeVisibility,
  type ObjectTypeMetadata,
  type ObjectTypeRegistration,
} from '@s4wave/sdk/objecttype/registry/registry.pb.js'

// ObjectTreeNode holds metadata for a node in the object tree.
export interface ObjectTreeNode {
  objectKey: string
  objectType: string
  objectTypeLabel?: string
  objectTypeDescription?: string
  isVirtual: boolean
}

export interface SpaceObjectActionTarget {
  objectKey: string
  objectType: string
  label: string
  objectTypeLabel: string
  objectTypeDescription: string
}

export type ObjectTypeMetadataById = ReadonlyMap<string, ObjectTypeMetadata>

// HIDDEN_OBJECT_TYPES is the set of object types hidden from the tree.
export const HIDDEN_OBJECT_TYPES = new Set([
  'space/settings',
  SPACE_SETTINGS_BLOCK_TYPE,
])

// isHiddenSpaceObject returns whether an object should be hidden from space
// object browsers and pickers.
export function isHiddenSpaceObject(
  objectKey?: string,
  objectType?: string,
  metadataById?: ObjectTypeMetadataById,
): boolean {
  if ((objectKey ?? '') === SPACE_SETTINGS_OBJECT_KEY) return true
  if (isHiddenObjectTypeMetadata(metadataById?.get(objectType ?? ''))) {
    return true
  }
  return HIDDEN_OBJECT_TYPES.has(objectType ?? '')
}

const iconSize = 'h-3.5 w-3.5'

const metadataIcons: Record<
  string,
  React.ComponentType<{ className?: string }>
> = {
  'book-open': LuBookOpen,
  bot: LuBot,
  'brain-circuit': LuBrainCircuit,
  check: LuCheck,
  'clipboard-list': LuClipboardList,
  file: LuFile,
  'file-question': LuFileQuestion,
  'file-text': LuFileText,
  gauge: LuGauge,
  'git-branch': LuGitBranch,
  'layout-grid': LuLayoutGrid,
  'list-checks': LuListChecks,
  'list-filter': LuListFilter,
  'message-square-text': LuMessageSquareText,
  network: LuNetwork,
  paintbrush: LuPaintbrush,
  scale: LuScale,
  table: LuTable,
  wrench: LuWrench,
}

export function buildObjectTypeMetadataMap(
  registrations: readonly ObjectTypeRegistration[],
): ObjectTypeMetadataById {
  const map = new Map<string, ObjectTypeMetadata>()
  const sorted = [...registrations].toSorted(
    (a, b) => (a.registrationId ?? 0) - (b.registrationId ?? 0),
  )
  for (const registration of sorted) {
    const typeId = registration.typeId ?? ''
    if (!typeId || !registration.metadata || map.has(typeId)) continue
    map.set(typeId, registration.metadata)
  }
  return map
}

// getObjectTypeIconComponent returns the icon component for an ObjectType.
export function getObjectTypeIconComponent(
  typeId: string,
  metadataById?: ObjectTypeMetadataById,
  fallback: React.ComponentType<{ className?: string }> = LuBox,
): React.ComponentType<{ className?: string }> {
  const iconName = metadataById?.get(typeId)?.iconName?.trim()
  if (iconName) return metadataIcons[iconName] ?? fallback

  switch (typeId) {
    case 'alpha/object-layout':
      return LuLayoutGrid
    case 'unixfs/fs-node':
      return LuFile
    case 'git/repo':
    case 'git/worktree':
      return LuGitBranch
    case 'canvas':
      return LuPaintbrush
    case 'kv/store':
    case 'sql/db':
      return LuDatabase
    case 'sql/query':
      return LuFileText
    case 'sql/schema':
    case 'sql/query-result':
      return LuTable
    case 'sql/table-view':
      return LuListFilter
    case 'sql/workbench':
      return LuLayoutGrid
    default:
      return fallback
  }
}

// getObjectTypeIcon returns the icon element for a given object type ID.
export function getObjectTypeIcon(
  typeId: string,
  metadataById?: ObjectTypeMetadataById,
): React.ReactNode {
  const Icon = getObjectTypeIconComponent(typeId, metadataById)
  return <Icon className={iconSize} />
}

// getObjectTypeLabel returns a human-readable label for a given object type ID.
export function getObjectTypeLabel(
  typeId: string,
  metadataById?: ObjectTypeMetadataById,
): string {
  const displayName = metadataById?.get(typeId)?.displayName?.trim()
  if (displayName) return displayName

  switch (typeId) {
    case 'alpha/object-layout':
      return 'Layout'
    case 'unixfs/fs-node':
      return 'File System'
    case 'git/repo':
      return 'Git Repository'
    case 'git/worktree':
      return 'Git Worktree'
    case 'canvas':
      return 'Canvas'
    case 'kv/store':
      return 'Key/Value Store'
    case 'sql/db':
      return 'SQL Database'
    case 'sql/query':
      return 'SQL Query'
    case 'sql/query-result':
      return 'SQL Query Result'
    case 'sql/schema':
      return 'SQL Schema'
    case 'sql/table-view':
      return 'SQL Table View'
    case 'sql/workbench':
      return 'SQL Workbench'
    default:
      return typeId || 'Object'
  }
}

export function getObjectTypeDescription(
  typeId: string,
  metadataById?: ObjectTypeMetadataById,
): string {
  return metadataById?.get(typeId)?.description?.trim() ?? ''
}

export function getObjectDisplayName(objectKey: string): string {
  const key = objectKey.trim()
  switch (key) {
    case 'object-layout':
      return 'Layout'
    case 'unixfs':
      return 'Files'
    case 'canvas':
      return 'Canvas'
    case 'notes':
      return 'Notes'
    case 'chat':
      return 'Chat'
  }

  const segment = key.split('/').filter(Boolean).at(-1) ?? key
  return humanizeObjectKeySegment(segment)
}

export function buildSpaceObjectActionTargets(
  objects: readonly WorldContentsObject[],
  metadataById?: ObjectTypeMetadataById,
): SpaceObjectActionTarget[] {
  return objects
    .flatMap((object) => {
      if (
        isHiddenSpaceObject(object.objectKey, object.objectType, metadataById)
      ) {
        return []
      }
      const objectKey = object.objectKey ?? ''
      const objectType = object.objectType ?? ''
      return [
        {
          objectKey,
          objectType,
          label: getObjectDisplayName(objectKey),
          objectTypeLabel: getObjectTypeLabel(objectType, metadataById),
          objectTypeDescription: getObjectTypeDescription(
            objectType,
            metadataById,
          ),
        },
      ]
    })
    .toSorted((a, b) => a.objectKey.localeCompare(b.objectKey))
}

function humanizeObjectKeySegment(segment: string): string {
  const decoded = safeDecodeURIComponent(segment)
  if (decoded === segment && !/[-_\s%]/.test(segment)) return segment
  const words = decoded.replace(/[-_]+/g, ' ').replace(/\s+/g, ' ').trim()
  if (!words) return segment || 'Object'
  return words.replace(/\b\w/g, (char) => char.toUpperCase())
}

function safeDecodeURIComponent(value: string): string {
  try {
    return decodeURIComponent(value)
  } catch {
    return value
  }
}

function isHiddenObjectTypeMetadata(
  metadata: ObjectTypeMetadata | undefined,
): boolean {
  return (
    metadata?.visibility === ObjectTypeVisibility.HIDDEN ||
    metadata?.visibility === ObjectTypeVisibility.INTERNAL
  )
}

interface TreeMapEntry {
  object?: SpaceObjectActionTarget
  children: Map<string, TreeMapEntry>
}

// buildObjectTree converts a flat list of WorldContentsObject into a TreeNode hierarchy.
export function buildObjectTree(
  objects: WorldContentsObject[],
  metadataById?: ObjectTypeMetadataById,
): TreeNode<ObjectTreeNode>[] {
  const root: Map<string, TreeMapEntry> = new Map()

  for (const obj of buildSpaceObjectActionTargets(objects, metadataById)) {
    const key = obj.objectKey

    const segments = key.split('/')
    let current = root
    for (let i = 0; i < segments.length; i++) {
      const seg = segments[i]
      if (!current.has(seg)) {
        current.set(seg, { children: new Map() })
      }
      const entry = current.get(seg)!
      if (i === segments.length - 1) {
        entry.object = obj
      }
      current = entry.children
    }
  }

  return mapToTreeNodes(root, '', metadataById)
}

function mapToTreeNodes(
  entries: Map<string, TreeMapEntry>,
  prefix: string,
  metadataById?: ObjectTypeMetadataById,
): TreeNode<ObjectTreeNode>[] {
  const result: TreeNode<ObjectTreeNode>[] = []
  const sorted = Array.from(entries.entries()).toSorted((a, b) =>
    a[0].localeCompare(b[0]),
  )

  for (const [name, entry] of sorted) {
    const fullKey = prefix ? `${prefix}/${name}` : name
    const children = mapToTreeNodes(entry.children, fullKey, metadataById)
    const isVirtual = !entry.object
    const objectType = entry.object?.objectType ?? ''
    const objectTypeLabel = isVirtual
      ? ''
      : getObjectTypeLabel(objectType, metadataById)
    const objectTypeDescription = isVirtual
      ? ''
      : getObjectTypeDescription(objectType, metadataById)

    const node: TreeNode<ObjectTreeNode> = {
      id: fullKey,
      name: isVirtual
        ? humanizeObjectKeySegment(name)
        : getObjectDisplayName(fullKey),
      detail: objectTypeLabel,
      icon: isVirtual ? (
        <LuFolder className={iconSize} />
      ) : (
        getObjectTypeIcon(objectType, metadataById)
      ),
      data: {
        objectKey: isVirtual ? fullKey : (entry.object?.objectKey ?? fullKey),
        objectType,
        objectTypeLabel,
        objectTypeDescription,
        isVirtual,
      },
    }

    if (children.length > 0) {
      node.children = children
    }

    result.push(node)
  }

  return result
}
