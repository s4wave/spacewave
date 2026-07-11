import { useMemo } from 'react'

import type { CanvasNodeData } from './types.js'
import { decodeCanvasGeometry } from './geometry.js'

interface CanvasGeometryNodeProps {
  node: CanvasNodeData
}

// CanvasGeometryNode renders a drawing from its persisted shapeData.
export function CanvasGeometryNode({ node }: CanvasGeometryNodeProps) {
  const geometry = useMemo(
    () => decodeCanvasGeometry(node.shapeData),
    [node.shapeData],
  )
  if (!geometry) return null

  const [start, ...points] = geometry.points
  const path = `M ${start.x} ${start.y} ${points
    .map((point) => `L ${point.x} ${point.y}`)
    .join(' ')}`
  return (
    <svg
      className="h-full w-full overflow-visible"
      viewBox={`0 0 ${node.width} ${node.height}`}
      preserveAspectRatio="none"
      data-canvas-geometry="pen"
      data-canvas-color={geometry.color}
    >
      <path
        d={path}
        fill="none"
        stroke={geometry.color}
        strokeWidth={2}
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  )
}
