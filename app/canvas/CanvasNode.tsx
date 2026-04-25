import { useCallback, useMemo, useRef, useState, useEffect, memo } from 'react'
import { useDrag } from '@use-gesture/react'

import { cn } from '@s4wave/web/style/utils.js'

// MIN_NODE_WIDTH is the minimum width a node can be resized to.
const MIN_NODE_WIDTH = 80

// MIN_NODE_HEIGHT is the minimum height a node can be resized to.
const MIN_NODE_HEIGHT = 60

import type { CanvasNodeData, CanvasCallbacks } from './types.js'
import type { SelectionFocus } from './useCanvasSelection.js'
import {
  SEMANTIC_ZOOM_OUTLINE,
  SIZE_THRESHOLD_WIDTH,
  SIZE_THRESHOLD_HEIGHT,
  UNMOUNT_DEBOUNCE_MS,
} from './types.js'
import { CanvasTextNode } from './CanvasTextNode.js'

// ResizeDirection indicates which corner is being dragged for resize.
type ResizeDirection = 'nw' | 'ne' | 'sw' | 'se'

// CanvasNodeProps are the props for CanvasNode.
interface CanvasNodeProps {
  node: CanvasNodeData
  scale: number
  selected: boolean
  focus: SelectionFocus
  visible: boolean
  callbacks: CanvasCallbacks
  onSelectWithFocus: (id: string, shift: boolean, focus: SelectionFocus) => void
  onMove: (id: string, dx: number, dy: number) => void
  onMoveEnd: () => void
  onResize?: (id: string, node: CanvasNodeData) => void
}

// ResizeHandleProps are the props for ResizeHandle.
interface ResizeHandleProps {
  direction: ResizeDirection
  scale: number
  onResizeDelta: (
    dir: ResizeDirection,
    dx: number,
    dy: number,
    last: boolean,
  ) => void
}

// parseShapePoint validates a decoded drawing point object.
function parseShapePoint(value: unknown): { x: number; y: number } | null {
  if (typeof value !== 'object' || value === null) {
    return null
  }
  const point = value as { x?: unknown; y?: unknown }
  if (typeof point.x !== 'number' || typeof point.y !== 'number') {
    return null
  }
  return { x: point.x, y: point.y }
}

// DragEdgeHandle renders a thin invisible border strip that can initiate node drag.
function DragEdgeHandle({ className }: { className: string }) {
  return (
    <div
      aria-hidden="true"
      data-drag-handle=""
      className={cn('absolute z-10 bg-transparent', className)}
    />
  )
}

// ResizeHandle renders an interactive corner resize handle.
function ResizeHandle({ direction, scale, onResizeDelta }: ResizeHandleProps) {
  const bind = useDrag(
    ({ delta: [dx, dy], last, event }) => {
      event?.stopPropagation()
      onResizeDelta(direction, dx / scale, dy / scale, last)
    },
    { filterTaps: true, pointer: { touch: true } },
  )

  const posClass = {
    nw: '-left-1.5 -top-1.5 cursor-nwse-resize',
    ne: '-right-1.5 -top-1.5 cursor-nesw-resize',
    se: '-bottom-1.5 -right-1.5 cursor-nwse-resize',
    sw: '-bottom-1.5 -left-1.5 cursor-nesw-resize',
  }[direction]

  const handlers = bind()
  const origPointerDown = handlers.onPointerDown
  handlers.onPointerDown = (e: React.PointerEvent) => {
    e.stopPropagation()
    origPointerDown?.(e)
  }

  return (
    <div
      {...handlers}
      className={cn(
        'border-brand/60 bg-background absolute h-2.5 w-2.5 rounded-full border',
        posClass,
      )}
      style={{ touchAction: 'none' }}
    />
  )
}

