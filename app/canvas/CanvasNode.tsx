import { useCallback, useRef, useState, useEffect, memo } from 'react'
import { useDrag } from '@use-gesture/react'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  fonts as themeFonts,
  colors as themeColors,
  radii as themeRadii,
} from '../../web/style/tokens.stylex.js'

import { utilityState as themeUtilityState } from '../../web/style/tokens.stylex.js'

import { motion as themeMotion } from '../../web/style/tokens.stylex.js'

const stylexStyles4 = stylex.create({
  rule1: {
    '--tw-duration': '.15s',
    WebkitUserSelect: 'none',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    boxSizing: 'border-box',
    color: themeColors['--color-card-foreground'],
    cursor: 'grab',
    pointerEvents: 'auto',
    position: 'absolute',
    touchAction: 'none',
    transform: 'var(--canvas-node-transform)',
    transformOrigin: 'var(--canvas-node-transform-origin)',
    transitionDuration: '.15s',
    transitionProperty: 'box-shadow',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    userSelect: 'none',
    height: 'var(--canvas-node-height)',
    left: 'var(--canvas-node-left)',
    top: 'var(--canvas-node-top)',
    width: 'var(--canvas-node-width)',
  },
  rule2: {
    '--tw-shadow':
      '0 4px 6px -1px var(--tw-shadow-color, #0000001a), 0 2px 4px -2px var(--tw-shadow-color, #0000001a)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
  },
  rule3: {
    '--tw-shadow': '0 0 #0000',
    borderColor: '#0000',
    backgroundColor: '#0000',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
  },
  rule4: { pointerEvents: 'none' },
})

const stylexStyles3 = stylex.create({
  rule1: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 60%, transparent)`,
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: themeColors['--color-background'],
    position: 'absolute',
    touchAction: 'none',
    height: `calc(${themeLayout['--spacing']} * 2.5)`,
    width: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    cursor: 'nwse-resize',
    left: `calc(${themeLayout['--spacing']} * -1.5)`,
    top: `calc(${themeLayout['--spacing']} * -1.5)`,
  },
  rule2: {
    cursor: 'nesw-resize',
    right: `calc(${themeLayout['--spacing']} * -1.5)`,
    top: `calc(${themeLayout['--spacing']} * -1.5)`,
  },
  rule3: {
    cursor: 'nwse-resize',
    bottom: `calc(${themeLayout['--spacing']} * -1.5)`,
    right: `calc(${themeLayout['--spacing']} * -1.5)`,
  },
  rule4: {
    cursor: 'nesw-resize',
    bottom: `calc(${themeLayout['--spacing']} * -1.5)`,
    left: `calc(${themeLayout['--spacing']} * -1.5)`,
  },
  rule5: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
  },
  rule6: {
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    '--tw-shadow':
      '0 1px 3px 0 var(--tw-shadow-color, #0000001a), 0 1px 2px -1px var(--tw-shadow-color, #0000001a)',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
  },
  rule7: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 20%, transparent)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    padding: themeLayout['--spacing'],
    overflow: 'hidden',
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'center',
    height: '100%',
  },
  rule2: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    overflow: 'hidden',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'center',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule3: {
    alignItems: 'center',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    height: '100%',
  },
  rule4: {
    borderTopLeftRadius: themeRadii['--radius-lg'],
    borderTopRightRadius: themeRadii['--radius-lg'],
    height: `calc(${themeLayout['--spacing']} * 1.5)`,
    left: `calc(${themeLayout['--spacing']} * 2)`,
    right: `calc(${themeLayout['--spacing']} * 2)`,
    top: 0,
  },
  rule5: {
    borderBottomLeftRadius: themeRadii['--radius-lg'],
    borderBottomRightRadius: themeRadii['--radius-lg'],
    bottom: 0,
    height: `calc(${themeLayout['--spacing']} * 1.5)`,
    left: `calc(${themeLayout['--spacing']} * 2)`,
    right: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule6: {
    borderBottomLeftRadius: themeRadii['--radius-lg'],
    borderTopLeftRadius: themeRadii['--radius-lg'],
    bottom: `calc(${themeLayout['--spacing']} * 2)`,
    left: 0,
    top: `calc(${themeLayout['--spacing']} * 2)`,
    width: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule7: {
    borderBottomRightRadius: themeRadii['--radius-lg'],
    borderTopRightRadius: themeRadii['--radius-lg'],
    bottom: `calc(${themeLayout['--spacing']} * 2)`,
    right: 0,
    top: `calc(${themeLayout['--spacing']} * 2)`,
    width: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule8: {
    '--tw-ring-shadow':
      'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    inset: 0,
    borderRadius: themeRadii['--radius-lg'],
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    pointerEvents: 'none',
    position: 'absolute',
  },
  rule9: {
    '--tw-ring-color': `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
  },
  rule10: {
    '--tw-ring-color': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
  },
  rule11: {
    overflow: 'hidden',
    touchAction: 'var(--canvas-content-touch-action)',
    transform: 'var(--canvas-content-transform)',
    transformOrigin: 'var(--canvas-content-transform-origin)',
    height: '100%',
    width: '100%',
  },
  rule12: { WebkitUserSelect: 'auto', cursor: 'default', userSelect: 'auto' },
  rule13: {
    inset: `calc(${themeLayout['--spacing']} * -1.5)`,
    borderRadius: themeRadii['--radius-lg'],
    pointerEvents: 'auto',
    position: 'absolute',
    zIndex: -1,
  },
})

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
import { CanvasGeometryNode } from './CanvasGeometryNode.js'

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

