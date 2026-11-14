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
  lastOf,
  resourceOf,
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
   * reconnect reopens a stream that fails after yielding, keeping the value
   * while it backs off exponentially. Once the backoff reaches its maximum,
   * the resource reports failed, with the value as last and Retry, and keeps
   * reopening at that wait without leaving the failed state. Each yielded
   * value resets the backoff and settles the resource. Retry reopens at once.
   * A stream that fails before its first value reports the error.
   */
  reconnect?: boolean
  /**
   * keepWhileLoading keeps the value of the previous deps while the stream
   * for new deps loads, for a query that grows in steps, such as a tree
   * level expanding. The kept value goes on the first value, failure or
   * empty settle, and never becomes last. Without it a deps change shows no
   * value while loading.
   */
  keepWhileLoading?: boolean
}

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
  const parentValueChangeCount = useResourceTransitionVersion([parent.value])
  const [retryCount, setRetryCount] = useState(0)
  const abortRetriedParentValueChangeRef = useRef<number | null>(null)
  const valueRef = useRef<T | null>(null)
  // reconnectingRef is set once the stream yielded under the current key;
  // quietRef is set while a capped stream reopens in the background, so it
  // stays failed even with nothing kept.
  const reconnectingRef = useRef(false)
  const quietRef = useRef(false)
  const [backoff] = useState(exponentialBackoff)

  // Restart the stream when the factory or the parent value changes. key
  // changes with the factory: a value never outlives its key.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const stableFactory = useCallback(streamFactory, deps)
  const key = useResourceTransitionVersion([stableFactory])
  const transitionVersion = useResourceTransitionVersion([
    stableFactory,
    parentValueChangeCount,
  ])

  // Track the current run and the value it reports.
  const {
    state,
    startAfter,
    complete,
    fail,
    isCurrent,
    isPending,
    waitAgain,
  } = useResourceTransitionState<T>(
    true,
    key,
    options?.keepWhileLoading ?? false,
  )
  const effectiveLoading =
    parent.loading || isPending(transitionVersion) || state.loading

  const keyRef = useRef(key)

  // Follow the stream for the current parent value and factory.
  useEffect(() => {
    // A new key starts over: no value, no reconnect, a fresh backoff.
    if (keyRef.current !== key) {
      keyRef.current = key
      valueRef.current = null
      reconnectingRef.current = false
      quietRef.current = false
      backoff.reset()
    }

    // Wait for the parent before opening the stream.
    const parentValue = parent.value
    const run = startAfter([parent], transitionVersion, key, quietRef.current)
    if (!run || parentValue === null) {
      return
    }

    // Follow the stream, keeping the current value for an equal item.
    let reconnectTimer: ReturnType<typeof setTimeout> | undefined
    void (async () => {
      let emitted = false
      try {
        for await (const item of stableFactory(parentValue, run.signal)) {
          if (!isCurrent(run)) {
            break
          }
          backoff.reset()
          reconnectingRef.current = false
          quietRef.current = false
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

        // Reopen after a backoff, keeping the value while waiting. At the
        // maximum wait, report the failure and keep reopening quietly.
        if (reconnect && (emitted || reconnectingRef.current)) {
          reconnectingRef.current = true
          const wait = backoff()
          if (backoff.capped()) {
            fail(run, e)
          } else {
            waitAgain(key)
          }
          reconnectTimer = setTimeout(() => {
            quietRef.current = backoff.capped()
            setRetryCount((c) => c + 1)
          }, wait)
          return
        }
        fail(run, e)
      }
    })()

    return () => {
      run.abort()
      clearTimeout(reconnectTimer)
    }
    // parent stands for its value, loading and error, listed below.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    backoff,
    reconnect,
    parent.loading,
    parent.value,
    parent.error,
    parentValueChangeCount,
    retryCount,
    transitionVersion,
    key,
    startAfter,
    complete,
    fail,
    isCurrent,
    waitAgain,
    stableFactory,
  ])

  // Reopen the stream at once on request. A stream that failed with nothing
  // kept shows loading again; one that kept a value stays failed over it
  // until the reopened stream yields, as startAfter keeps it, so its content
  // never blanks.
  const retry = useCallback(() => {
    quietRef.current = false
    setRetryCount((c) => c + 1)
  }, [])

  // Report the parent's error ahead of the stream's own.
  return useMemo(
    () =>
      resourceOf<T>({
        value: state.value,
        last: lastOf(state),
        loading: effectiveLoading,
        error: parent.error ?? state.error,
        retry: parent.error ? parent.retry : retry,
      }),
    [state, effectiveLoading, retry, parent.error, parent.retry],
  )
}
