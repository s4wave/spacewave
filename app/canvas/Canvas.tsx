/* eslint-disable react-doctor/no-giant-component */
import {
  useState,
  useCallback,
  useRef,
  useEffect,
  useMemo,
  type RefObject,
} from 'react'

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
  Viewport,
} from './types.js'
import { useCanvasViewport, computeGridStyle } from './useCanvasViewport.js'
import { useVisibleNodes, type ContainerSize } from './useVisibleNodes.js'
import {
  useCanvasSelection,
  type UseCanvasSelectionResult,
} from './useCanvasSelection.js'
import {
  useCanvasActions,
  type UseCanvasActionsResult,
} from './useCanvasActions.js'
import { useCanvasCommands } from './useCanvasCommands.js'
import { CanvasNode } from './CanvasNode.js'
import { CanvasEdgeLayer } from './CanvasEdgeLayer.js'
import { CanvasDrawingLayer } from './CanvasDrawingLayer.js'
import { CanvasTextNode } from './CanvasTextNode.js'
import { CanvasToolbar } from './CanvasToolbar.js'
import {
  CanvasContextMenu,
  type CanvasContextMenuState,
} from './CanvasContextMenu.js'
import {
  CanvasMinimap,
  DEFAULT_MINIMAP_WIDTH,
  DEFAULT_MINIMAP_HEIGHT,
} from './CanvasMinimap.js'
import { CanvasSelectionOverlay } from './CanvasSelectionOverlay.js'
import { CanvasScaleIndicator } from './CanvasScaleIndicator.js'
import { CanvasSyncStatus } from './CanvasSyncStatus.js'
import { DEFAULT_CANVAS_COLOR, type CanvasGeometryKind } from './geometry.js'

// XL_BREAKPOINT is the container width threshold for the xl tailwind breakpoint.
const XL_BREAKPOINT = 1280

// DEFAULT_TEXT_NODE_WIDTH is the default width for new text nodes.
const DEFAULT_TEXT_NODE_WIDTH = 200

// MIN_TEXT_NODE_HEIGHT is the minimum height for a text node (single line + padding).
const MIN_TEXT_NODE_HEIGHT = 32

// PENDING_OBJECT_INSERT_TTL_MS bounds how long a context-menu placement hint
// stays valid while the user picks an object from the command palette.
const PENDING_OBJECT_INSERT_TTL_MS = 5000

// MIN_OBJECT_DRAG_SIZE is the smallest dragged rectangle that creates an object.
const MIN_OBJECT_DRAG_SIZE = 20

interface Point {
  x: number
  y: number
}

interface PendingObjectInsert extends Point {
  createdAt: number
}

interface ObjectDragRect {
  x: number
  y: number
  w: number
  h: number
}

type CanvasNodeMap = CanvasStateData['nodes']

