import { useCallback, useMemo, useState } from 'react'
import { LuEllipsis } from 'react-icons/lu'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { useWorldQuery } from '@s4wave/web/hooks/useWorldQuery.js'
import type { TreeNode } from '@s4wave/web/ui/tree/TreeNode.js'

import {
  listObjectLevel,
  objectLevelPrefix,
  type ObjectLevel,
  type ObjectTreeNode,
  type ObjectTypeMetadataById,
} from './object-tree.js'

// objectLevelPageSize is how many entries a tree level reads at a time.
export const objectLevelPageSize = 200

// ObjectTree is the loaded part of a Space's object tree.
export interface ObjectTree {
  // nodes are the root nodes with the children of each expanded node, or null
  // while the root level loads.
  nodes: TreeNode<ObjectTreeNode>[] | null
  // loadMore reads one more page of the level under prefix.
  loadMore: (prefix: string) => void
}

// useObjectTree follows the root level of a World's object tree and the level
// under each node in expandedIds. A level with unread entries ends in a node
// that reads one more page of that level when activated.
export function useObjectTree(
  world: Resource<IWorldState>,
  expandedIds: ReadonlySet<string>,
  metadataById?: ObjectTypeMetadataById,
): ObjectTree {
  const [limits, setLimits] = useState<ReadonlyMap<string, number>>(
    () => new Map(),
  )
  const prefixes = useMemo(
    () => ['', ...Array.from(expandedIds, objectLevelPrefix)],
    [expandedIds],
  )

  const levels = useWorldQuery(
    world,
    async (state, signal) => {
      const entries = await Promise.all(
        prefixes.map(
          async (prefix) =>
            [
              prefix,
              await listObjectLevel(
                state,
                prefix,
                limits.get(prefix) ?? objectLevelPageSize,
                metadataById,
                signal,
              ),
            ] as const,
        ),
      )
      return new Map(entries)
    },
    [prefixes, limits, metadataById],
  ).value

  const loadMore = useCallback((prefix: string) => {
    setLimits((prev) =>
      new Map(prev).set(
        prefix,
        (prev.get(prefix) ?? objectLevelPageSize) + objectLevelPageSize,
      ),
    )
  }, [])

  const nodes = useMemo(
    () => (levels ? attachLevels(levels, '', loadMore) : null),
    [levels, loadMore],
  )

  return { nodes, loadMore }
}

// attachLevels returns the nodes of the level under prefix, nesting the loaded
// level under each node that has one.
function attachLevels(
  levels: ReadonlyMap<string, ObjectLevel>,
  prefix: string,
  loadMore: (prefix: string) => void,
): TreeNode<ObjectTreeNode>[] {
  const level = levels.get(prefix)
  if (!level) return []

  const nodes = level.nodes.map((node) => {
    const childPrefix = objectLevelPrefix(node.id)
    if (!node.hasChildren || !levels.has(childPrefix)) return node
    return { ...node, children: attachLevels(levels, childPrefix, loadMore) }
  })
  if (level.more) {
    nodes.push(moreNode(prefix, loadMore))
  }
  return nodes
}

// moreNode builds the node that reads the next page of a level.
function moreNode(
  prefix: string,
  loadMore: (prefix: string) => void,
): TreeNode<ObjectTreeNode> {
  return {
    id: `${prefix}…`,
    name: 'Show more',
    icon: <LuEllipsis className="h-3.5 w-3.5" />,
    icons: [
      {
        icon: <LuEllipsis />,
        tooltip: 'Show more objects',
        onClick: () => loadMore(prefix),
      },
    ],
    data: {
      objectKey: '',
      objectType: '',
      isVirtual: true,
      morePrefix: prefix,
    },
  }
}
