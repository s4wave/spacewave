import { useMemo, type ReactNode } from 'react'

import type { CanvasNodeData } from './types.js'
import { decodeCanvasGeometry } from './geometry.js'

interface CanvasGeometryNodeProps {
  node: CanvasNodeData
}

// CanvasGeometryNode renders drawing and primitive-shape nodes from shapeData.
export function CanvasGeometryNode({ node }: CanvasGeometryNodeProps) {
  const geometry = useMemo(
    () => decodeCanvasGeometry(node.shapeData),
    [node.shapeData],
  )
  if (!geometry) return null

  const [start, end] = geometry.points
  const markerId = `canvas-arrow-${node.id.replace(/[^A-Za-z0-9_-]/g, '-')}`
  const shared = {
    fill: 'none',
    stroke: geometry.color,
    strokeWidth: 2,
    strokeLinecap: 'round' as const,
    strokeLinejoin: 'round' as const,
  }

  let content: ReactNode
  switch (geometry.kind) {
    case 'pen': {
      const d = `M ${start.x} ${start.y} ${geometry.points
        .slice(1)
        .map((point) => `L ${point.x} ${point.y}`)
        .join(' ')}`
      content = <path d={d} {...shared} />
      break
    }
    case 'line':
      content = (
        <line x1={start.x} y1={start.y} x2={end.x} y2={end.y} {...shared} />
      )
      break
    case 'arrow':
      content = (
        <>
          <defs>
            <marker
              id={markerId}
              viewBox="0 0 10 10"
              refX="9"
              refY="5"
              markerWidth="5"
              markerHeight="5"
              orient="auto-start-reverse"
            >
              <path d="M 0 0 L 10 5 L 0 10 z" fill={geometry.color} />
            </marker>
          </defs>
          <line
            x1={start.x}
            y1={start.y}
            x2={end.x}
            y2={end.y}
            markerEnd={`url(#${markerId})`}
            {...shared}
          />
        </>
      )
      break
    case 'rectangle':
      content = (
        <rect
          x={Math.min(start.x, end.x)}
          y={Math.min(start.y, end.y)}
          width={Math.abs(end.x - start.x)}
          height={Math.abs(end.y - start.y)}
          {...shared}
        />
      )
      break
    case 'ellipse': {
      const width = Math.abs(end.x - start.x)
      const height = Math.abs(end.y - start.y)
      content = (
        <ellipse
          cx={Math.min(start.x, end.x) + width / 2}
          cy={Math.min(start.y, end.y) + height / 2}
          rx={width / 2}
          ry={height / 2}
          {...shared}
        />
      )
      break
    }
  }

  return (
    <svg
      className="h-full w-full overflow-visible"
      viewBox={`0 0 ${node.width} ${node.height}`}
      preserveAspectRatio="none"
      data-canvas-geometry={geometry.kind}
      data-canvas-color={geometry.color}
    >
      {content}
    </svg>
  )
}
