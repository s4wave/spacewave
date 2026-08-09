import { useMemo, useCallback, useRef } from 'react'

import { cn } from '@s4wave/web/style/utils.js'

import type { CanvasNodeData, Viewport } from './types.js'
import type { ContainerSize } from './useVisibleNodes.js'

export const DEFAULT_MINIMAP_WIDTH = 200

export const DEFAULT_MINIMAP_HEIGHT = 150

const MINIMAP_PADDING = 10

interface CanvasMinimapProps {
  nodes: Map<string, CanvasNodeData>
  viewport: Viewport
  containerSize: ContainerSize
  onViewportChange: (v: Viewport) => void
  width?: number
  height?: number
  className?: string
}

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

  const contentWidth = bounds.maxX - bounds.minX
  const contentHeight = bounds.maxY - bounds.minY

  const scaleX = (MINIMAP_WIDTH - MINIMAP_PADDING * 2) / (contentWidth || 1)
  const scaleY = (MINIMAP_HEIGHT - MINIMAP_PADDING * 2) / (contentHeight || 1)
  const minimapScale = Math.min(scaleX, scaleY)

  const toMinimap = useCallback(
    (cx: number, cy: number) => ({
      mx: (cx - bounds.minX) * minimapScale + MINIMAP_PADDING,
      my: (cy - bounds.minY) * minimapScale + MINIMAP_PADDING,
    }),
    [bounds.minX, bounds.minY, minimapScale],
  )

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

  const centerMinimapAt = useCallback(
    (mx: number, my: number) => {
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

  const handleMinimapCenter = useCallback(
    (e: React.MouseEvent) => {
      const rect = minimapRef.current?.getBoundingClientRect()
      if (!rect) return
      centerMinimapAt(e.clientX - rect.left, e.clientY - rect.top)
    },
    [centerMinimapAt],
  )

  const handleMinimapKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key !== 'Enter' && e.key !== ' ') return
      e.preventDefault()
      centerMinimapAt(MINIMAP_WIDTH / 2, MINIMAP_HEIGHT / 2)
    },
    [centerMinimapAt, MINIMAP_WIDTH, MINIMAP_HEIGHT],
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
      role="button"
      tabIndex={0}
      aria-label="Center canvas minimap"
      onClick={handleMinimapCenter}
      onKeyDown={handleMinimapKeyDown}
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
