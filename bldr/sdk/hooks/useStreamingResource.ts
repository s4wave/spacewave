import {
  useCallback,
  useEffect,
  useEffectEvent,
  useMemo,
  useRef,
  useState,
} from 'react'
import { isAbortError } from 'starpc'

import { exponentialBackoff } from '../../web/bldr/retry.js'
import type { Resource } from './useResource.js'
import {
  useResourceTransitionState,
  useResourceTransitionVersion,
} from './resourceTransitionState.js'

/** UseStreamingResourceOptions tunes how a streaming resource follows its stream. */
export interface UseStreamingResourceOptions<T> {
  /**
   * equals reports whether a yielded value matches the current one. A matching
   * value keeps the current value's identity, so its consumers do not render.
   */
  equals?: (prev: T, next: T) => boolean
  /**
   * reconnect reopens a stream that fails after yielding, keeping the last
   * value while it backs off exponentially. Each yielded value resets the
   * backoff. A stream that fails before its first value reports the error.
   */
  reconnect?: boolean
  /**
   * firstValueTimeout is how long, in milliseconds, an opened stream may go
   * without its first value before the hook closes it and reopens it after a
   * backoff, staying in its loading state, so a stream stuck behind a stalled
   * or restarted computation recovers without a reload. The deadline doubles
   * with each consecutive stall, up to eight times this value, so a slow
   * computation still finishes. A yielded value resets it. Each open has its
   * own deadline, a reconnect included. Zero disables it. Defaults to
   * defaultFirstValueTimeout.
   */
  firstValueTimeout?: number
}

/** defaultFirstValueTimeout is the default first-value deadline in ms. */
export const defaultFirstValueTimeout = 15_000

/**
 * useStreamingResource subscribes to an async iterable derived from a parent
 * resource. Returns a Resource<T> that updates on each yielded value.
 */
export function useStreamingResource<P, T>(
  parent: Resource<P>,
  streamFactory: (parent: P, signal: AbortSignal) => AsyncIterable<T>,
  deps: React.DependencyList,
  options?: UseStreamingResourceOptions<T>,
): Resource<T> {
  // Track the options, the last value and the stream's restart state.
  const equals = useEffectEvent(
    (prev: T, next: T) => options?.equals?.(prev, next) ?? false,
  )
  const reconnect = options?.reconnect ?? false
  const firstValueTimeout = options?.firstValueTimeout ?? defaultFirstValueTimeout
  const parentValueChangeCount = useResourceTransitionVersion([parent.value])
  const [retryCount, setRetryCount] = useState(0)
  const abortRetriedParentValueChangeRef = useRef<number | null>(null)
  const valueRef = useRef<T | null>(null)
  const reconnectingRef = useRef(false)
  // Track consecutive stalls for the current factory and parent value.
  const stallsRef = useRef({ version: 0, count: 0 })
  const [backoff] = useState(exponentialBackoff)

  // Restart the stream when the factory or the parent value changes.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const stableFactory = useCallback(streamFactory, deps)
  const transitionVersion = useResourceTransitionVersion([
    stableFactory,
    parentValueChangeCount,
  ])

  // Track the current run and the value it reports.
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

  // Follow the stream for the current parent value and factory.
  useEffect(() => {
    // Wait for the parent before opening the stream.
    const parentValue = parent.value
    if (parentValue === null) {
      settleNull(transitionVersion, parent.loading)
      return
    }
    if (parent.loading) {
      setLoading()
      return
    }

    // Reset the deadline series when the factory or parent value changes.
    if (stallsRef.current.version !== transitionVersion) {
      stallsRef.current = { version: transitionVersion, count: 0 }
    }

    // Follow the stream, keeping the current value for an equal item.
    const run = begin(transitionVersion)
    let reconnectTimer: ReturnType<typeof setTimeout> | undefined

    // Close a stream that gives no first value by its deadline and reopen it
    // after a backoff. The hook keeps its loading state during the backoff.
    let deadlineTimer: ReturnType<typeof setTimeout> | undefined
    if (firstValueTimeout > 0) {
      const deadline = firstValueTimeout * 2 ** Math.min(stallsRef.current.count, 3)
      deadlineTimer = setTimeout(() => {
        if (!isCurrent(run)) return
        stallsRef.current.count += 1
        run.abort()
        reconnectTimer = setTimeout(
          () => setRetryCount((c) => c + 1),
          backoff(),
        )
      }, deadline)
    }

    void (async () => {
      let emitted = false
      try {
        for await (const item of stableFactory(parentValue, run.signal)) {
          if (!isCurrent(run)) {
            break
          }
          clearTimeout(deadlineTimer)
          stallsRef.current.count = 0
          backoff.reset()
          reconnectingRef.current = false
          if (
            abortRetriedParentValueChangeRef.current === parentValueChangeCount
          ) {
            abortRetriedParentValueChangeRef.current = null
          }
          const prev = valueRef.current
          const value = prev !== null && equals(prev, item) ? prev : item
          if (emitted && value === prev) continue
          emitted = true
          valueRef.current = value
          complete(run, value)
        }
        clearTimeout(deadlineTimer)
        if (!isCurrent(run)) {
          return
        }
        if (!emitted) {
          complete(run, null)
        }
      } catch (err) {
        clearTimeout(deadlineTimer)
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

        // Reopen after a backoff, keeping the last value while waiting.
        if (reconnect && (emitted || reconnectingRef.current)) {
          reconnectingRef.current = true
          setLoading()
          reconnectTimer = setTimeout(
            () => setRetryCount((c) => c + 1),
            backoff(),
          )
          return
        }
        fail(run, e)
      }
    })()

    return () => {
      run.abort()
      clearTimeout(deadlineTimer)
      clearTimeout(reconnectTimer)
    }
  }, [
    backoff,
    reconnect,
    firstValueTimeout,
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

  // Restart the stream on request.
  const retry = useCallback(() => {
    setRetryCount((c) => c + 1)
  }, [])

  // Report the parent's error ahead of the stream's own.
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
