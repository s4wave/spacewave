import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
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
  const prevParentValueRef = useRef(parent.value)
  const parentValueChangeCountRef = useRef(0)
  const startedParentValueChangeCountRef = useRef(0)
  const startedStreamGenerationRef = useRef(0)

  if (prevParentValueRef.current !== parent.value) {
    prevParentValueRef.current = parent.value
    parentValueChangeCountRef.current += 1
  }
  const parentValueChangeCount = parentValueChangeCountRef.current

  // eslint-disable-next-line react-hooks/exhaustive-deps
  const stableFactory = useCallback(streamFactory, deps)
  const hasPendingParentChange =
    startedParentValueChangeCountRef.current !== parentValueChangeCount
  const effectiveLoading = parent.loading || hasPendingParentChange || loading

  useEffect(() => {
    const parentValue = parent.value
    if (!parentValue) {
      setValue(null)
      setLoading(parent.loading)
      setError(null)
      return
    }
    if (parent.loading) {
      setLoading(true)
      setError(null)
      return
    }

    const abort = new AbortController()
    const generation = startedStreamGenerationRef.current + 1
    startedStreamGenerationRef.current = generation
    startedParentValueChangeCountRef.current = parentValueChangeCount
    setLoading(true)
    setError(null)

    void (async () => {
      let emitted = false
      try {
        for await (const item of stableFactory(parentValue, abort.signal)) {
          if (
            abort.signal.aborted ||
            generation !== startedStreamGenerationRef.current
          ) {
            break
          }
          emitted = true
          setValue(item)
          setLoading(false)
        }
        if (
          abort.signal.aborted ||
          generation !== startedStreamGenerationRef.current
        ) {
          return
        }
        if (!emitted) {
          setValue(null)
        }
        setLoading(false)
      } catch (err) {
        if (abort.signal.aborted) return
        if (generation !== startedStreamGenerationRef.current) return
        if (isAbortError(err)) return
        if (err instanceof Error && err.name === 'AbortError') return
        const e = err instanceof Error ? err : new Error(String(err))
        setError(e)
        setLoading(false)
      }
    })()

    return () => abort.abort()
  }, [parent.loading, parentValueChangeCount, retryCount, stableFactory])

  const retry = useCallback(() => {
    setRetryCount((c) => c + 1)
  }, [])

  return useMemo(
    () => ({
      value,
      loading: effectiveLoading,
      error: parent.error ?? error,
      retry: parent.error ? parent.retry : retry,
    }),
    [value, effectiveLoading, error, retry, parent.error, parent.retry],
  )
}
