import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'

import type { LoadingView } from '@s4wave/web/ui/loading/types.js'

interface ResourceViewOptions {
  title: string
  loadingDetail?: string
  loadedDetail?: string
  onCancel?: () => void
}

// toResourceView is the catch-all adapter for SDK Resources that only
// distinguish loading / loaded / error. Pass title + optional detail hints;
// the adapter handles state transitions. When the resource is loaded the
// view is 'synced' so the primitive unmounts naturally at the call site
// (callers typically short-circuit and render the loaded UI instead).
export function toResourceView<T>(
  resource: Resource<T> | null | undefined,
  options: ResourceViewOptions,
): LoadingView {
  const { title, loadingDetail, loadedDetail, onCancel } = options
  if (!resource) {
    return {
      state: 'loading',
      title,
      detail: loadingDetail ?? 'Preparing...',
      onCancel,
    }
  }
  if (resource.error) {
    return {
      state: 'error',
      title,
      detail: 'Loading failed.',
      error: resource.error.message,
      onRetry: resource.retry,
      onCancel,
    }
  }
  if (resource.loading || resource.value === null) {
    return {
      state: 'active',
      title,
      detail: loadingDetail ?? 'Loading...',
      onCancel,
    }
  }
  return {
    state: 'synced',
    title,
    detail: loadedDetail,
  }
}
