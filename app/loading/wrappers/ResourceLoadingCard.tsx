import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

import { toResourceView } from '../status/resource.js'

interface ResourceLoadingCardProps<T> {
  className?: never
  resource: Resource<T> | null | undefined
  title: string
  loadingDetail?: string
  loadedDetail?: string
  onCancel?: () => void
  sx?: ComponentStyles
}

// ResourceLoadingCard is the catch-all wrapper for SDK Resources that only
// distinguish loading / loaded / error. Pass the resource plus display hints.
export function ResourceLoadingCard<T>({
  resource,
  title,
  loadingDetail,
  loadedDetail,
  onCancel,
  sx,
}: Omit<ResourceLoadingCardProps<T>, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  return (
    <LoadingCard
      view={toResourceView(resource, {
        title,
        loadingDetail,
        loadedDetail,
        onCancel,
      })}
      sx={sx}
    />
  )
}
