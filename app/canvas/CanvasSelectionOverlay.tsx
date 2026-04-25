import { cn } from '@s4wave/web/style/utils.js'

import type { DragRect } from './useCanvasSelection.js'

// CanvasSelectionOverlayProps are the props for CanvasSelectionOverlay.
interface CanvasSelectionOverlayProps {
  dragRect: DragRect | null
  className?: string
}

// CanvasSelectionOverlay renders the drag-select rectangle overlay.
export function CanvasSelectionOverlay({
  dragRect,
  className,
}: CanvasSelectionOverlayProps) {
  if (!dragRect) return null

  const left = Math.min(dragRect.startX, dragRect.endX)
  const top = Math.min(dragRect.startY, dragRect.endY)
  const width = Math.abs(dragRect.endX - dragRect.startX)
  const height = Math.abs(dragRect.endY - dragRect.startY)

  return (
    <div
      className={cn(
        'border-brand/30 bg-brand/5 pointer-events-none absolute rounded-sm border',
        className,
      )}
      style={{ left, top, width, height }}
    />
  )
}
