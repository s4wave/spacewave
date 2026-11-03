import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import { useMemo, useCallback, useRef } from 'react'

import type { CanvasNodeData, Viewport } from './types.js'
import type { ContainerSize } from './useVisibleNodes.js'

import * as stylex from '@stylexjs/stylex'
import {
  colors as themeColors,
  utilityState as themeUtilityState,
} from '../../web/style/tokens.stylex.js'

import {
  layout as themeLayout,
  radii as themeRadii,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    width: 'var(--minimap-width)',
    height: 'var(--minimap-height)',
    position: 'absolute',
    right: `calc(${themeLayout['--spacing']} * 4)`,
    bottom: `calc(${themeLayout['--spacing']} * 4)`,
    overflow: 'hidden',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    left: 'var(--minimap-left)',
    top: 'var(--minimap-top)',
    width: 'var(--minimap-rect-width)',
    height: 'var(--minimap-rect-height)',
    position: 'absolute',
    borderRadius: 'var(--radius-xs)',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 25%, transparent)`,
  },
  rule2: {
    left: 'var(--minimap-left)',
    top: 'var(--minimap-top)',
    width: 'var(--minimap-rect-width)',
    height: 'var(--minimap-rect-height)',
    position: 'absolute',
    borderRadius: 'var(--radius-sm)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
  },
})

// DEFAULT_MINIMAP_WIDTH is the default width of the minimap in pixels.
export const DEFAULT_MINIMAP_WIDTH = 200

// DEFAULT_MINIMAP_HEIGHT is the default height of the minimap in pixels.
export const DEFAULT_MINIMAP_HEIGHT = 150

// MINIMAP_PADDING is the padding around content in the minimap.
const MINIMAP_PADDING = 10

// CanvasMinimapProps are the props for CanvasMinimap.
interface CanvasMinimapProps {
  className?: never
  nodes: Map<string, CanvasNodeData>
  viewport: Viewport
  containerSize: ContainerSize
  onViewportChange: (v: Viewport) => void
  width?: number
  height?: number
  sx?: ComponentStyles
}

// CanvasMinimap renders a small overview of all nodes with a viewport indicator.
export function CanvasMinimap({
  nodes,
  viewport,
  containerSize,
  onViewportChange,
  width: MINIMAP_WIDTH = DEFAULT_MINIMAP_WIDTH,
  height: MINIMAP_HEIGHT = DEFAULT_MINIMAP_HEIGHT,
  sx,
}: Omit<CanvasMinimapProps, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
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
      sx={[stylexStyles2.rule1, sx]}
      style={{
        '--minimap-width': `${MINIMAP_WIDTH}px`,
        '--minimap-height': `${MINIMAP_HEIGHT}px`,
      }}
      role="button"
      tabIndex={0}
      aria-label="Center canvas minimap"
      onClick={handleMinimapCenter}
      onKeyDown={handleMinimapKeyDown}
    >
      {nodeRects.map((r) => (
        <div
          key={r.key}
          sx={stylexStyles.rule1}
          style={{
            '--minimap-left': `${r.x}px`,
            '--minimap-top': `${r.y}px`,
            '--minimap-rect-width': `${r.width}px`,
            '--minimap-rect-height': `${r.height}px`,
          }}
        />
      ))}
      <div
        sx={stylexStyles.rule2}
        style={{
          '--minimap-left': `${vpRect.x}px`,
          '--minimap-top': `${vpRect.y}px`,
          '--minimap-rect-width': `${vpRect.width}px`,
          '--minimap-rect-height': `${vpRect.height}px`,
        }}
      />
    </div>
  )
}
