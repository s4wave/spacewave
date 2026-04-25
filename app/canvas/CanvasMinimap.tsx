import { useMemo, useCallback, useRef } from 'react'

import { cn } from '@s4wave/web/style/utils.js'

import type { CanvasNodeData, Viewport } from './types.js'
import type { ContainerSize } from './useVisibleNodes.js'

// DEFAULT_MINIMAP_WIDTH is the default width of the minimap in pixels.
export const DEFAULT_MINIMAP_WIDTH = 200

// DEFAULT_MINIMAP_HEIGHT is the default height of the minimap in pixels.
export const DEFAULT_MINIMAP_HEIGHT = 150

// MINIMAP_PADDING is the padding around content in the minimap.
const MINIMAP_PADDING = 10

// CanvasMinimapProps are the props for CanvasMinimap.
interface CanvasMinimapProps {
  nodes: Map<string, CanvasNodeData>
  viewport: Viewport
  containerSize: ContainerSize
  onViewportChange: (v: Viewport) => void
  width?: number
  height?: number
  className?: string
}

// CanvasMinimap renders a small overview of all nodes with a viewport indicator.
export function CanvasMinimap({
  nodes,
  viewport,
  containerSize,
  onViewportChange,
  width: MINIMAP_WIDTH = DEFAULT_MINIMAP_WIDTH,
  height: MINIMAP_HEIGHT = DEFAULT_MINIMAP_HEIGHT,
  className,
}: CanvasMinimapProps) {
  const minimapRef = useRef<HTMLDivElement | null>(null)

  // Compute the bounding box of all nodes.
  const bounds = useMemo(() => {
    if (nodes.size === 0) {
      return { minX: 0, minY: 0, maxX: 100, maxY: 100 }
    }

    let minX = Infinity
    let minY = Infinity
    let maxX = -Infinity
    let maxY = -Infinity

    for (const node of nodes.values()) {
      minX = Math.min(minX, node.x)
      minY = Math.min(minY, node.y)
      maxX = Math.max(maxX, node.x + node.width)
      maxY = Math.max(maxY, node.y + node.height)
    }

    // Include the current viewport area in the bounds.
    const vpLeft = -viewport.x / viewport.scale
    const vpTop = -viewport.y / viewport.scale
    const vpRight = vpLeft + containerSize.width / viewport.scale
    const vpBottom = vpTop + containerSize.height / viewport.scale

    minX = Math.min(minX, vpLeft)
    minY = Math.min(minY, vpTop)
    maxX = Math.max(maxX, vpRight)
    maxY = Math.max(maxY, vpBottom)

    return { minX, minY, maxX, maxY }
  }, [nodes, viewport, containerSize])

  // Scale from canvas space to minimap space.
  const contentWidth = bounds.maxX - bounds.minX
  const contentHeight = bounds.maxY - bounds.minY

  const scaleX = (MINIMAP_WIDTH - MINIMAP_PADDING * 2) / (contentWidth || 1)
  const scaleY = (MINIMAP_HEIGHT - MINIMAP_PADDING * 2) / (contentHeight || 1)
  const minimapScale = Math.min(scaleX, scaleY)

  // Convert canvas coordinates to minimap coordinates.
  const toMinimap = useCallback(
    (cx: number, cy: number) => ({
      mx: (cx - bounds.minX) * minimapScale + MINIMAP_PADDING,
      my: (cy - bounds.minY) * minimapScale + MINIMAP_PADDING,
    }),
    [bounds.minX, bounds.minY, minimapScale],
  )

  // Viewport rectangle in minimap space.
  const vpRect = useMemo(() => {
    const vpLeft = -viewport.x / viewport.scale
    const vpTop = -viewport.y / viewport.scale
    const vpWidth = containerSize.width / viewport.scale
    const vpHeight = containerSize.height / viewport.scale

    const { mx, my } = toMinimap(vpLeft, vpTop)
    return {
      x: mx,
      y: my,
      width: vpWidth * minimapScale,
      height: vpHeight * minimapScale,
    }
  }, [viewport, containerSize, toMinimap, minimapScale])

  const handleClick = useCallback(
    (e: React.MouseEvent) => {
      const rect = minimapRef.current?.getBoundingClientRect()
      if (!rect) return

      const mx = e.clientX - rect.left
      const my = e.clientY - rect.top

      // Convert minimap click to canvas center.
      const cx = (mx - MINIMAP_PADDING) / minimapScale + bounds.minX
      const cy = (my - MINIMAP_PADDING) / minimapScale + bounds.minY

      onViewportChange({
        x: -cx * viewport.scale + containerSize.width / 2,
        y: -cy * viewport.scale + containerSize.height / 2,
        scale: viewport.scale,
      })
    },
    [minimapScale, bounds, viewport.scale, containerSize, onViewportChange],
  )

  const nodeRects = useMemo(() => {
    const rects: Array<{
      key: string
      x: number
      y: number
      width: number
      height: number
    }> = []
    for (const [id, node] of nodes) {
      const { mx, my } = toMinimap(node.x, node.y)
      rects.push({
        key: id,
        x: mx,
        y: my,
        width: Math.max(2, node.width * minimapScale),
        height: Math.max(2, node.height * minimapScale),
      })
    }
    return rects
  }, [nodes, toMinimap, minimapScale])

  return (
    <div
      ref={minimapRef}
      className={cn(
        'bg-background-card/30 border-foreground/6 absolute right-4 bottom-4 overflow-hidden rounded-lg border backdrop-blur-sm',
        className,
      )}
      style={{ width: MINIMAP_WIDTH, height: MINIMAP_HEIGHT }}
      onClick={handleClick}
    >
      {nodeRects.map((r) => (
        <div
          key={r.key}
          className="bg-foreground-alt/25 absolute rounded-[1px]"
          style={{
            left: r.x,
            top: r.y,
            width: r.width,
            height: r.height,
          }}
        />
      ))}
      <div
        className="border-brand/30 bg-brand/5 absolute rounded-sm border"
        style={{
          left: vpRect.x,
          top: vpRect.y,
          width: vpRect.width,
          height: vpRect.height,
        }}
      />
    </div>
  )
}