/** generateNodeId creates a unique node ID. */
function generateNodeId(): string {
  return `node-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
}

/** drawingKindFor maps a drawing tool to its geometry kind, or null for other tools. */
function drawingKindFor(tool: CanvasTool): CanvasGeometryKind | null {
  switch (tool) {
    case 'draw':
      return 'pen'
    case 'line':
    case 'arrow':
    case 'rectangle':
    case 'ellipse':
      return tool
    default:
      return null
  }
}

/** useContainerSize observes the content size of the element. */
function useContainerSize(ref: RefObject<HTMLElement | null>): ContainerSize {
  const [containerSize, setContainerSize] = useState<ContainerSize>({
    width: 0,
    height: 0,
  })

  useEffect(() => {
    const el = ref.current
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
  }, [ref])

  return containerSize
}

interface UseFocusNodeParams {
  focusNodeId: string | null | undefined
  nodes: CanvasNodeMap
  selection: UseCanvasSelectionResult
  view: { scale: number; setViewport: (v: Viewport) => void }
  containerSize: ContainerSize
}

/** useFocusNode selects the focus node and centers the viewport on it. */
function useFocusNode({
  focusNodeId,
  nodes,
  selection,
  view,
  containerSize,
}: UseFocusNodeParams) {
  useEffect(() => {
    if (!focusNodeId) return
    const node = nodes.get(focusNodeId)
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
    view.setViewport({
      x: containerSize.width / 2 - cx * view.scale,
      y: containerSize.height / 2 - cy * view.scale,
      scale: view.scale,
    })
  }, [
    focusNodeId,
    selection,
    nodes,
    selection.selectedNodeIds,
    selection.focus,
    selection.setSelection,
    selection.setFocus,
    containerSize,
    view,
  ])
}

/**
 * usePendingText holds the ephemeral text editor shown after a text tool
 * click, and commits its content as a new text node.
 */
function usePendingText(
  callbacks: CanvasCallbacks,
  selection: UseCanvasSelectionResult,
) {
  const [pendingText, setPendingText] = useState<Point | null>(null)
  const pendingTextRef = useRef<HTMLDivElement | null>(null)

  const commitPendingText = useCallback(
    (content: string) => {
      if (!pendingText) return
      const el = pendingTextRef.current
      const id = generateNodeId()
      const w = el
        ? Math.max(el.scrollWidth + 4, DEFAULT_TEXT_NODE_WIDTH)
        : DEFAULT_TEXT_NODE_WIDTH
      const h = el
        ? Math.max(el.scrollHeight + 4, MIN_TEXT_NODE_HEIGHT)
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

  const cancelPendingText = useCallback(() => {
    setPendingText(null)
  }, [])

  return {
    pendingText,
    pendingTextRef,
    setPendingText,
    commitPendingText,
    cancelPendingText,
  }
}

/**
 * useNodeDrag accumulates local drag offsets while nodes move and persists
 * them when the drag ends.
 */
function useNodeDrag(
  nodes: CanvasNodeMap,
  selection: UseCanvasSelectionResult,
  callbacks: CanvasCallbacks,
) {
  const [dragOffsets, setDragOffsets] = useState<Map<
    string,
    { dx: number; dy: number }
  > | null>(null)

  // Merge server state with local drag offsets.
  const effectiveNodes = useMemo(() => {
    if (!dragOffsets) return nodes
    const merged = new Map(nodes)
    for (const [id, offset] of dragOffsets) {
      const n = merged.get(id)
      if (n) {
        merged.set(id, { ...n, x: n.x + offset.dx, y: n.y + offset.dy })
      }
    }
    return merged
  }, [nodes, dragOffsets])

  const cancelDrag = useCallback(() => {
    setDragOffsets(null)
  }, [])

  const handleNodeMove = useCallback(
    (id: string, dx: number, dy: number) => {
      // Accumulate local drag offsets without firing RPC.
      const idsToMove = selection.selectedNodeIds.has(id)
        ? selection.selectedNodeIds
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
    const next: CanvasNodeMap = new Map()
    for (const [id, offset] of dragOffsets) {
      const n = nodes.get(id)
      if (n) {
        next.set(id, { ...n, x: n.x + offset.dx, y: n.y + offset.dy })
      }
    }
    setDragOffsets(null)
    if (next.size > 0) {
      callbacks.onNodesChange?.(next)
    }
  }, [dragOffsets, nodes, callbacks])

  return { effectiveNodes, cancelDrag, handleNodeMove, handleNodeMoveEnd }
}

/**
 * useCanvasCoords converts pointer and container positions into canvas space
 * for the current viewport.
 */
function useCanvasCoords(
  containerRef: RefObject<HTMLElement | null>,
  viewport: Viewport,
) {
  const screenToCanvas = useCallback(
    (sx: number, sy: number): Point => ({
      x: (sx - viewport.x) / viewport.scale,
      y: (sy - viewport.y) / viewport.scale,
    }),
    [viewport],
  )

  // clientToCanvas converts client coords, or returns null before mount.
  const clientToCanvas = useCallback(
    (clientX: number, clientY: number): Point | null => {
      const rect = containerRef.current?.getBoundingClientRect()
      if (!rect) return null
      return screenToCanvas(clientX - rect.left, clientY - rect.top)
    },
    [containerRef, screenToCanvas],
  )

  const getViewportCenter = useCallback((): Point => {
    const rect = containerRef.current?.getBoundingClientRect()
    if (!rect) return { x: 0, y: 0 }
    return screenToCanvas(rect.width / 2, rect.height / 2)
  }, [containerRef, screenToCanvas])

  return { clientToCanvas, getViewportCenter }
}

interface UseCanvasInsertionParams {
  callbacks: CanvasCallbacks
  selection: UseCanvasSelectionResult
  imageObjectKey: string | undefined
  getViewportCenter: () => Point
  startPendingText: (point: Point) => void
}

/**
 * useCanvasInsertion adds text, pinned objects, and images to the canvas, and
 * opens the command palette pickers that choose them.
 */
function useCanvasInsertion({
  callbacks,
  selection,
  imageObjectKey,
  getViewportCenter,
  startPendingText,
}: UseCanvasInsertionParams) {
  const openCommand = useOpenCommand()
  const pendingObjectInsertRef = useRef<PendingObjectInsert | null>(null)

  const addTextAt = useCallback(
    (point?: Point) => {
      startPendingText(point ?? getViewportCenter())
    },
    [startPendingText, getViewportCenter],
  )

  const resolvePendingObjectInsertPoint = useCallback((): Point => {
    const pending = pendingObjectInsertRef.current
    pendingObjectInsertRef.current = null
    if (!pending) return getViewportCenter()
    if (Date.now() - pending.createdAt > PENDING_OBJECT_INSERT_TTL_MS) {
      return getViewportCenter()
    }
    return { x: pending.x, y: pending.y }
  }, [getViewportCenter])

  const addObjectAt = useCallback(
    (objectKey: string) => {
      const point = resolvePendingObjectInsertPoint()
      callbacks.onPinObject?.(objectKey, point.x, point.y)
    },
    [callbacks, resolvePendingObjectInsertPoint],
  )

  const addImageAt = useCallback(
    (path: string) => {
      if (!imageObjectKey) return
      const point = getViewportCenter()
      const id = generateNodeId()
      callbacks.onNodesChange?.(
        new Map([
          [
            id,
            {
              id,
              x: point.x - 200,
              y: point.y - 150,
              width: 400,
              height: 300,
              zIndex: 0,
              type: 'world_object',
              objectKey: imageObjectKey,
              viewPath: path,
              pinned: true,
            },
          ],
        ]),
      )
      selection.toggleSelect(id, false)
    },
    [callbacks, getViewportCenter, imageObjectKey, selection],
  )

  // requestObjectPicker opens the object picker. A point pins the chosen
  // object there; without one it lands at the current viewport center.
  const requestObjectPicker = useCallback(
    (point: Point | null) => {
      pendingObjectInsertRef.current = point
        ? { ...point, createdAt: Date.now() }
        : null
      openCommand('canvas.add-object')
    },
    [openCommand],
  )

  const requestImagePicker = useCallback(() => {
    openCommand('canvas.add-image')
  }, [openCommand])

  return {
    addTextAt,
    addObjectAt,
    addImageAt,
    requestObjectPicker,
    requestImagePicker,
  }
}

interface UseCanvasContextMenuParams {
  actions: UseCanvasActionsResult['actions']
  clientToCanvas: (clientX: number, clientY: number) => Point | null
  addTextAt: (point?: Point) => void
  requestObjectPicker: (point: Point | null) => void
}

/**
 * useCanvasContextMenu tracks the background context menu and the canvas
 * position it was opened at.
 */
function useCanvasContextMenu({
  actions,
  clientToCanvas,
  addTextAt,
  requestObjectPicker,
}: UseCanvasContextMenuParams) {
  const [menuState, setMenuState] = useState<CanvasContextMenuState | null>(
    null,
  )
  const canvasPositionRef = useRef<Point | null>(null)

  const closeContextMenu = useCallback(() => {
    setMenuState(null)
  }, [])

  const handleBackgroundContextMenu = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => {
      const target = e.target as HTMLElement
      if (target.closest('[data-canvas-node]')) return
      const canvasPosition = clientToCanvas(e.clientX, e.clientY)
      if (!canvasPosition) return
      e.preventDefault()
      canvasPositionRef.current = canvasPosition
      setMenuState({ position: { x: e.clientX, y: e.clientY } })
    },
    [clientToCanvas],
  )

  // takeCanvasPosition returns the position the menu opened at, once.
  const takeCanvasPosition = useCallback((): Point | null => {
    const point = canvasPositionRef.current
    canvasPositionRef.current = null
    return point
  }, [])

  const handleAddText = useCallback(() => {
    const point = takeCanvasPosition()
    if (!point) return
    closeContextMenu()
    addTextAt(point)
  }, [takeCanvasPosition, closeContextMenu, addTextAt])

  const handleAddObject = useCallback(() => {
    const point = takeCanvasPosition()
    if (!point) return
    closeContextMenu()
    requestObjectPicker(point)
  }, [takeCanvasPosition, closeContextMenu, requestObjectPicker])

  // closeThen closes the menu and then runs the canvas action.
  const closeThen = useCallback(
    (run: () => void) => () => {
      closeContextMenu()
      run()
    },
    [closeContextMenu],
  )

  const menuProps = {
    state: menuState,
    onClose: closeContextMenu,
    onPaste: closeThen(actions.paste),
    onAddText: handleAddText,
    onAddObject: handleAddObject,
    onFitView: closeThen(actions['fit-view']),
    onZoomReset: closeThen(actions['zoom-reset']),
    onSelectAll: closeThen(actions['select-all']),
  }

  return { menuProps, closeContextMenu, handleBackgroundContextMenu }
}

interface UseBackgroundInputParams {
  tool: CanvasTool
  selection: UseCanvasSelectionResult
  clientToCanvas: (clientX: number, clientY: number) => Point | null
  startPendingText: (point: Point) => void
  addTextAt: () => void
}

/** useBackgroundInput handles clicks and keys on the canvas background. */
function useBackgroundInput({
  tool,
  selection,
  clientToCanvas,
  startPendingText,
  addTextAt,
}: UseBackgroundInputParams) {
  const handleBackgroundClick = useCallback(
    (e: React.MouseEvent) => {
      const target = e.target as HTMLElement
      if (target.closest('[data-canvas-node]')) return

      if (tool === 'text') {
        const pt = clientToCanvas(e.clientX, e.clientY)
        if (pt) startPendingText(pt)
        return
      }

      selection.clearSelection()
    },
    [tool, selection, clientToCanvas, startPendingText],
  )

  const handleBackgroundKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLDivElement>) => {
      if (e.currentTarget !== e.target) return
      if (e.key === 'Escape') {
        e.preventDefault()
        selection.clearSelection()
        return
      }
      if (tool !== 'text') return
      if (e.key !== 'Enter' && e.key !== ' ') return
      e.preventDefault()
      addTextAt()
    },
    [tool, selection, addTextAt],
  )

  return { handleBackgroundClick, handleBackgroundKeyDown }
}

interface UseObjectDragParams {
  tool: CanvasTool
  callbacks: CanvasCallbacks
  selection: UseCanvasSelectionResult
  clientToCanvas: (clientX: number, clientY: number) => Point | null
  selectTool: () => void
}

/** useObjectDrag creates a world object node from a drag on the background. */
function useObjectDrag({
  tool,
  callbacks,
  selection,
  clientToCanvas,
  selectTool,
}: UseObjectDragParams) {
  const dragStartRef = useRef<Point | null>(null)
  const [dragRect, setDragRect] = useState<ObjectDragRect | null>(null)

  const handlePointerDown = useCallback(
    (e: React.PointerEvent) => {
      if (tool !== 'object') return
      const target = e.target as HTMLElement
      if (target.closest('[data-canvas-node]')) return
      const pt = clientToCanvas(e.clientX, e.clientY)
      if (!pt) return
      dragStartRef.current = pt
      setDragRect({ x: pt.x, y: pt.y, w: 0, h: 0 })
      target.setPointerCapture(e.pointerId)
      e.preventDefault()
    },
    [tool, clientToCanvas],
  )

  const handlePointerMove = useCallback(
    (e: React.PointerEvent) => {
      const start = dragStartRef.current
      if (!start) return
      const pt = clientToCanvas(e.clientX, e.clientY)
      if (!pt) return
      setDragRect({
        x: Math.min(start.x, pt.x),
        y: Math.min(start.y, pt.y),
        w: Math.abs(pt.x - start.x),
        h: Math.abs(pt.y - start.y),
      })
    },
    [clientToCanvas],
  )

  const handlePointerUp = useCallback(() => {
    const r = dragStartRef.current ? dragRect : null
    dragStartRef.current = null
    setDragRect(null)
    if (!r || r.w < MIN_OBJECT_DRAG_SIZE || r.h < MIN_OBJECT_DRAG_SIZE) return
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
    selectTool()
  }, [dragRect, callbacks, selection, selectTool])

  return { dragRect, handlePointerDown, handlePointerMove, handlePointerUp }
}

interface CanvasPendingTextProps {
  pendingText: Point
  innerRef: RefObject<HTMLDivElement | null>
  onCommit: (content: string) => void
  onCancel: () => void
}

/** CanvasPendingText renders the text editor shown at a text tool click. */
function CanvasPendingText({
  pendingText,
  innerRef,
  onCommit,
  onCancel,
}: CanvasPendingTextProps) {
  return (
    <div
      role="presentation"
      ref={innerRef}
      style={{
        '--canvas-pending-left': `${pendingText.x - DEFAULT_TEXT_NODE_WIDTH / 2}px`,
        '--canvas-pending-top': `${pendingText.y - MIN_TEXT_NODE_HEIGHT / 2}px`,
        '--canvas-pending-width': `${DEFAULT_TEXT_NODE_WIDTH}px`,
        '--canvas-pending-min-height': `${MIN_TEXT_NODE_HEIGHT}px`,
      }}
      className="canvas-pending-text bg-background-card/30 text-card-foreground pointer-events-auto rounded-lg backdrop-blur-sm"
      onPointerDown={(e) => e.stopPropagation()}
      onClick={(e) => e.stopPropagation()}
    >
      <CanvasTextNode
        content=""
        autoEdit
        onChange={onCommit}
        onCancel={onCancel}
      />
    </div>
  )
}

interface CanvasObjectDragRectProps {
  rect: ObjectDragRect
  viewport: Viewport
}

/** CanvasObjectDragRect renders the rectangle drawn by the object tool. */
function CanvasObjectDragRect({ rect, viewport }: CanvasObjectDragRectProps) {
  return (
    <div
      className="canvas-drag-rectangle border-brand/30 bg-brand/5 pointer-events-none absolute rounded-lg border border-dashed"
      style={{
        '--canvas-drag-left': `${rect.x * viewport.scale + viewport.x}px`,
        '--canvas-drag-top': `${rect.y * viewport.scale + viewport.y}px`,
        '--canvas-drag-width': `${rect.w * viewport.scale}px`,
        '--canvas-drag-height': `${rect.h * viewport.scale}px`,
      }}
    />
  )
}

/** CanvasGridLayer renders the background grid driven by the viewport. */
function CanvasGridLayer({
  viewport,
  layerRef,
}: {
  viewport: Viewport
  layerRef: RefObject<HTMLDivElement | null>
}) {
  // Transform and grid styles are initial values for React rendering.
  // During gestures, useCanvasViewport applies these directly to the
  // DOM via transformLayerRef/gridLayerRef for zero-cost panning.
  const gridStyle = useMemo(() => computeGridStyle(viewport), [viewport])

  return (
    <div
      ref={layerRef}
      className="canvas-grid pointer-events-none absolute inset-0"
      style={{
        '--canvas-grid-color': gridStyle.backgroundColor,
        '--canvas-grid-image': gridStyle.backgroundImage,
        '--canvas-grid-size': gridStyle.backgroundSize,
        '--canvas-grid-position': gridStyle.backgroundPosition,
        '--canvas-grid-opacity': gridStyle.opacity,
      }}
    />
  )
}

interface CanvasMinimapPanelProps {
  nodes: CanvasNodeMap
  viewport: Viewport
  containerSize: ContainerSize
  onViewportChange: (v: Viewport) => void
}

/** CanvasMinimapPanel sizes the minimap by the container width. */
function CanvasMinimapPanel({
  nodes,
  viewport,
  containerSize,
  onViewportChange,
}: CanvasMinimapPanelProps) {
  const divisor = containerSize.width >= XL_BREAKPOINT ? 1 : 2

  return (
    <CanvasMinimap
      nodes={nodes}
      viewport={viewport}
      containerSize={containerSize}
      onViewportChange={onViewportChange}
      width={DEFAULT_MINIMAP_WIDTH / divisor}
      height={DEFAULT_MINIMAP_HEIGHT / divisor}
    />
  )
}

// CanvasProps are the props for the Canvas component.
interface CanvasProps {
  state: CanvasStateData
  ephemeralEdges?: EphemeralEdge[]
  tool?: CanvasTool
  callbacks: CanvasCallbacks
  pendingMutations?: number
  objectSubItems?: SubItemsCallback
  imageObjectKey?: string
  imageSubItems?: SubItemsCallback
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
  imageObjectKey,
  imageSubItems,
  focusNodeId,
  className,
}: CanvasProps) {
  const [toolInternal, setToolInternal] = useState<CanvasTool>('select')
  const [drawingColor, setDrawingColor] = useState(DEFAULT_CANVAS_COLOR)
  const tool = toolProp ?? toolInternal
  const onToolChange = toolProp === undefined ? setToolInternal : undefined
  const selectTool = useCallback(() => setToolInternal('select'), [])

  const viewportContainerRef = useRef<HTMLDivElement | null>(null)
  const viewportRef = useRef({ x: 0, y: 0, scale: 1 })
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

  const view = useMemo(
    () => ({ scale: viewport.scale, setViewport }),
    [viewport.scale, setViewport],
  )
  const containerSize = useContainerSize(viewportContainerRef)
  useFocusNode({
    focusNodeId,
    nodes: state.nodes,
    selection,
    view,
    containerSize,
  })

  const {
    pendingText,
    pendingTextRef,
    setPendingText,
    commitPendingText,
    cancelPendingText,
  } = usePendingText(callbacks, selection)
  const { effectiveNodes, cancelDrag, handleNodeMove, handleNodeMoveEnd } =
    useNodeDrag(state.nodes, selection, callbacks)
  const visibleNodeIds = useVisibleNodes(
    effectiveNodes,
    viewport,
    containerSize,
  )

  const { actions, moveSelected } = useCanvasActions({
    selection,
    nodes: state.nodes,
    layoutMetadata: state.layoutMetadata,
    callbacks,
    viewport,
    setViewport,
    containerSize,
  })

  // Notify consumer of selection changes.
  useEffect(() => {
    callbacks.onNodeSelect?.(selection.selectedNodeIds)
  }, [selection.selectedNodeIds, callbacks])

  const handleNodeResize = useCallback(
    (id: string, resized: CanvasNodeData) => {
      callbacks.onNodesChange?.(new Map([[id, resized]]))
    },
    [callbacks],
  )

  const startPendingText = useCallback(
    (point: Point) => {
      setPendingText(point)
      selectTool()
    },
    [setPendingText, selectTool],
  )
  const { clientToCanvas, getViewportCenter } = useCanvasCoords(
    viewportContainerRef,
    viewport,
  )
  const {
    addTextAt,
    addObjectAt,
    addImageAt,
    requestObjectPicker,
    requestImagePicker,
  } = useCanvasInsertion({
    callbacks,
    selection,
    imageObjectKey,
    getViewportCenter,
    startPendingText,
  })
  const handleCommandAddText = useCallback(() => {
    addTextAt()
  }, [addTextAt])

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
    onAddImage: addImageAt,
    addImageSubItems: imageSubItems,
  })

  const canAddObject = !!callbacks.onPinObject && !!objectSubItems
  const canAddImage = !!imageObjectKey && !!imageSubItems
  const { menuProps, closeContextMenu, handleBackgroundContextMenu } =
    useCanvasContextMenu({
      actions,
      clientToCanvas,
      addTextAt,
      requestObjectPicker,
    })
  const { handleBackgroundClick, handleBackgroundKeyDown } = useBackgroundInput(
    {
      tool,
      selection,
      clientToCanvas,
      startPendingText,
      addTextAt: handleCommandAddText,
    },
  )
  const objectDrag = useObjectDrag({
    tool,
    callbacks,
    selection,
    clientToCanvas,
    selectTool,
  })

  const { handlePointerDown: handleObjectPointerDown } = objectDrag
  const handleBackgroundPointerDown = useCallback(
    (e: React.PointerEvent) => {
      closeContextMenu()
      handleObjectPointerDown(e)
    },
    [closeContextMenu, handleObjectPointerDown],
  )

  const nodeEntries = useMemo(
    () => Array.from(effectiveNodes),
    [effectiveNodes],
  )

  const handleStrokeComplete = useCallback(
    (node: CanvasNodeData) => {
      callbacks.onNodesChange?.(new Map([[node.id, node]]))
    },
    [callbacks],
  )

  const drawingKind = drawingKindFor(tool)

  return (
    <div className={cn('flex h-full outline-none', className)}>
      <CanvasToolbar
        tool={tool}
        color={drawingColor}
        onToolChange={onToolChange ?? (() => {})}
        onColorChange={setDrawingColor}
        actions={actions}
        onAddObject={canAddObject ? () => requestObjectPicker(null) : undefined}
        onAddImage={canAddImage ? requestImagePicker : undefined}
      />
      <div
        ref={viewportContainerRef}
        data-testid="canvas-viewport"
        role="application"
        aria-label="Canvas"
        className={cn(
          'relative flex-1 touch-none overflow-hidden bg-background-canvas outline-none',
          (tool === 'text' || tool === 'object') && 'cursor-crosshair',
        )}
        onClick={handleBackgroundClick}
        onContextMenu={handleBackgroundContextMenu}
        onPointerDown={handleBackgroundPointerDown}
        onPointerMove={objectDrag.handlePointerMove}
        onPointerUp={objectDrag.handlePointerUp}
        onKeyDown={handleBackgroundKeyDown}
        tabIndex={0}
      >
        <CanvasGridLayer viewport={viewport} layerRef={gridLayerRef} />
        {/* Gesture layer for viewport pan/drag-select. Sits below the
            transform layer so canvas nodes receive pointer events directly
            without viewport gesture interference. */}
        <div ref={gestureLayerRef} className="absolute inset-0 touch-none" />
        <div
          ref={transformLayerRef}
          className="canvas-transform pointer-events-none"
          style={{
            '--canvas-transform': `translate3d(${viewport.x}px, ${viewport.y}px, 0) scale(${viewport.scale})`,
            '--canvas-transform-origin': '0 0',
          }}
        >
          <CanvasEdgeLayer
            edges={state.edges}
            ephemeralEdges={ephemeralEdges}
            nodes={effectiveNodes}
            callbacks={callbacks}
          />
          {nodeEntries.map(([id, node]) => (
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
            <CanvasPendingText
              pendingText={pendingText}
              innerRef={pendingTextRef}
              onCommit={commitPendingText}
              onCancel={cancelPendingText}
            />
          )}
        </div>
        <CanvasDrawingLayer
          visible={drawingKind !== null}
          viewport={viewport}
          kind={drawingKind ?? 'pen'}
          color={drawingColor}
          onStrokeComplete={handleStrokeComplete}
        />
        {objectDrag.dragRect && (
          <CanvasObjectDragRect
            rect={objectDrag.dragRect}
            viewport={viewport}
          />
        )}
        <CanvasSelectionOverlay dragRect={selection.dragRect} />
        <CanvasMinimapPanel
          nodes={state.nodes}
          viewport={viewport}
          containerSize={containerSize}
          onViewportChange={setViewport}
        />
        <CanvasScaleIndicator scale={viewport.scale} />
        {pendingMutations !== undefined && (
          <CanvasSyncStatus pending={pendingMutations} />
        )}
      </div>
      <CanvasContextMenu canAddObject={canAddObject} {...menuProps} />
    </div>
  )
}
