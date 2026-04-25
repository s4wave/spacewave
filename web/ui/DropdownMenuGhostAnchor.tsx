import { forwardRef, type ComponentPropsWithoutRef } from 'react'
import { createPortal } from 'react-dom'

export interface DropdownMenuGhostAnchorProps extends ComponentPropsWithoutRef<'div'> {
  x: number
  y: number
}

// DropdownMenuGhostAnchor renders a fixed-position trigger anchor in document.body.
export const DropdownMenuGhostAnchor = forwardRef<
  HTMLDivElement,
  DropdownMenuGhostAnchorProps
>(function DropdownMenuGhostAnchor(
  { x, y, style, ...props }: DropdownMenuGhostAnchorProps,
  ref,
) {
  if (typeof document === 'undefined') return null

  return createPortal(
    <div
      ref={ref}
      data-slot="dropdown-menu-ghost-anchor"
      style={{
        position: 'fixed',
        left: x,
        top: y,
        width: 0,
        height: 0,
        pointerEvents: 'none',
        ...style,
      }}
      {...props}
    />,
    document.body,
  )
})
