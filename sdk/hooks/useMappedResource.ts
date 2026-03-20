import { useMemo } from 'react'
import type { Resource } from './useResource.js'

// useMappedResource applies a synchronous transform to a Resource value.
// Returns a new Resource with the transformed value, propagating loading/error/retry.
export function useMappedResource<A, B>(
  source: Resource<A>,
  mapFn: (value: A) => B,
  deps?: React.DependencyList,
): Resource<B> {
  const value = useMemo(
    () => (source.value !== null ? mapFn(source.value) : null),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [source.value, ...(deps ?? [])],
  )

  return useMemo(
    () => ({
      value,
      loading: source.loading,
      error: source.error,
      retry: source.retry,
    }),
    [value, source.loading, source.error, source.retry],
  )
}
