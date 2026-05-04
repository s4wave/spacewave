import type { ComponentPropsWithoutRef } from 'react'

// ExternalLink opens off-site anchors in a separate browser tab.
export function ExternalLink({
  rel,
  target,
  ...props
}: ComponentPropsWithoutRef<'a'>) {
  return (
    <a
      {...props}
      target={target ?? '_blank'}
      rel={rel ?? 'noopener noreferrer'}
    />
  )
}
