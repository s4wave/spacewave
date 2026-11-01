import { type ComponentPropsWithRef } from 'react'
import { createPortal } from 'react-dom'
import * as stylex from '@stylexjs/stylex'

const styles = stylex.create({
  anchor: {
    position: 'fixed',
    left: 'var(--dropdown-ghost-left)',
    top: 'var(--dropdown-ghost-top)',
    width: 0,
    height: 0,
    pointerEvents: 'none',
  },
})

export interface DropdownMenuGhostAnchorProps extends Omit<
  ComponentPropsWithRef<'div'>,
  'className'
> {
  sx?: stylex.StyleXStyles
  className?: never
  x: number
  y: number
}

// DropdownMenuGhostAnchor renders a fixed-position trigger anchor in document.body.
export function DropdownMenuGhostAnchor({
  x,
  y,
  sx,
  ...props
}: DropdownMenuGhostAnchorProps) {
  // Keep the detached anchor out of server renders.
  if (typeof document === 'undefined') return null

  return createPortal(
    <div
      data-slot="dropdown-menu-ghost-anchor"
      sx={[styles.anchor, sx]}
      style={{
        '--dropdown-ghost-left': `${x}px`,
        '--dropdown-ghost-top': `${y}px`,
      }}
      {...props}
    />,
    document.body,
  )
}
