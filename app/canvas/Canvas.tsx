import { useState, useCallback, useRef, useEffect, useMemo } from 'react'

import {
  type SubItemsCallback,
  useOpenCommand,
} from '@s4wave/web/command/CommandContext.js'
import { cn } from '@s4wave/web/style/utils.js'

import type {
  CanvasStateData,
  CanvasNodeData,
  CanvasCallbacks,
  CanvasTool,
  EphemeralEdge,
} from './types.js'
import { useCanvasViewport, computeGridStyle } from './useCanvasViewport.js'
import { useVisibleNodes, type ContainerSize } from './useVisibleNodes.js'
import { useCanvasSelection } from './useCanvasSelection.js'
import { useCanvasActions } from './useCanvasActions.js'
import { useCanvasCommands } from './useCanvasCommands.js'
import { CanvasNode } from './CanvasNode.js'
import { CanvasEdgeLayer } from './CanvasEdgeLayer.js'
import { CanvasDrawingLayer } from './CanvasDrawingLayer.js'
import { CanvasTextNode } from './CanvasTextNode.js'
import { CanvasToolbar } from './CanvasToolbar.js'
import { CanvasContextMenu } from './CanvasContextMenu.js'
import {
  CanvasMinimap,
  DEFAULT_MINIMAP_WIDTH,
  DEFAULT_MINIMAP_HEIGHT,
} from './CanvasMinimap.js'
import { CanvasSelectionOverlay } from './CanvasSelectionOverlay.js'
import { CanvasScaleIndicator } from './CanvasScaleIndicator.js'
import { CanvasSyncStatus } from './CanvasSyncStatus.js'

// XL_BREAKPOINT is the container width threshold for the xl tailwind breakpoint.
const XL_BREAKPOINT = 1280

// DEFAULT_TEXT_NODE_WIDTH is the default width for new text nodes.
const DEFAULT_TEXT_NODE_WIDTH = 200

// MIN_TEXT_NODE_HEIGHT is the minimum height for a text node (single line + padding).
const MIN_TEXT_NODE_HEIGHT = 32

// PENDING_OBJECT_INSERT_TTL_MS bounds how long a context-menu placement hint
// stays valid while the user picks an object from the command palette.
const PENDING_OBJECT_INSERT_TTL_MS = 5000

