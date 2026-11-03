import { useMemo, useCallback } from 'react'

import { GraphLinkPill } from './GraphLinkPill.js'
import type {
  CanvasEdgeData,
  CanvasNodeData,
  EphemeralEdge,
  CanvasCallbacks,
} from './types.js'

import * as stylex from '@stylexjs/stylex'
import {
  colors as themeColors,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    zIndex: 0,
    pointerEvents: 'none',
    position: 'absolute',
    inset: 0,
    height: '100%',
    width: '100%',
    overflow: 'visible',
  },
  rule2: {
    fill: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule3: {
    fill: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 30%, transparent)`,
  },
  rule4: {
    stroke: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule5: {
    fill: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule6: {
    stroke: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 30%, transparent)`,
  },
  rule7: { pointerEvents: 'auto' },
})

// CanvasEdgeLayerProps are the props for CanvasEdgeLayer.
interface CanvasEdgeLayerProps {
  edges: CanvasEdgeData[]
  ephemeralEdges?: EphemeralEdge[]
  nodes: Map<string, CanvasNodeData>
  callbacks: CanvasCallbacks
}

// getNodeCenter returns the center coordinates of a node.
function getNodeCenter(node: CanvasNodeData): { cx: number; cy: number } {
  return {
    cx: node.x + node.width / 2,
    cy: node.y + node.height / 2,
  }
}

// bezierPath builds an SVG cubic bezier path from source to target centers.
function bezierPath(sx: number, sy: number, tx: number, ty: number): string {
  const dx = Math.abs(tx - sx) * 0.3
  return `M ${sx} ${sy} C ${sx + dx} ${sy}, ${tx - dx} ${ty}, ${tx} ${ty}`
}

// straightPath builds an SVG straight line path.
function straightPath(sx: number, sy: number, tx: number, ty: number): string {
  return `M ${sx} ${sy} L ${tx} ${ty}`
}

// manualEdgePathID returns an SVG-safe local id for manual edge label paths.
function manualEdgePathID(edgeID: string, index: number): string {
  const safe = edgeID.replace(/[^A-Za-z0-9_-]/g, '-')
  return `canvas-manual-edge-${index}-${safe || 'edge'}`
}

// CanvasEdgeLayer renders SVG edges between nodes.
export function CanvasEdgeLayer({
  edges,
  ephemeralEdges,
  nodes,
  callbacks,
}: CanvasEdgeLayerProps) {
  const manualEdges = useMemo(() => {
    const result: Array<{
      path: string
      pathID: string
      label?: string
      key: string
    }> = []
    edges.forEach((edge, index) => {
      const source = nodes.get(edge.sourceNodeId)
      const target = nodes.get(edge.targetNodeId)
      if (!source || !target) return

      const s = getNodeCenter(source)
      const t = getNodeCenter(target)
      const path =
        edge.style === 'straight'
          ? straightPath(s.cx, s.cy, t.cx, t.cy)
          : bezierPath(s.cx, s.cy, t.cx, t.cy)

      result.push({
        path,
        pathID: manualEdgePathID(edge.id, index),
        label: edge.label,
        key: edge.id,
      })
    })
    return result
  }, [edges, nodes])

  const ephemeral = useMemo(() => {
    if (!ephemeralEdges) return []
    const result: Array<{
      path: string
      linkedObjectLabel: string
      edge: EphemeralEdge
      key: string
      actionX: number
      actionY: number
      hasTarget: boolean
    }> = []

    for (const edge of ephemeralEdges) {
      const sourceNode = nodes.get(edge.sourceNodeId)
      if (!sourceNode) continue

      const targetNode = edge.targetNodeId ? nodes.get(edge.targetNodeId) : null

      if (targetNode) {
        const s = getNodeCenter(sourceNode)
        const t = getNodeCenter(targetNode)
        const path = bezierPath(s.cx, s.cy, t.cx, t.cy)
        result.push({
          path,
          linkedObjectLabel: edge.linkedObjectLabel,
          edge,
          key: edge.renderKey,
          actionX: (s.cx + t.cx) / 2,
          actionY: (s.cy + t.cy) / 2,
          hasTarget: true,
        })
        continue
      }

      if (edge.stubX === undefined || edge.stubY === undefined) continue

      const s = getNodeCenter(sourceNode)
      const path = bezierPath(s.cx, s.cy, edge.stubX, edge.stubY)
      result.push({
        path,
        linkedObjectLabel: edge.linkedObjectLabel,
        edge,
        key: edge.renderKey,
        actionX: edge.stubX,
        actionY: edge.stubY,
        hasTarget: false,
      })
    }
    return result
  }, [ephemeralEdges, nodes])

  const handlePin = useCallback(
    (objectKey: string, x: number, y: number) => {
      callbacks.onPinObject?.(objectKey, x, y)
    },
    [callbacks],
  )

  const handleFocus = useCallback(
    (objectKey: string, nodeId: string) => {
      callbacks.onFocusObject?.(objectKey, nodeId)
    },
    [callbacks],
  )

  const handleHide = useCallback(
    (edge: EphemeralEdge) => {
      callbacks.onHideGraphLink?.(edge)
    },
    [callbacks],
  )

  const handleDelete = useCallback(
    (edge: EphemeralEdge) => {
      callbacks.onDeleteGraphLink?.(edge)
    },
    [callbacks],
  )

  return (
    <svg sx={stylexStyles.rule1}>
      <defs>
        <marker
          id="canvas-arrowhead"
          markerWidth="10"
          markerHeight="7"
          refX="10"
          refY="3.5"
          orient="auto"
        >
          <polygon points="0 0, 10 3.5, 0 7" sx={stylexStyles.rule2} />
        </marker>
        <marker
          id="canvas-arrowhead-dashed"
          markerWidth="10"
          markerHeight="7"
          refX="10"
          refY="3.5"
          orient="auto"
        >
          <polygon points="0 0, 10 3.5, 0 7" sx={stylexStyles.rule3} />
        </marker>
      </defs>

      {manualEdges.map((edge) => (
        <g key={edge.key}>
          <path
            id={edge.pathID}
            d={edge.path}
            fill="none"
            sx={stylexStyles.rule4}
            strokeWidth={2}
            markerEnd="url(#canvas-arrowhead)"
          />
          {edge.label && (
            <text sx={stylexStyles.rule5} textAnchor="middle" dy={-8}>
              <textPath href={`#${edge.pathID}`} startOffset="50%">
                {edge.label}
              </textPath>
            </text>
          )}
        </g>
      ))}

      {ephemeral.map((edge) => (
        <g key={edge.key}>
          <path
            d={edge.path}
            fill="none"
            sx={stylexStyles.rule6}
            strokeWidth={1.5}
            strokeDasharray="6 4"
            markerEnd="url(#canvas-arrowhead-dashed)"
          />
          <foreignObject
            x={edge.actionX - 12}
            y={edge.actionY - 13}
            width={260}
            height={26}
            sx={stylexStyles.rule7}
          >
            <GraphLinkPill
              edge={edge.edge}
              loaded={edge.hasTarget}
              onPrimary={() => {
                if (edge.hasTarget && edge.edge.targetNodeId) {
                  handleFocus(edge.edge.linkedObjectKey, edge.edge.targetNodeId)
                  return
                }
                handlePin(edge.edge.linkedObjectKey, edge.actionX, edge.actionY)
              }}
              onHide={() => handleHide(edge.edge)}
              onDelete={() => handleDelete(edge.edge)}
            />
          </foreignObject>
        </g>
      ))}
    </svg>
  )
}
