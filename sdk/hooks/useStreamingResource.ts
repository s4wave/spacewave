import { useCallback, useEffect, useMemo, useState } from 'react'
import { isAbortError } from 'starpc'

import type { Resource } from './useResource.js'

// useStreamingResource subscribes to an async iterable derived from a parent resource.
// Returns a Resource<T> that updates on each yielded value.
export function useStreamingResource<P, T>(
  parent: Resource<P>,
  streamFactory: (parent: P, signal: AbortSignal) => AsyncIterable<T>,
  deps: React.DependencyList,
): Resource<T> {
  const [value, setValue] = useState<T | null>(null)
  const [error, setError] = useState<Error | null>(null)
  const [loading, setLoading] = useState(true)
  const [retryCount, setRetryCount] = useState(0)

  // eslint-disable-next-line react-hooks/exhaustive-deps
  const stableFactory = useCallback(streamFactory, deps)

  useEffect(() => {
    const parentValue = parent.value
    if (!parentValue) {
      setValue(null)
      setLoading(true)
      setError(null)
      return
    }

    const abort = new AbortController()
    setLoading(true)
    setError(null)

    void (async () => {
      try {
        for await (const item of stableFactory(parentValue, abort.signal)) {
          if (abort.signal.aborted) break
          setValue(item)
          setLoading(false)
        }
      } catch (err) {
        if (abort.signal.aborted) return
        if (isAbortError(err)) return
        if (err instanceof Error && err.name === 'AbortError') return
        const e = err instanceof Error ? err : new Error(String(err))
        setError(e)
        setLoading(false)
      }
    })()

    return () => abort.abort()
  }, [parent.value, retryCount, stableFactory])

  const retry = useCallback(() => {
    setRetryCount((c) => c + 1)
  }, [])

  return useMemo(
    () => ({
      value: parent.loading ? null : value,
      loading: parent.loading || loading,
      error: parent.error ?? error,
      retry: parent.error ? parent.retry : retry,
    }),
    [value, loading, error, retry, parent.loading, parent.error, parent.retry],
  )
}
