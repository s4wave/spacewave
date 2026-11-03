import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'

import type { DragRect } from './useCanvasSelection.js'

import * as stylex from '@stylexjs/stylex'
import {
  utilityState as themeUtilityState,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    borderRadius: 'var(--radius-sm)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
    pointerEvents: 'none',
    position: 'absolute',
    height: 'var(--canvas-selection-height)',
    left: 'var(--canvas-selection-left)',
    top: 'var(--canvas-selection-top)',
    width: 'var(--canvas-selection-width)',
  },
})

// CanvasSelectionOverlayProps are the props for CanvasSelectionOverlay.
interface CanvasSelectionOverlayProps {
  className?: never
  dragRect: DragRect | null
  sx?: ComponentStyles
}

// CanvasSelectionOverlay renders the drag-select rectangle overlay.
export function CanvasSelectionOverlay({
  dragRect,
  sx,
}: Omit<CanvasSelectionOverlayProps, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  if (!dragRect) return null

  const left = Math.min(dragRect.startX, dragRect.endX)
  const top = Math.min(dragRect.startY, dragRect.endY)
  const width = Math.abs(dragRect.endX - dragRect.startX)
  const height = Math.abs(dragRect.endY - dragRect.startY)

  return (
    <div
      sx={[stylexStyles.rule1, sx]}
      style={{
        '--canvas-selection-left': `${left}px`,
        '--canvas-selection-top': `${top}px`,
        '--canvas-selection-width': `${width}px`,
        '--canvas-selection-height': `${height}px`,
      }}
    />
  )
}
