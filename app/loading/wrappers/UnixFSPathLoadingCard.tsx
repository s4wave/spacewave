import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

import { toUnixFSPathView } from '../status/unixfs-path.js'

interface UnixFSPathLoadingCardProps {
  root: Resource<unknown> | null | undefined
  lookup: Resource<unknown> | null | undefined
  stat: Resource<unknown> | null | undefined
  entries: Resource<unknown> | null | undefined
  path?: string
  onRetry?: () => void
  onCancel?: () => void
  className?: string
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
  className,
}: UnixFSPathLoadingCardProps) {
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
      className={className}
    />
  )
}