// CanvasNode renders a single positioned node on the canvas.
export const CanvasNode = memo(function CanvasNode({
  node,
  scale,
  selected,
  focus,
  visible,
  callbacks,
  onSelectWithFocus,
  onMove,
  onMoveEnd,
  onResize,
}: CanvasNodeProps) {
  const [mounted, setMounted] = useState(visible)
  const unmountTimer = useRef<ReturnType<typeof setTimeout> | null>(null)

  // Debounced mount/unmount for virtualization.
  useEffect(() => {
    if (visible) {
      if (unmountTimer.current) {
        clearTimeout(unmountTimer.current)
        unmountTimer.current = null
      }
    } else {
      unmountTimer.current = setTimeout(() => {
        setMounted(false)
      }, UNMOUNT_DEBOUNCE_MS)
    }
    return () => {
      if (unmountTimer.current) {
        clearTimeout(unmountTimer.current)
      }
    }
  }, [visible])

  const hasInteractiveContent =
    node.type === 'world_object' || node.type === 'text'

  const bind = useDrag(
    ({ delta: [dx, dy], first, last, event }) => {
      if (first) {
        const shiftKey =
          event instanceof MouseEvent || event instanceof PointerEvent ?
            event.shiftKey
          : false
        onSelectWithFocus(node.id, shiftKey, 'border')
      }
      if (dx !== 0 || dy !== 0) {
        onMove(node.id, dx / scale, dy / scale)
      }
      if (last) {
        onMoveEnd()
      }
    },
    { filterTaps: true, pointer: { touch: true } },
  )

  // Wrap bind() handlers for nodes with interactive content. Two things:
  // 1. Skip the gesture on pointerdown inside [data-interactive-content] so
  //    @use-gesture doesn't capture the pointer and steal events.
  // 2. Remove onClickCapture which @use-gesture adds for filterTaps. That
  //    capture-phase handler fires parent-to-child and blocks clicks from
  //    reaching embedded content (e.g. UnixFS browser rows).
  // Selection is NOT triggered here; handleClick handles it after children
  // have processed the click event.
  const wrappedBind = useCallback(() => {
    const handlers = bind()
    if (!hasInteractiveContent) return handlers
    // Remove the capture-phase click handler that @use-gesture adds for
    // filterTaps. It fires before children and would block their onClick.
    delete (handlers as Record<string, unknown>).onClickCapture
    const origPointerDown = handlers.onPointerDown
    handlers.onPointerDown = (e: React.PointerEvent) => {
      const target = e.target
      if (target instanceof Element) {
        // Drag handles always initiate drag, even inside interactive content,
        // unless the target is an interactive element (button, input, etc.).
        const dragHandle = target.closest('[data-drag-handle]')
        if (
          dragHandle &&
          !target.closest('button, input, select, textarea, a, [role="button"]')
        ) {
          origPointerDown?.(e)
          return
        }
        // Don't start drag inside interactive content.
        if (target.closest('[data-interactive-content]')) {
          return
        }
      }
      origPointerDown?.(e)
    }
    return handlers
  }, [bind, hasInteractiveContent])

  const handleClick = useCallback(
    (e: React.MouseEvent) => {
      e.stopPropagation()
      const target = e.target
      const isContent =
        hasInteractiveContent &&
        target instanceof Element &&
        !!target.closest('[data-interactive-content]') &&
        !target.closest('[data-drag-handle]')
      onSelectWithFocus(node.id, e.shiftKey, isContent ? 'content' : 'border')
    },
    [node.id, hasInteractiveContent, onSelectWithFocus],
  )

  const isOutlineOnly = scale < SEMANTIC_ZOOM_OUTLINE
  const isCompact =
    node.width < SIZE_THRESHOLD_WIDTH || node.height < SIZE_THRESHOLD_HEIGHT
  const isContentFocused =
    selected && focus === 'content' && hasInteractiveContent
  const showsBorderDragHandles = node.type === 'world_object' && !isOutlineOnly

  // When zoomed past 100%, counter-scale the content so it stays at native
  // pixel size. The node border grows from the canvas zoom, giving the
  // content more room to render at 1:1 instead of scaling up.
  const internalScale = scale > 1 ? 1 / scale : 1

  const handleTextChange = useCallback(
    (content: string) => {
      callbacks.onNodesChange?.(
        new Map([[node.id, { ...node, textContent: content }]]),
      )
    },
    [node, callbacks],
  )

  // Local resize state accumulates deltas during drag.
  const resizeRef = useRef<{
    dx: number
    dy: number
    dw: number
    dh: number
  } | null>(null)
  const [resizeOverride, setResizeOverride] = useState<{
    x: number
    y: number
    w: number
    h: number
  } | null>(null)

  const handleResizeDelta = useCallback(
    (dir: ResizeDirection, dx: number, dy: number, last: boolean) => {
      const prev = resizeRef.current ?? { dx: 0, dy: 0, dw: 0, dh: 0 }
      let ddx = 0,
        ddy = 0,
        ddw = 0,
        ddh = 0
      if (dir === 'se') {
        ddw = dx
        ddh = dy
      } else if (dir === 'sw') {
        ddx = dx
        ddw = -dx
        ddh = dy
      } else if (dir === 'ne') {
        ddw = dx
        ddy = dy
        ddh = -dy
      } else if (dir === 'nw') {
        ddx = dx
        ddy = dy
        ddw = -dx
        ddh = -dy
      }

      const next = {
        dx: prev.dx + ddx,
        dy: prev.dy + ddy,
        dw: prev.dw + ddw,
        dh: prev.dh + ddh,
      }

      const newW = Math.max(MIN_NODE_WIDTH, node.width + next.dw)
      const newH = Math.max(MIN_NODE_HEIGHT, node.height + next.dh)
      const clampedDw = newW - node.width
      const clampedDh = newH - node.height

      resizeRef.current = next

      if (last) {
        resizeRef.current = null
        setResizeOverride(null)
        if (
          onResize &&
          (clampedDw !== 0 || clampedDh !== 0 || next.dx !== 0 || next.dy !== 0)
        ) {
          onResize(node.id, {
            ...node,
            x: node.x + next.dx,
            y: node.y + next.dy,
            width: newW,
            height: newH,
          })
        }
      } else {
        setResizeOverride({
          x: node.x + next.dx,
          y: node.y + next.dy,
          w: newW,
          h: newH,
        })
      }
    },
    [node, onResize],
  )

  const style = useMemo(
    () => ({
      position: 'absolute' as const,
      left: resizeOverride?.x ?? node.x,
      top: resizeOverride?.y ?? node.y,
      width: resizeOverride?.w ?? node.width,
      height: resizeOverride?.h ?? node.height,
      zIndex: node.zIndex,
      touchAction: 'none' as const,
    }),
    [node.x, node.y, node.width, node.height, node.zIndex, resizeOverride],
  )

  if (!visible && !mounted) return null

  // labelText is the short label shown in outline-only mode.
  const labelText = node.objectKey ?? node.type

  const content = (() => {
    if (isOutlineOnly) {
      return (
        <div className="flex h-full items-center justify-center overflow-hidden p-1">
          <span className="text-foreground-alt/60 truncate text-center text-xs font-medium">
            {labelText}
          </span>
        </div>
      )
    }

    if (node.type === 'text') {
      return (
        <CanvasTextNode
          content={node.textContent ?? ''}
          onChange={handleTextChange}
        />
      )
    }

    if (node.type === 'drawing' && node.shapeData) {
      const points: Array<{ x: number; y: number }> = (() => {
        try {
          const decoded: unknown = JSON.parse(
            new TextDecoder().decode(node.shapeData),
          )
          if (!Array.isArray(decoded)) {
            return []
          }
          return decoded.flatMap((point) => {
            const nextPoint = parseShapePoint(point)
            return nextPoint ? [nextPoint] : []
          })
        } catch {
          return []
        }
      })()
      if (points.length >= 2) {
        const d = `M ${points[0].x} ${points[0].y} ${points
          .slice(1)
          .map((p) => `L ${p.x} ${p.y}`)
          .join(' ')}`
        return (
          <svg
            className="h-full w-full"
            viewBox={`0 0 ${node.width} ${node.height}`}
            preserveAspectRatio="none"
          >
            <path
              d={d}
              fill="none"
              stroke="currentColor"
              strokeWidth={2}
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </svg>
        )
      }
    }

    if (node.type === 'world_object' && callbacks.renderNodeContent) {
      return callbacks.renderNodeContent(node)
    }

    if (isCompact) {
      return (
        <div className="text-foreground-alt/40 flex h-full items-center justify-center text-xs">
          {node.type}
        </div>
      )
    }

    return null
  })()

  return (
    <div
      data-canvas-node={node.id}
      {...wrappedBind()}
      onClick={handleClick}
      style={style}
      className={cn(
        'text-card-foreground border-foreground/6 pointer-events-auto box-border cursor-grab rounded-lg border transition-shadow duration-150 select-none',
        isOutlineOnly ? 'bg-background-card/50'
        : node.type === 'world_object' ?
          'bg-background-card/30 shadow-sm backdrop-blur-sm'
        : node.type === 'text' ? 'bg-background-card/20'
        : null,
        selected && 'shadow-md',
        !isOutlineOnly &&
          (node.type === 'drawing' || node.type === 'text') &&
          !selected &&
          'border-transparent bg-transparent shadow-none',
        !isOutlineOnly &&
          node.type === 'drawing' &&
          !selected &&
          'pointer-events-none',
      )}
    >
      {/* Invisible hit area extending outward from the border for easier clicking.
          z-[-1] keeps it behind node content so it doesn't steal clicks. */}
      <div className="pointer-events-auto absolute -inset-1.5 -z-1 rounded-lg" />
      {showsBorderDragHandles && (
        <>
          <DragEdgeHandle className="top-0 right-2 left-2 h-1.5 rounded-t-lg" />
          <DragEdgeHandle className="right-2 bottom-0 left-2 h-1.5 rounded-b-lg" />
          <DragEdgeHandle className="top-2 bottom-2 left-0 w-1.5 rounded-l-lg" />
          <DragEdgeHandle className="top-2 right-0 bottom-2 w-1.5 rounded-r-lg" />
        </>
      )}
      {isOutlineOnly ?
        content
      : <div
          data-interactive-content={hasInteractiveContent ? '' : undefined}
          style={{
            transform:
              internalScale < 1 ? `scale(${internalScale})` : undefined,
            transformOrigin: 'top left',
            width:
              internalScale < 1 ?
                `${(100 / internalScale).toFixed(2)}%`
              : '100%',
            height:
              internalScale < 1 ?
                `${(100 / internalScale).toFixed(2)}%`
              : '100%',
            touchAction: isContentFocused ? 'auto' : undefined,
          }}
          className={cn(
            'h-full w-full overflow-hidden',
            isContentFocused && 'cursor-default select-auto',
          )}
        >
          {content}
        </div>
      }
      {selected && (
        <>
          <div
            className={cn(
              'pointer-events-none absolute inset-0 rounded-lg ring-1',
              isContentFocused ? 'ring-brand/30' : 'ring-brand/50',
            )}
          />
          <ResizeHandle
            direction="ne"
            scale={scale}
            onResizeDelta={handleResizeDelta}
          />
          <ResizeHandle
            direction="se"
            scale={scale}
            onResizeDelta={handleResizeDelta}
          />
          <ResizeHandle
            direction="sw"
            scale={scale}
            onResizeDelta={handleResizeDelta}
          />
          <ResizeHandle
            direction="nw"
            scale={scale}
            onResizeDelta={handleResizeDelta}
          />
        </>
      )}
    </div>
  )
})
