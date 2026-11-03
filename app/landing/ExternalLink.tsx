import * as stylex from '@stylexjs/stylex'
import type { ComponentPropsWithoutRef } from 'react'

// ExternalLink opens off-site anchors in a separate browser tab.
export function ExternalLink({
  sx,
  rel,
  target,
  ...props
}: ComponentPropsWithoutRef<'a'> & {
  sx?: stylex.StyleXStyles
  className?: never
}) {
  return (
    <a
      sx={sx}
      {...props}
      target={target ?? '_blank'}
      rel={rel ?? 'noopener noreferrer'}
    />
  )
}
