import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

import { toResourceView } from '../status/resource.js'

interface ResourceLoadingCardProps<T> {
  resource: Resource<T> | null | undefined
  title: string
  loadingDetail?: string
  loadedDetail?: string
  onCancel?: () => void
  className?: string
}

// ResourceLoadingCard is the catch-all wrapper for SDK Resources that only
// distinguish loading / loaded / error. Pass the resource plus display hints.
export function ResourceLoadingCard<T>({
  resource,
  title,
  loadingDetail,
  loadedDetail,
  onCancel,
  className,
}: ResourceLoadingCardProps<T>) {
  return (
    <LoadingCard
      view={toResourceView(resource, {
        title,
        loadingDetail,
        loadedDetail,
        onCancel,
      })}
      className={className}
    />
  )
}
