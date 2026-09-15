import { type ComponentPropsWithRef } from 'react'
import { createPortal } from 'react-dom'

export interface DropdownMenuGhostAnchorProps extends ComponentPropsWithRef<'div'> {
  x: number
  y: number
}

// DropdownMenuGhostAnchor renders a fixed-position trigger anchor in document.body.
export function DropdownMenuGhostAnchor({
  x,
  y,
  ...props
}: DropdownMenuGhostAnchorProps) {
  if (typeof document === 'undefined') return null

  return createPortal(
    <div
      data-slot="dropdown-menu-ghost-anchor"
      className="dropdown-ghost-anchor"
      style={{
        '--dropdown-ghost-left': `${x}px`,
        '--dropdown-ghost-top': `${y}px`,
      }}
      {...props}
    />,
    document.body,
  )
}
