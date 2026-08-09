import { useState, useCallback, useMemo } from 'react'

import type { CanvasNodeData } from './types.js'

export interface DragRect {
  startX: number
  startY: number
  endX: number
  endY: number
}

export type SelectionFocus = 'content' | 'border'

export interface UseCanvasSelectionResult {
  selectedNodeIds: Set<string>
  focus: SelectionFocus
  dragRect: DragRect | null
  toggleSelect: (id: string, shift: boolean) => void
  selectWithFocus: (id: string, shift: boolean, focus: SelectionFocus) => void
  selectAll: (nodes: Map<string, CanvasNodeData>) => void
  clearSelection: () => void
  setSelection: (ids: Set<string>) => void
  setFocus: (focus: SelectionFocus) => void
  startDragRect: (x: number, y: number) => void
  updateDragRect: (x: number, y: number) => void
  endDragRect: (
    nodes: Map<string, CanvasNodeData>,
    viewportX: number,
    viewportY: number,
    scale: number,
  ) => void
}

export function useCanvasSelection(): UseCanvasSelectionResult {
  const [selectedNodeIds, setSelectedNodeIds] = useState<Set<string>>(
    () => new Set(),
  )
  const [focus, setFocus] = useState<SelectionFocus>('border')
  const [dragRect, setDragRect] = useState<DragRect | null>(null)

  const toggleSelect = useCallback((id: string, shift: boolean) => {
    setSelectedNodeIds((prev) => {
      const next = new Set(shift ? prev : undefined)
      if (prev.has(id) && shift) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
    setFocus('border')
  }, [])

  const selectWithFocus = useCallback(
    (id: string, shift: boolean, f: SelectionFocus) => {
      setSelectedNodeIds((prev) => {
        // Return the same reference when selection wouldn't change. This
        // prevents unnecessary re-renders that destroy DOM elements between
        // the first and second click, breaking native dblclick generation.
        if (!shift && prev.size === 1 && prev.has(id)) return prev
        const next = new Set(shift ? prev : undefined)
        if (prev.has(id) && shift) {
          next.delete(id)
        } else {
          next.add(id)
        }
        return next
      })
      setFocus(f)
    },
    [],
  )

  const selectAll = useCallback((nodes: Map<string, CanvasNodeData>) => {
    setSelectedNodeIds(new Set(nodes.keys()))
  }, [])

  const clearSelection = useCallback(() => {
    setSelectedNodeIds(new Set())
  }, [])

  const setSelection = useCallback((ids: Set<string>) => {
    setSelectedNodeIds(ids)
  }, [])

  const startDragRect = useCallback((x: number, y: number) => {
    setDragRect({ startX: x, startY: y, endX: x, endY: y })
  }, [])

  const updateDragRect = useCallback((x: number, y: number) => {
    setDragRect((prev) => (prev ? { ...prev, endX: x, endY: y } : null))
  }, [])

  const endDragRect = useCallback(
    (
      nodes: Map<string, CanvasNodeData>,
      viewportX: number,
      viewportY: number,
      scale: number,
    ) => {
      const rect = dragRect
      if (!rect) {
        setDragRect(null)
        return
      }

      const left = Math.min(rect.startX, rect.endX)
      const top = Math.min(rect.startY, rect.endY)
      const right = Math.max(rect.startX, rect.endX)
      const bottom = Math.max(rect.startY, rect.endY)

      const canvasLeft = (left - viewportX) / scale
      const canvasTop = (top - viewportY) / scale
      const canvasRight = (right - viewportX) / scale
      const canvasBottom = (bottom - viewportY) / scale

      const selected = new Set<string>()
      for (const [id, node] of nodes) {
        const nodeRight = node.x + node.width
        const nodeBottom = node.y + node.height

        if (
          node.x <= canvasRight &&
          nodeRight >= canvasLeft &&
          node.y <= canvasBottom &&
          nodeBottom >= canvasTop
        ) {
          selected.add(id)
        }
      }

      setSelectedNodeIds(selected)
      setDragRect(null)
    },
    [dragRect],
  )

  return useMemo(
    () => ({
      selectedNodeIds,
      focus,
      dragRect,
      toggleSelect,
      selectWithFocus,
      selectAll,
      clearSelection,
      setSelection,
      setFocus,
      startDragRect,
      updateDragRect,
      endDragRect,
    }),
    [
      selectedNodeIds,
      focus,
      dragRect,
      toggleSelect,
      selectWithFocus,
      selectAll,
      clearSelection,
      setSelection,
      setFocus,
      startDragRect,
      updateDragRect,
      endDragRect,
    ],
  )
}
