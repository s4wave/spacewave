import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

import { toUnixFSPathView } from '../status/unixfs-path.js'

interface UnixFSPathLoadingCardProps {
  className?: never
  root: Resource<unknown> | null | undefined
  lookup: Resource<unknown> | null | undefined
  stat: Resource<unknown> | null | undefined
  entries: Resource<unknown> | null | undefined
  path?: string
  onRetry?: () => void
  onCancel?: () => void
  sx?: ComponentStyles
}

// UnixFSPathLoadingCard combines the four sequential UnixFS resource handles
// (root, path lookup, stat, readdir) into a single staged LoadingCard.
export function UnixFSPathLoadingCard({
  root,
  lookup,
  stat,
  entries,
  path,
  onRetry,
  onCancel,
  sx,
}: Omit<UnixFSPathLoadingCardProps, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  return (
    <LoadingCard
      view={toUnixFSPathView({
        root,
        lookup,
        stat,
        entries,
        path,
        onRetry,
        onCancel,
      })}
      sx={sx}
    />
  )
}
