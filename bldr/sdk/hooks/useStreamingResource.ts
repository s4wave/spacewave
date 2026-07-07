import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { isAbortError } from 'starpc'

import type { Resource } from './useResource.js'
import {
  useResourceTransitionState,
  useResourceTransitionVersion,
} from './resourceTransitionState.js'

// useStreamingResource subscribes to an async iterable derived from a parent resource.
// Returns a Resource<T> that updates on each yielded value.
export function useStreamingResource<P, T>(
  parent: Resource<P>,
  streamFactory: (parent: P, signal: AbortSignal) => AsyncIterable<T>,
  deps: React.DependencyList,
): Resource<T> {
  const parentValueChangeCount = useResourceTransitionVersion([parent.value])
  const [retryCount, setRetryCount] = useState(0)
  const abortRetriedParentValueChangeRef = useRef<number | null>(null)

  // eslint-disable-next-line react-hooks/exhaustive-deps
  const stableFactory = useCallback(streamFactory, deps)
  const transitionVersion = useResourceTransitionVersion([
    stableFactory,
    parentValueChangeCount,
  ])
  const {
    state,
    begin,
    complete,
    fail,
    isCurrent,
    isPending,
    setLoading,
    settleNull,
  } = useResourceTransitionState<T>(true)
  const effectiveLoading =
    parent.loading || isPending(transitionVersion) || state.loading

  useEffect(() => {
    const parentValue = parent.value
    if (parentValue === null) {
      settleNull(transitionVersion, parent.loading)
      return
    }
    if (parent.loading) {
      setLoading()
      return
    }

    const run = begin(transitionVersion)

    void (async () => {
      let emitted = false
      try {
        for await (const item of stableFactory(parentValue, run.signal)) {
          if (!isCurrent(run)) {
            break
          }
          emitted = true
          if (
            abortRetriedParentValueChangeRef.current === parentValueChangeCount
          ) {
            abortRetriedParentValueChangeRef.current = null
          }
          complete(run, item)
        }
        if (!isCurrent(run)) {
          return
        }
        if (!emitted) {
          complete(run, null)
        }
      } catch (err) {
        if (!isCurrent(run)) return
        const e = err instanceof Error ? err : new Error(String(err))
        if (isAbortError(err) || e.name === 'AbortError') {
          if (
            abortRetriedParentValueChangeRef.current !== parentValueChangeCount
          ) {
            abortRetriedParentValueChangeRef.current = parentValueChangeCount
            setRetryCount((c) => c + 1)
            return
          }
          fail(run, e)
          return
        }
        fail(run, e)
      }
    })()

    return () => run.abort()
  }, [
    parent.loading,
    parent.value,
    parentValueChangeCount,
    retryCount,
    transitionVersion,
    begin,
    complete,
    fail,
    isCurrent,
    setLoading,
    settleNull,
    stableFactory,
  ])

  const retry = useCallback(() => {
    setRetryCount((c) => c + 1)
  }, [])

  return useMemo(
    () => ({
      value: state.value,
      loading: effectiveLoading,
      error: parent.error ?? state.error,
      retry: parent.error ? parent.retry : retry,
    }),
    [
      state.value,
      effectiveLoading,
      state.error,
      retry,
      parent.error,
      parent.retry,
    ],
  )
}
