import { useMemo } from 'react'
import type { Resource } from './useResource.js'
import { resourceOf } from './resourceTransitionState.js'

// useMappedResource applies a synchronous transform to a Resource value.
// Returns a new Resource with the transformed value, propagating
// loading/error/retry. A failed source's last value is mapped as well.
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

  const sourceLast = source.error ? source.last : null
  const last = useMemo(
    () => (sourceLast !== null ? mapFn(sourceLast) : null),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [sourceLast, ...(deps ?? [])],
  )

  return useMemo(
    () =>
      resourceOf<B>({
        value,
        last,
        loading: source.loading,
        error: source.error,
        retry: source.retry,
      }),
    [value, last, source.loading, source.error, source.retry],
  )
}
