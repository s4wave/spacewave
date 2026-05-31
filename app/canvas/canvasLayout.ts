import type { CanvasLayoutMetadataData, CanvasNodeData } from './types.js'

const LAYOUT_ORIGIN_X = 80
const LAYOUT_ORIGIN_Y = 80
const RANK_GAP = 100
const LANE_GAP = 180
const NODE_GAP = 24
const GROUP_GAP = 32

const SEMANTIC_LANE_ORDER = [
  'source',
  'filesystem',
  'audit',
  'execution',
  'gate',
  'repair',
  'proof',
  'publish',
]

interface LayoutNode {
  id: string
  node: CanvasNodeData
  metadata: CanvasLayoutMetadataData
  rank: number
  lane: string
  group: string
  stableNodeId: string
}

function laneOrder(lane: string): number {
  const idx = SEMANTIC_LANE_ORDER.indexOf(lane)
  return idx === -1 ? SEMANTIC_LANE_ORDER.length : idx
}

function compareText(a: string, b: string): number {
  if (a < b) return -1
  if (a > b) return 1
  return 0
}

function compareLayoutNode(a: LayoutNode, b: LayoutNode): number {
  return (
    a.rank - b.rank ||
    compareText(a.group, b.group) ||
    compareText(a.stableNodeId, b.stableNodeId) ||
    compareText(a.id, b.id)
  )
}

function buildRankXPositions(layoutNodes: LayoutNode[]): Map<number, number> {
  const rankWidths = new Map<number, number>()
  for (const entry of layoutNodes) {
    rankWidths.set(
      entry.rank,
      Math.max(rankWidths.get(entry.rank) ?? 0, entry.node.width),
    )
  }

  const rankXPositions = new Map<number, number>()
  let x = LAYOUT_ORIGIN_X
  for (const rank of [...rankWidths.keys()].toSorted((a, b) => a - b)) {
    rankXPositions.set(rank, x)
    x += (rankWidths.get(rank) ?? 0) + RANK_GAP
  }
  return rankXPositions
}

// organizeCanvasNodes returns changed node positions for metadata-backed nodes.
export function organizeCanvasNodes(
  nodes: Map<string, CanvasNodeData>,
  layoutMetadata: Map<string, CanvasLayoutMetadataData>,
): Map<string, CanvasNodeData> {
  const layoutNodes: LayoutNode[] = []
  for (const [id, metadata] of layoutMetadata) {
    const node = nodes.get(id)
    if (!node) continue
    layoutNodes.push({
      id,
      node,
      metadata,
      rank: metadata.rank ?? 0,
      lane: metadata.lane ?? '',
      group: metadata.group ?? '',
      stableNodeId: metadata.stableNodeId ?? '',
    })
  }
  if (layoutNodes.length === 0) {
    return new Map()
  }

  const rankXPositions = buildRankXPositions(layoutNodes)

  const lanes = new Map<string, LayoutNode[]>()
  for (const entry of layoutNodes) {
    const lane = entry.lane
    const existing = lanes.get(lane)
    if (existing) {
      existing.push(entry)
      continue
    }
    lanes.set(lane, [entry])
  }

  const changed = new Map<string, CanvasNodeData>()
  let laneY = LAYOUT_ORIGIN_Y
  const sortedLanes = [...lanes.entries()].toSorted(([a], [b]) => {
    return laneOrder(a) - laneOrder(b) || compareText(a, b)
  })

  for (const [, laneNodes] of sortedLanes) {
    laneNodes.sort(compareLayoutNode)
    let y = laneY
    let previousGroup: string | null = null
    let laneBottom = laneY

    for (const entry of laneNodes) {
      if (previousGroup !== null && previousGroup !== entry.group) {
        y += GROUP_GAP
      }
      const x = rankXPositions.get(entry.rank) ?? LAYOUT_ORIGIN_X
      if (entry.node.x !== x || entry.node.y !== y) {
        changed.set(entry.id, { ...entry.node, x, y })
      }
      laneBottom = Math.max(laneBottom, y + entry.node.height)
      y += entry.node.height + NODE_GAP
      previousGroup = entry.group
    }

    laneY = laneBottom + LANE_GAP
  }

  return changed
}
