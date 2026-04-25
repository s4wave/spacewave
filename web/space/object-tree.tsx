import React from 'react'
import {
  LuBox,
  LuFile,
  LuFolder,
  LuGitBranch,
  LuLayoutGrid,
  LuPaintbrush,
} from 'react-icons/lu'
import {
  SPACE_SETTINGS_BLOCK_TYPE,
  SPACE_SETTINGS_OBJECT_KEY,
} from '@s4wave/core/space/world/world.js'
import type { TreeNode } from '@s4wave/web/ui/tree/TreeNode.js'
import type { WorldContentsObject } from '@s4wave/core/space/world/world.pb.js'

// ObjectTreeNode holds metadata for a node in the object tree.
export interface ObjectTreeNode {
  objectKey: string
  objectType: string
  isVirtual: boolean
}

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
): boolean {
  if ((objectKey ?? '') === SPACE_SETTINGS_OBJECT_KEY) return true
  return HIDDEN_OBJECT_TYPES.has(objectType ?? '')
}

const iconSize = 'h-3.5 w-3.5'

// getObjectTypeIcon returns the icon element for a given object type ID.
export function getObjectTypeIcon(typeId: string): React.ReactNode {
  switch (typeId) {
    case 'alpha/object-layout':
      return <LuLayoutGrid className={iconSize} />
    case 'unixfs/fs-node':
      return <LuFile className={iconSize} />
    case 'git/repo':
    case 'git/worktree':
      return <LuGitBranch className={iconSize} />
    case 'canvas':
      return <LuPaintbrush className={iconSize} />
    default:
      return <LuBox className={iconSize} />
  }
}

// getObjectTypeLabel returns a human-readable label for a given object type ID.
export function getObjectTypeLabel(typeId: string): string {
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
    default:
      return typeId || 'Object'
  }
}

interface TreeMapEntry {
  object?: WorldContentsObject
  children: Map<string, TreeMapEntry>
}

// buildObjectTree converts a flat list of WorldContentsObject into a TreeNode hierarchy.
export function buildObjectTree(
  objects: WorldContentsObject[],
): TreeNode<ObjectTreeNode>[] {
  const root: Map<string, TreeMapEntry> = new Map()

  for (const obj of objects) {
    const key = obj.objectKey ?? ''
    const type = obj.objectType ?? ''
    if (isHiddenSpaceObject(key, type)) continue

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

  return mapToTreeNodes(root, '')
}

function mapToTreeNodes(
  entries: Map<string, TreeMapEntry>,
  prefix: string,
): TreeNode<ObjectTreeNode>[] {
  const result: TreeNode<ObjectTreeNode>[] = []
  const sorted = [...entries.entries()].sort((a, b) => a[0].localeCompare(b[0]))

  for (const [name, entry] of sorted) {
    const fullKey = prefix ? `${prefix}/${name}` : name
    const children = mapToTreeNodes(entry.children, fullKey)
    const isVirtual = !entry.object
    const objectType = entry.object?.objectType ?? ''

    const node: TreeNode<ObjectTreeNode> = {
      id: fullKey,
      name,
      icon:
        isVirtual ?
          <LuFolder className={iconSize} />
        : getObjectTypeIcon(objectType),
      data: {
        objectKey: isVirtual ? fullKey : (entry.object?.objectKey ?? fullKey),
        objectType,
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
