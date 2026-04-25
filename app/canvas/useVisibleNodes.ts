import { useMemo } from 'react'

import type { CanvasNodeData, Viewport } from './types.js'
import { VIEWPORT_MARGIN } from './types.js'

// ContainerSize represents the pixel dimensions of the canvas container.
export interface ContainerSize {
  width: number
  height: number
}

// useVisibleNodes returns the set of node IDs that are visible in the current viewport.
export function useVisibleNodes(
  nodes: Map<string, CanvasNodeData>,
  viewport: Viewport,
  containerSize: ContainerSize,
): Set<string> {
  return useMemo(() => {
    const visible = new Set<string>()

    // Viewport bounds in canvas coordinates.
    const vLeft =
      -viewport.x / viewport.scale - VIEWPORT_MARGIN / viewport.scale
    const vTop = -viewport.y / viewport.scale - VIEWPORT_MARGIN / viewport.scale
    const vRight =
      (-viewport.x + containerSize.width) / viewport.scale +
      VIEWPORT_MARGIN / viewport.scale
    const vBottom =
      (-viewport.y + containerSize.height) / viewport.scale +
      VIEWPORT_MARGIN / viewport.scale

    for (const [id, node] of nodes) {
      const nodeRight = node.x + node.width
      const nodeBottom = node.y + node.height

      // AABB intersection test.
      if (
        node.x <= vRight &&
        nodeRight >= vLeft &&
        node.y <= vBottom &&
        nodeBottom >= vTop
      ) {
        visible.add(id)
      }
    }

    return visible
  }, [
    nodes,
    viewport.x,
    viewport.y,
    viewport.scale,
    containerSize.width,
    containerSize.height,
  ])
}
