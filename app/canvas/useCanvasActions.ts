import { useCallback, useMemo } from 'react'

import type {
  CanvasAction,
  CanvasCallbacks,
  CanvasNodeData,
  Viewport,
} from './types.js'
import { MIN_SCALE, MAX_SCALE, ZOOM_STEPS } from './types.js'
import type { UseCanvasSelectionResult } from './useCanvasSelection.js'
import type { ContainerSize } from './useVisibleNodes.js'

// UseCanvasActionsParams are the parameters for useCanvasActions.
interface UseCanvasActionsParams {
  selection: UseCanvasSelectionResult
  nodes: Map<string, CanvasNodeData>
  callbacks: CanvasCallbacks
  viewport: Viewport
  setViewport: (v: Viewport) => void
  containerSize: ContainerSize
}

// UseCanvasActionsResult is the return type of useCanvasActions.
export interface UseCanvasActionsResult {
  actions: Record<CanvasAction, () => void>
  moveSelected: (dx: number, dy: number) => void
}

// useCanvasActions provides an action map for canvas operations.
export function useCanvasActions(
  params: UseCanvasActionsParams,
): UseCanvasActionsResult {
  const { selection, nodes, callbacks, viewport, setViewport, containerSize } =
    params

  const deleteSelected = useCallback(() => {
    if (selection.selectedNodeIds.size === 0) return
    const ids = Array.from(selection.selectedNodeIds)
    selection.clearSelection()
    callbacks.onNodesRemove?.(ids)
  }, [selection, callbacks])

  const copy = useCallback(() => {
    // Copy selected node IDs to a custom clipboard format on the global clipboard.
    const ids = Array.from(selection.selectedNodeIds)
    if (ids.length === 0) return
    const data = JSON.stringify(ids.map((id) => nodes.get(id)).filter(Boolean))
    navigator.clipboard.writeText(data).catch(() => {
      // Clipboard write may fail in some environments.
    })
  }, [selection.selectedNodeIds, nodes])

  const paste = useCallback(() => {
    navigator.clipboard
      .readText()
      .then((text) => {
        try {
          const parsed = JSON.parse(text) as CanvasNodeData[]
          if (!Array.isArray(parsed)) return
          const newNodes = new Map<string, CanvasNodeData>()
          const newIds = new Set<string>()
          for (const node of parsed) {
            const id = `${node.id}-copy-${Date.now()}`
            newNodes.set(id, { ...node, id, x: node.x + 20, y: node.y + 20 })
            newIds.add(id)
          }
          callbacks.onNodesChange?.(newNodes)
          selection.setSelection(newIds)
        } catch {
          // Ignore non-JSON clipboard content.
        }
      })
      .catch(() => {
        // Clipboard read may fail in some environments.
      })
  }, [callbacks, selection])

  const selectAll = useCallback(() => {
    selection.selectAll(nodes)
  }, [selection, nodes])

  const deselect = useCallback(() => {
    selection.clearSelection()
  }, [selection])

  const zoomIn = useCallback(() => {
    const next = ZOOM_STEPS.find((s) => s > viewport.scale + 0.001) ?? MAX_SCALE
    const cx = containerSize.width / 2
    const cy = containerSize.height / 2
    setViewport({
      x: cx - ((cx - viewport.x) / viewport.scale) * next,
      y: cy - ((cy - viewport.y) / viewport.scale) * next,
      scale: next,
    })
  }, [viewport, setViewport, containerSize])

  const zoomOut = useCallback(() => {
    const prev =
      [...ZOOM_STEPS].reverse().find((s) => s < viewport.scale - 0.001) ??
      MIN_SCALE
    const cx = containerSize.width / 2
    const cy = containerSize.height / 2
    setViewport({
      x: cx - ((cx - viewport.x) / viewport.scale) * prev,
      y: cy - ((cy - viewport.y) / viewport.scale) * prev,
      scale: prev,
    })
  }, [viewport, setViewport, containerSize])

  const zoomReset = useCallback(() => {
    const cx = containerSize.width / 2
    const cy = containerSize.height / 2
    setViewport({
      x: cx - (cx - viewport.x) / viewport.scale,
      y: cy - (cy - viewport.y) / viewport.scale,
      scale: 1,
    })
  }, [viewport, setViewport, containerSize])

  const fitView = useCallback(() => {
    if (nodes.size === 0) {
      setViewport({ x: 0, y: 0, scale: 1 })
      return
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

    const contentWidth = maxX - minX
    const contentHeight = maxY - minY
    const padding = 40

    const scaleX = (containerSize.width - padding * 2) / contentWidth
    const scaleY = (containerSize.height - padding * 2) / contentHeight
    const scale = Math.min(
      Math.max(MIN_SCALE, Math.min(scaleX, scaleY)),
      MAX_SCALE,
    )

    const cx = (minX + maxX) / 2
    const cy = (minY + maxY) / 2
    setViewport({
      x: containerSize.width / 2 - cx * scale,
      y: containerSize.height / 2 - cy * scale,
      scale,
    })
  }, [nodes, setViewport, containerSize])

  const bringToFront = useCallback(() => {
    if (selection.selectedNodeIds.size === 0) return
    let maxZ = 0
    for (const node of nodes.values()) {
      if (node.zIndex > maxZ) maxZ = node.zIndex
    }
    const changed = new Map<string, CanvasNodeData>()
    for (const id of selection.selectedNodeIds) {
      const node = nodes.get(id)
      if (node) {
        changed.set(id, { ...node, zIndex: maxZ + 1 })
      }
    }
    if (changed.size > 0) callbacks.onNodesChange?.(changed)
  }, [selection.selectedNodeIds, nodes, callbacks])

  const sendToBack = useCallback(() => {
    if (selection.selectedNodeIds.size === 0) return
    let minZ = 0
    for (const node of nodes.values()) {
      if (node.zIndex < minZ) minZ = node.zIndex
    }
    const changed = new Map<string, CanvasNodeData>()
    for (const id of selection.selectedNodeIds) {
      const node = nodes.get(id)
      if (node) {
        changed.set(id, { ...node, zIndex: minZ - 1 })
      }
    }
    if (changed.size > 0) callbacks.onNodesChange?.(changed)
  }, [selection.selectedNodeIds, nodes, callbacks])

  const actions: Record<CanvasAction, () => void> = useMemo(
    () => ({
      delete: deleteSelected,
      copy,
      paste,
      // Undo/redo are no-ops until a history system is added.
      undo: () => {},
      redo: () => {},
      'select-all': selectAll,
      deselect,
      'zoom-in': zoomIn,
      'zoom-out': zoomOut,
      'zoom-reset': zoomReset,
      'fit-view': fitView,
      'bring-to-front': bringToFront,
      'send-to-back': sendToBack,
    }),
    [
      deleteSelected,
      copy,
      paste,
      selectAll,
      deselect,
      zoomIn,
      zoomOut,
      zoomReset,
      fitView,
      bringToFront,
      sendToBack,
    ],
  )

  const moveSelected = useCallback(
    (dx: number, dy: number) => {
      if (selection.selectedNodeIds.size === 0) return
      const changed = new Map<string, CanvasNodeData>()
      for (const id of selection.selectedNodeIds) {
        const node = nodes.get(id)
        if (node) {
          changed.set(id, { ...node, x: node.x + dx, y: node.y + dy })
        }
      }
      if (changed.size > 0) callbacks.onNodesChange?.(changed)
    },
    [selection.selectedNodeIds, nodes, callbacks],
  )

  return useMemo(() => ({ actions, moveSelected }), [actions, moveSelected])
}