// generateNodeId creates a unique node ID.
function generateNodeId(): string {
  return `node-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
}

// CanvasProps are the props for the Canvas component.
interface CanvasProps {
  state: CanvasStateData
  ephemeralEdges?: EphemeralEdge[]
  tool?: CanvasTool
  callbacks: CanvasCallbacks
  pendingMutations?: number
  objectSubItems?: SubItemsCallback
  focusNodeId?: string | null
  className?: string
}

// Canvas is the main canvas container that composes all canvas sub-components.
export function Canvas({
  state,
  ephemeralEdges,
  tool: toolProp,
  callbacks,
  pendingMutations,
  objectSubItems,
  focusNodeId,
  className,
}: CanvasProps) {
  const [toolInternal, setToolInternal] = useState<CanvasTool>('select')
  const tool = toolProp ?? toolInternal

  const [containerSize, setContainerSize] = useState<ContainerSize>({
    width: 0,
    height: 0,
  })

  const viewportContainerRef = useRef<HTMLDivElement | null>(null)
  const viewportRef = useRef({ x: 0, y: 0, scale: 1 })
  const lastContextMenuCanvasPositionRef = useRef<{
    x: number
    y: number
  } | null>(null)
  const pendingObjectInsertRef = useRef<{
    x: number
    y: number
    createdAt: number
  } | null>(null)
  const openCommand = useOpenCommand()

  const selection = useCanvasSelection()

  const dragSelectHandler = useMemo(
    () => ({
      onStart: (x: number, y: number) => {
        selection.startDragRect(x, y)
      },
      onMove: (x: number, y: number) => {
        selection.updateDragRect(x, y)
      },
      onEnd: () => {
        selection.endDragRect(
          state.nodes,
          viewportRef.current.x,
          viewportRef.current.y,
          viewportRef.current.scale,
        )
      },
    }),
    [selection, state.nodes],
  )

  const {
    viewport,
    setViewport,
    gestureLayerRef,
    transformLayerRef,
    gridLayerRef,
  } = useCanvasViewport({
    tool,
    dragSelect: dragSelectHandler,
    containerRef: viewportContainerRef,
  })

  useEffect(() => {
    viewportRef.current = viewport
  }, [viewport])

  useEffect(() => {
    if (!focusNodeId) return
    const node = state.nodes.get(focusNodeId)
    if (!node) return
    if (
      selection.selectedNodeIds.size !== 1 ||
      !selection.selectedNodeIds.has(focusNodeId)
    ) {
      selection.setSelection(new Set([focusNodeId]))
    }
    if (selection.focus !== 'border') {
      selection.setFocus('border')
    }
    const cx = node.x + node.width / 2
    const cy = node.y + node.height / 2
    setViewport({
      x: containerSize.width / 2 - cx * viewport.scale,
      y: containerSize.height / 2 - cy * viewport.scale,
      scale: viewport.scale,
    })
  }, [
    focusNodeId,
    state.nodes,
    selection.selectedNodeIds,
    selection.focus,
    selection.setSelection,
    selection.setFocus,
    containerSize,
    viewport.scale,
    setViewport,
  ])

  // Observe container size.
  useEffect(() => {
    const el = viewportContainerRef.current
    if (!el) return
    const observer = new ResizeObserver((entries) => {
      for (const entry of entries) {
        setContainerSize({
          width: entry.contentRect.width,
          height: entry.contentRect.height,
        })
      }
    })
    observer.observe(el)
    return () => observer.disconnect()
  }, [])

  // Ephemeral text editor state (text tool click before commit).
  const [pendingText, setPendingText] = useState<{
    x: number
    y: number
  } | null>(null)
  const pendingTextRef = useRef<HTMLDivElement | null>(null)

  const handlePendingTextCommit = useCallback(
    (content: string) => {
      if (!pendingText) return
      const el = pendingTextRef.current
      const id = generateNodeId()
      const w =
        el ?
          Math.max(el.scrollWidth + 4, DEFAULT_TEXT_NODE_WIDTH)
        : DEFAULT_TEXT_NODE_WIDTH
      const h =
        el ?
          Math.max(el.scrollHeight + 4, MIN_TEXT_NODE_HEIGHT)
        : MIN_TEXT_NODE_HEIGHT
      const node: CanvasNodeData = {
        id,
        x: pendingText.x - w / 2,
        y: pendingText.y - h / 2,
        width: w,
        height: h,
        zIndex: 0,
        type: 'text',
        textContent: content,
      }
      callbacks.onNodesChange?.(new Map([[id, node]]))
      selection.toggleSelect(id, false)
      setPendingText(null)
    },
    [pendingText, callbacks, selection],
  )

  const handlePendingTextCancel = useCallback(() => {
    setPendingText(null)
  }, [])

  // Local node overrides during drag (not persisted until drop).
  const [dragOffsets, setDragOffsets] = useState<Map<
    string,
    { dx: number; dy: number }
  > | null>(null)

  // Merge server state with local drag offsets.
  const effectiveNodes = useMemo(() => {
    if (!dragOffsets) return state.nodes
    const merged = new Map(state.nodes)
    for (const [id, offset] of dragOffsets) {
      const n = merged.get(id)
      if (n) {
        merged.set(id, { ...n, x: n.x + offset.dx, y: n.y + offset.dy })
      }
    }
    return merged
  }, [state.nodes, dragOffsets])

  const visibleNodeIds = useVisibleNodes(
    effectiveNodes,
    viewport,
    containerSize,
  )

  const cancelDrag = useCallback(() => {
    setDragOffsets(null)
  }, [])

  const onToolChange = toolProp === undefined ? setToolInternal : undefined
  const { actions, moveSelected } = useCanvasActions({
    selection,
    nodes: state.nodes,
    callbacks,
    viewport,
    setViewport,
    containerSize,
  })

  // Notify consumer of selection changes.
  useEffect(() => {
    callbacks.onNodeSelect?.(selection.selectedNodeIds)
  }, [selection.selectedNodeIds, callbacks])

  const handleNodeMove = useCallback(
    (id: string, dx: number, dy: number) => {
      // Accumulate local drag offsets without firing RPC.
      const idsToMove =
        selection.selectedNodeIds.has(id) ?
          selection.selectedNodeIds
        : new Set([id])

      setDragOffsets((prev) => {
        const next = new Map(prev ?? [])
        for (const moveId of idsToMove) {
          const existing = next.get(moveId) ?? { dx: 0, dy: 0 }
          next.set(moveId, { dx: existing.dx + dx, dy: existing.dy + dy })
        }
        return next
      })
    },
    [selection.selectedNodeIds],
  )

  const handleNodeMoveEnd = useCallback(() => {
    if (!dragOffsets || dragOffsets.size === 0) return

    // Apply accumulated offsets and persist via callback.
    const next = new Map<
      string,
      typeof state.nodes extends Map<string, infer V> ? V : never
    >()
    for (const [id, offset] of dragOffsets) {
      const n = state.nodes.get(id)
      if (n) {
        next.set(id, { ...n, x: n.x + offset.dx, y: n.y + offset.dy })
      }
    }
    setDragOffsets(null)
    if (next.size > 0) {
      callbacks.onNodesChange?.(next)
    }
  }, [dragOffsets, state, callbacks])

  const handleNodeResize = useCallback(
    (id: string, resized: import('./types.js').CanvasNodeData) => {
      callbacks.onNodesChange?.(new Map([[id, resized]]))
    },
    [callbacks],
  )

  // screenToCanvas converts screen-relative coords to canvas space.
  const screenToCanvas = useCallback(
    (sx: number, sy: number) => ({
      x: (sx - viewport.x) / viewport.scale,
      y: (sy - viewport.y) / viewport.scale,
    }),
    [viewport],
  )

  const getViewportCenterCanvasPosition = useCallback(() => {
    const rect = viewportContainerRef.current?.getBoundingClientRect()
    if (!rect) {
      return { x: 0, y: 0 }
    }
    return screenToCanvas(rect.width / 2, rect.height / 2)
  }, [screenToCanvas])

  const addTextAt = useCallback(
    (point?: { x: number; y: number }) => {
      const nextPoint = point ?? getViewportCenterCanvasPosition()
      setPendingText(nextPoint)
      setToolInternal('select')
    },
    [getViewportCenterCanvasPosition],
  )

  const resolvePendingObjectInsertPoint = useCallback(() => {
    const pending = pendingObjectInsertRef.current
    pendingObjectInsertRef.current = null
    if (!pending) {
      return getViewportCenterCanvasPosition()
    }
    if (Date.now() - pending.createdAt > PENDING_OBJECT_INSERT_TTL_MS) {
      return getViewportCenterCanvasPosition()
    }
    return { x: pending.x, y: pending.y }
  }, [getViewportCenterCanvasPosition])

  const addObjectAt = useCallback(
    (objectKey: string) => {
      const point = resolvePendingObjectInsertPoint()
      callbacks.onPinObject?.(objectKey, point.x, point.y)
    },
    [callbacks, resolvePendingObjectInsertPoint],
  )

  const [contextMenuState, setContextMenuState] = useState<{
    position: { x: number; y: number }
    canvasPosition: { x: number; y: number }
  } | null>(null)

  const closeContextMenu = useCallback(() => {
    setContextMenuState(null)
  }, [])

  const handleCommandAddText = useCallback(() => {
    addTextAt()
  }, [addTextAt])

  const handleContextMenuPaste = useCallback(() => {
    closeContextMenu()
    actions.paste()
  }, [closeContextMenu, actions])

  const handleContextMenuAddText = useCallback(() => {
    const point = lastContextMenuCanvasPositionRef.current
    if (!point) return
    lastContextMenuCanvasPositionRef.current = null
    closeContextMenu()
    addTextAt(point)
  }, [closeContextMenu, addTextAt])

  const handleContextMenuAddObject = useCallback(() => {
    const point = lastContextMenuCanvasPositionRef.current
    if (!point) return
    lastContextMenuCanvasPositionRef.current = null
    pendingObjectInsertRef.current = {
      ...point,
      createdAt: Date.now(),
    }
    closeContextMenu()
    openCommand('canvas.add-object')
  }, [closeContextMenu, openCommand])

  // handleToolbarAddObject opens the object picker without a pinned position
  // so the chosen object lands at the current viewport center.
  const handleToolbarAddObject = useCallback(() => {
    pendingObjectInsertRef.current = null
    openCommand('canvas.add-object')
  }, [openCommand])

  const canAddObject = !!callbacks.onPinObject && !!objectSubItems

  const handleContextMenuFitView = useCallback(() => {
    closeContextMenu()
    actions['fit-view']()
  }, [closeContextMenu, actions])

  const handleContextMenuZoomReset = useCallback(() => {
    closeContextMenu()
    actions['zoom-reset']()
  }, [closeContextMenu, actions])

  const handleContextMenuSelectAll = useCallback(() => {
    closeContextMenu()
    actions['select-all']()
  }, [closeContextMenu, actions])

  useCanvasCommands({
    actions,
    moveSelected,
    selectionFocus: selection.focus,
    hasSelection: selection.selectedNodeIds.size > 0,
    onToolChange,
    onCancelDrag: cancelDrag,
    onSetFocus: selection.setFocus,
    onAddText: handleCommandAddText,
    onAddObject: addObjectAt,
    addObjectSubItems: objectSubItems,
  })

  const handleBackgroundClick = useCallback(
    (e: React.MouseEvent) => {
      const target = e.target as HTMLElement
      if (target.closest('[data-canvas-node]')) return

      if (tool === 'text') {
        const rect = viewportContainerRef.current?.getBoundingClientRect()
        if (!rect) return
        const pt = screenToCanvas(e.clientX - rect.left, e.clientY - rect.top)
        setPendingText({ x: pt.x, y: pt.y })
        setToolInternal('select')
        return
      }

      selection.clearSelection()
    },
    [tool, selection, screenToCanvas],
  )

  const handleBackgroundContextMenu = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => {
      const target = e.target as HTMLElement
      if (target.closest('[data-canvas-node]')) return
      const rect = viewportContainerRef.current?.getBoundingClientRect()
      if (!rect) return
      e.preventDefault()
      const canvasPosition = screenToCanvas(
        e.clientX - rect.left,
        e.clientY - rect.top,
      )
      lastContextMenuCanvasPositionRef.current = canvasPosition
      setContextMenuState({
        position: { x: e.clientX, y: e.clientY },
        canvasPosition,
      })
    },
    [screenToCanvas],
  )

  // Object tool: drag on background to create a world_object node.
  const objectDragRef = useRef<{ startX: number; startY: number } | null>(null)
  const [objectDragRect, setObjectDragRect] = useState<{
    x: number
    y: number
    w: number
    h: number
  } | null>(null)

  const handleBackgroundPointerDown = useCallback(
    (e: React.PointerEvent) => {
      if (tool !== 'object') return
      const target = e.target as HTMLElement
      if (target.closest('[data-canvas-node]')) return
      const rect = viewportContainerRef.current?.getBoundingClientRect()
      if (!rect) return
      const pt = screenToCanvas(e.clientX - rect.left, e.clientY - rect.top)
      objectDragRef.current = { startX: pt.x, startY: pt.y }
      setObjectDragRect({ x: pt.x, y: pt.y, w: 0, h: 0 })
      ;(e.target as HTMLElement).setPointerCapture(e.pointerId)
      e.preventDefault()
    },
    [tool, screenToCanvas],
  )

  const handleBackgroundPointerMove = useCallback(
    (e: React.PointerEvent) => {
      if (!objectDragRef.current) return
      const rect = viewportContainerRef.current?.getBoundingClientRect()
      if (!rect) return
      const pt = screenToCanvas(e.clientX - rect.left, e.clientY - rect.top)
      const sx = objectDragRef.current.startX
      const sy = objectDragRef.current.startY
      setObjectDragRect({
        x: Math.min(sx, pt.x),
        y: Math.min(sy, pt.y),
        w: Math.abs(pt.x - sx),
        h: Math.abs(pt.y - sy),
      })
    },
    [screenToCanvas],
  )

  const handleBackgroundPointerUp = useCallback(() => {
    if (!objectDragRef.current || !objectDragRect) {
      objectDragRef.current = null
      setObjectDragRect(null)
      return
    }
    objectDragRef.current = null
    const r = objectDragRect
    setObjectDragRect(null)
    if (r.w < 20 || r.h < 20) return
    const id = generateNodeId()
    const node: CanvasNodeData = {
      id,
      x: r.x,
      y: r.y,
      width: r.w,
      height: r.h,
      zIndex: 0,
      type: 'world_object',
    }
    callbacks.onNodesChange?.(new Map([[id, node]]))
    selection.toggleSelect(id, false)
    setToolInternal('select')
  }, [objectDragRect, callbacks, selection])

  // Transform and grid styles are initial values for React rendering.
  // During gestures, useCanvasViewport applies these directly to the
  // DOM via transformLayerRef/gridLayerRef for zero-cost panning.
  const transformStyle = useMemo(
    () => ({
      transform: `translate3d(${viewport.x}px, ${viewport.y}px, 0) scale(${viewport.scale})`,
      transformOrigin: '0 0',
    }),
    [viewport.x, viewport.y, viewport.scale],
  )

  const gridStyle = useMemo(() => computeGridStyle(viewport), [viewport])

  const nodeEntries = useMemo(() => {
    const entries: Array<{
      id: string
      node: typeof effectiveNodes extends Map<string, infer V> ? V : never
    }> = []
    for (const [id, node] of effectiveNodes) {
      entries.push({ id, node })
    }
    return entries
  }, [effectiveNodes])

  const handleStrokeComplete = useCallback(
    (node: CanvasNodeData) => {
      callbacks.onNodesChange?.(new Map([[node.id, node]]))
    },
    [callbacks],
  )

  return (
    <div className={cn('flex h-full outline-none', className)}>
      <CanvasToolbar
        tool={tool}
        onToolChange={onToolChange ?? (() => {})}
        actions={actions}
        onAddObject={canAddObject ? handleToolbarAddObject : undefined}
      />
      <div
        ref={viewportContainerRef}
        data-testid="canvas-viewport"
        className={cn(
          'relative flex-1 touch-none overflow-hidden bg-[var(--color-background-canvas)] outline-none',
          tool === 'text' && 'cursor-crosshair',
          tool === 'object' && 'cursor-crosshair',
        )}
        onClick={handleBackgroundClick}
        onContextMenu={handleBackgroundContextMenu}
        onPointerDown={(e) => {
          setContextMenuState(null)
          handleBackgroundPointerDown(e)
        }}
        onPointerMove={handleBackgroundPointerMove}
        onPointerUp={handleBackgroundPointerUp}
        tabIndex={0}
      >
        <div
          ref={gridLayerRef}
          className="pointer-events-none absolute inset-0"
          style={gridStyle}
        />
        {/* Gesture layer for viewport pan/drag-select. Sits below the
            transform layer so canvas nodes receive pointer events directly
            without viewport gesture interference. */}
        <div
          ref={gestureLayerRef}
          className="absolute inset-0"
          style={{ touchAction: 'none' }}
        />
        <div
          ref={transformLayerRef}
          className="pointer-events-none"
          style={transformStyle}
        >
          <CanvasEdgeLayer
            edges={state.edges}
            ephemeralEdges={ephemeralEdges}
            nodes={effectiveNodes}
            callbacks={callbacks}
          />
          {nodeEntries.map(({ id, node }) => (
            <CanvasNode
              key={id}
              node={node}
              scale={viewport.scale}
              selected={selection.selectedNodeIds.has(id)}
              focus={selection.focus}
              visible={visibleNodeIds.has(id)}
              callbacks={callbacks}
              onSelectWithFocus={selection.selectWithFocus}
              onMove={handleNodeMove}
              onMoveEnd={handleNodeMoveEnd}
              onResize={handleNodeResize}
            />
          ))}
          {pendingText && (
            <div
              ref={pendingTextRef}
              style={{
                position: 'absolute',
                left: pendingText.x - DEFAULT_TEXT_NODE_WIDTH / 2,
                top: pendingText.y - MIN_TEXT_NODE_HEIGHT / 2,
                width: DEFAULT_TEXT_NODE_WIDTH,
                minHeight: MIN_TEXT_NODE_HEIGHT,
                touchAction: 'none',
              }}
              className="bg-background-card/30 text-card-foreground pointer-events-auto rounded-lg backdrop-blur-sm"
              onPointerDown={(e) => e.stopPropagation()}
              onClick={(e) => e.stopPropagation()}
            >
              <CanvasTextNode
                content=""
                autoEdit
                onChange={handlePendingTextCommit}
                onCancel={handlePendingTextCancel}
              />
            </div>
          )}
        </div>
        <CanvasDrawingLayer
          visible={tool === 'draw'}
          viewport={viewport}
          onStrokeComplete={handleStrokeComplete}
        />
        {objectDragRect && (
          <div
            className="border-brand/30 bg-brand/5 pointer-events-none absolute rounded-lg border border-dashed"
            style={{
              left: objectDragRect.x * viewport.scale + viewport.x,
              top: objectDragRect.y * viewport.scale + viewport.y,
              width: objectDragRect.w * viewport.scale,
              height: objectDragRect.h * viewport.scale,
            }}
          />
        )}
        <CanvasSelectionOverlay dragRect={selection.dragRect} />
        <CanvasMinimap
          nodes={state.nodes}
          viewport={viewport}
          containerSize={containerSize}
          onViewportChange={setViewport}
          width={
            containerSize.width >= XL_BREAKPOINT ?
              DEFAULT_MINIMAP_WIDTH
            : DEFAULT_MINIMAP_WIDTH / 2
          }
          height={
            containerSize.width >= XL_BREAKPOINT ?
              DEFAULT_MINIMAP_HEIGHT
            : DEFAULT_MINIMAP_HEIGHT / 2
          }
        />
        <CanvasScaleIndicator scale={viewport.scale} />
        {pendingMutations !== undefined && (
          <CanvasSyncStatus pending={pendingMutations} />
        )}
      </div>
      <CanvasContextMenu
        state={
          contextMenuState ? { position: contextMenuState.position } : null
        }
        canAddObject={canAddObject}
        onClose={closeContextMenu}
        onPaste={handleContextMenuPaste}
        onAddText={handleContextMenuAddText}
        onAddObject={handleContextMenuAddObject}
        onFitView={handleContextMenuFitView}
        onZoomReset={handleContextMenuZoomReset}
        onSelectAll={handleContextMenuSelectAll}
      />
    </div>
  )
}