// DragEdgeHandle renders a thin invisible border strip that can initiate node drag.
function DragEdgeHandle({ sx }: { sx?: stylex.StyleXStyles }) {
  return (
    <div
      aria-hidden="true"
      data-drag-handle=""
      sx={[dragEdgeStyles.edge, sx]}
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

  const positionStyles = {
    nw: stylexStyles2.rule1,
    ne: stylexStyles2.rule2,
    se: stylexStyles2.rule3,
    sw: stylexStyles2.rule4,
  }[direction]

  const handlers = bind()
  const origPointerDown = handlers.onPointerDown
  handlers.onPointerDown = (e: React.PointerEvent) => {
    e.stopPropagation()
    origPointerDown?.(e)
  }

  return <div {...handlers} sx={[[stylexStyles3.rule1], [positionStyles]]} />
}

// useCanvasNodeController owns visibility debounce, drag selection, and resize
// projection for one canvas node.
function useCanvasNodeController({
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
  const [mountedAfterHide, setMountedAfterHide] = useState(false)
  const wasVisible = useRef(visible)
  const unmountTimer = useRef<number | null>(null)

  // Debounced unmount keeps a node alive briefly after it leaves the viewport so
  // fast pan/zoom updates do not churn embedded object viewers.
  useEffect(() => {
    if (visible) {
      if (unmountTimer.current) {
        clearTimeout(unmountTimer.current)
        unmountTimer.current = null
      }
      setMountedAfterHide(false)
      wasVisible.current = true
      return
    }

    if (wasVisible.current) {
      setMountedAfterHide(true)
    }
    wasVisible.current = false
    unmountTimer.current = window.setTimeout(() => {
      setMountedAfterHide(false)
      unmountTimer.current = null
    }, UNMOUNT_DEBOUNCE_MS)
    return () => {
      if (unmountTimer.current) {
        clearTimeout(unmountTimer.current)
        unmountTimer.current = null
      }
    }
  }, [visible])

  const hasInteractiveContent =
    node.type === 'world_object' || node.type === 'text'

  const bind = useDrag(
    ({ delta: [dx, dy], first, last, event }) => {
      if (first) {
        const shiftKey =
          event instanceof MouseEvent || event instanceof PointerEvent
            ? event.shiftKey
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

  const handleNodeSelect = useCallback(
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

  const handleNodeKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key !== 'Enter' && e.key !== ' ') return
      e.preventDefault()
      e.stopPropagation()
      onSelectWithFocus(node.id, e.shiftKey, 'border')
    },
    [node.id, onSelectWithFocus],
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

  return {
    callbacks,
    handleNodeKeyDown,
    handleNodeSelect,
    handleResizeDelta,
    handleTextChange,
    hasInteractiveContent,
    internalScale,
    isCompact,
    isContentFocused,
    isOutlineOnly,
    mountedAfterHide,
    node,
    scale,
    selected,
    showsBorderDragHandles,
    resizeOverride,
    visible,
    wasVisible,
    wrappedBind,
  }
}

interface CanvasNodeContentProps {
  node: CanvasNodeData
  callbacks: CanvasCallbacks
  isOutlineOnly: boolean
  isCompact: boolean
  onTextChange: (content: string) => void
}

/** CanvasNodeContent renders the body of a node by its type and zoom level. */
function CanvasNodeContent({
  node,
  callbacks,
  isOutlineOnly,
  isCompact,
  onTextChange,
}: CanvasNodeContentProps) {
  if (isOutlineOnly) {
    // The short label shown in outline-only mode.
    return (
      <div sx={stylexStyles.rule1}>
        <span sx={stylexStyles.rule2}>{node.objectKey ?? node.type}</span>
      </div>
    )
  }

  if (node.type === 'text') {
    return (
      <CanvasTextNode
        content={node.textContent ?? ''}
        onChange={onTextChange}
      />
    )
  }

  if ((node.type === 'drawing' || node.type === 'shape') && node.shapeData) {
    return <CanvasGeometryNode node={node} />
  }

  if (node.type === 'world_object' && callbacks.renderNodeContent) {
    return callbacks.renderNodeContent(node)
  }

  if (isCompact) {
    return <div sx={stylexStyles.rule3}>{node.type}</div>
  }

  return null
}

/** nodeTypeBackgroundStyles returns the card background for a node type. */
function nodeTypeBackgroundStyles(
  type: CanvasNodeData['type'],
  isOutlineOnly: boolean,
): stylex.StyleXStyles {
  if (isOutlineOnly) return stylexStyles2.rule5
  if (type === 'world_object') {
    return stylexStyles2.rule6
  }
  if (type === 'text') return stylexStyles2.rule7
  return null
}

/** isInkNode reports whether the node is a drawing, shape, or text node. */
function isInkNode(type: CanvasNodeData['type']): boolean {
  return type === 'drawing' || type === 'shape' || type === 'text'
}

/** nodeStyles composes the native node surface in its original override order. */
function nodeStyles(
  node: CanvasNodeData,
  isOutlineOnly: boolean,
  selected: boolean,
): stylex.StyleXStyles {
  const unselectedInk = !isOutlineOnly && !selected
  return [
    stylexStyles4.rule1,
    nodeTypeBackgroundStyles(node.type, isOutlineOnly),
    selected && stylexStyles4.rule2,
    unselectedInk && isInkNode(node.type) && stylexStyles4.rule3,
    unselectedInk && node.type === 'drawing' && stylexStyles4.rule4,
  ]
}

/** CanvasNodeBorderHandles renders the invisible edges that start a node drag. */
function CanvasNodeBorderHandles() {
  return (
    <>
      <DragEdgeHandle sx={stylexStyles.rule4} />
      <DragEdgeHandle sx={stylexStyles.rule5} />
      <DragEdgeHandle sx={stylexStyles.rule6} />
      <DragEdgeHandle sx={stylexStyles.rule7} />
    </>
  )
}

interface CanvasNodeSelectionProps {
  scale: number
  isContentFocused: boolean
  onResizeDelta: ResizeHandleProps['onResizeDelta']
}

/** CanvasNodeSelection renders the selection ring and the resize handles. */
function CanvasNodeSelection({
  scale,
  isContentFocused,
  onResizeDelta,
}: CanvasNodeSelectionProps) {
  return (
    <>
      <div
        sx={[
          stylexStyles.rule8,
          isContentFocused ? stylexStyles.rule9 : stylexStyles.rule10,
        ]}
      />
      {(['ne', 'se', 'sw', 'nw'] as const).map((direction) => (
        <ResizeHandle
          key={direction}
          direction={direction}
          scale={scale}
          onResizeDelta={onResizeDelta}
        />
      ))}
    </>
  )
}

interface CanvasNodeContentFrameProps {
  hasInteractiveContent: boolean
  internalScale: number
  isContentFocused: boolean
  children: React.ReactNode
}

/** CanvasNodeContentFrame counter-scales node content to its native pixel size. */
function CanvasNodeContentFrame({
  hasInteractiveContent,
  internalScale,
  isContentFocused,
  children,
}: CanvasNodeContentFrameProps) {
  const contentSize =
    internalScale < 1 ? `${(100 / internalScale).toFixed(2)}%` : '100%'

  return (
    <div
      data-interactive-content={hasInteractiveContent ? '' : undefined}
      style={{
        '--canvas-content-transform':
          internalScale < 1 ? `scale(${internalScale})` : 'none',
        '--canvas-content-transform-origin': 'top left',
        '--canvas-content-width': contentSize,
        '--canvas-content-height': contentSize,
        '--canvas-content-touch-action': isContentFocused ? 'auto' : 'none',
      }}
      sx={[stylexStyles.rule11, !!isContentFocused && stylexStyles.rule12]}
    >
      {children}
    </div>
  )
}

// CanvasNode renders a single positioned node on the canvas.
export const CanvasNode = memo(function CanvasNode(props: CanvasNodeProps) {
  const {
    callbacks,
    handleNodeKeyDown,
    handleNodeSelect,
    handleResizeDelta,
    handleTextChange,
    hasInteractiveContent,
    internalScale,
    isCompact,
    isContentFocused,
    isOutlineOnly,
    mountedAfterHide,
    node,
    scale,
    selected,
    showsBorderDragHandles,
    resizeOverride,
    visible,
    wasVisible,
    wrappedBind,
  } = useCanvasNodeController(props)

  if (!visible && !mountedAfterHide && !wasVisible.current) return null

  const content = (
    <CanvasNodeContent
      node={node}
      callbacks={callbacks}
      isOutlineOnly={isOutlineOnly}
      isCompact={isCompact}
      onTextChange={handleTextChange}
    />
  )

  return (
    <div
      role="button"
      tabIndex={0}
      data-canvas-node={node.id}
      {...wrappedBind()}
      onClick={handleNodeSelect}
      onKeyDown={handleNodeKeyDown}
      style={{
        '--canvas-node-left': `${resizeOverride?.x ?? node.x}px`,
        '--canvas-node-top': `${resizeOverride?.y ?? node.y}px`,
        '--canvas-node-width': `${resizeOverride?.w ?? node.width}px`,
        '--canvas-node-height': `${resizeOverride?.h ?? node.height}px`,
      }}
      sx={[
        nodeStyles(node, isOutlineOnly, selected),
        nodeLayerStyles.layer(node.zIndex),
      ]}
    >
      {/* Invisible hit area extending outward from the border for easier clicking.
          z-[-1] keeps it behind node content so it doesn't steal clicks. */}
      <div sx={stylexStyles.rule13} />
      {showsBorderDragHandles && <CanvasNodeBorderHandles />}
      {isOutlineOnly ? (
        content
      ) : (
        <CanvasNodeContentFrame
          hasInteractiveContent={hasInteractiveContent}
          internalScale={internalScale}
          isContentFocused={isContentFocused}
        >
          {content}
        </CanvasNodeContentFrame>
      )}
      {selected && (
        <CanvasNodeSelection
          scale={scale}
          isContentFocused={isContentFocused}
          onResizeDelta={handleResizeDelta}
        />
      )}
    </div>
  )
})

// dragEdgeStyles owns the drag target beneath each side-specific caller.
const dragEdgeStyles = stylex.create({
  edge: { backgroundColor: 'transparent', position: 'absolute', zIndex: 10 },
})

// nodeLayerStyles preserves each node's dynamic integer stacking order.
const nodeLayerStyles = stylex.create({
  layer: (zIndex: number) => ({ zIndex }),
})
