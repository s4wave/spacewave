import * as stylex from '@stylexjs/stylex'
import type { ComponentPropsWithoutRef } from 'react'

import { stylexProps } from '@s4wave/web/style/props.js'

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
      {...stylexProps([sx], props)}
      target={target ?? '_blank'}
      rel={rel ?? 'noopener noreferrer'}
    />
  )
}
