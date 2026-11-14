import { useCallback, useMemo, useRef, useState } from 'react'

import type { Resource, ResourceFields } from './useResource.js'

/**
 * ResourceTransitionState is what a resource hook holds for one key: the
 * dependency version its values belong to.
 *
 * value is what the hook reports while it is not failed. It stays while the
 * resource loads again, and becomes null whenever the resource fails or its
 * parent stops being ready with no value of its own. last is the most recent
 * value for this key once value has gone null that way; the hook shows it
 * only on the failed state. keepable is false once the run that produced
 * value disposed its handles, so a disposed value never becomes last. owner
 * is the generation of the run that produced value, or last once it moved.
 */
export type ResourceTransitionState<T> = {
  key: number
  owner: number
  value: T | null
  last: T | null
  keepable: boolean
  loading: boolean
  error: Error | null
}

export type ResourceTransitionRun = {
  generation: number
  key: number
  signal: AbortSignal
  abort: () => void
}

/** ParentState is the part of a parent Resource that gates a child. */
export type ParentState = {
  value: unknown
  loading: boolean
  error: Error | null
}

/**
 * isReleasedHandle reports whether value is a handle the server or client
 * released. A released handle is never a resource's last value.
 */
export function isReleasedHandle(value: unknown): boolean {
  return (
    typeof value === 'object' &&
    value !== null &&
    'released' in value &&
    value.released === true
  )
}

/**
 * resourceReady reports whether a child may start on resource: it holds a
 * live value and is neither loading nor failed. Every hook that starts a
 * child gates on this predicate alone.
 */
export function resourceReady(resource: ParentState): boolean {
  return (
    resource.value !== null &&
    !resource.loading &&
    resource.error === null &&
    !isReleasedHandle(resource.value)
  )
}

/**
 * resourceOf builds the Resource state for fields. An error wins: the
 * resource is failed, value is null and last keeps fields.last unless it is a
 * released handle. Otherwise loading wins over settled. It lives here rather
 * than in useResource.js, which many tests mock whole.
 */
export function resourceOf<T>(fields: ResourceFields<T>): Resource<T> {
  const { value, loading, error, last, ...base } = fields
  if (error) {
    const kept = last ?? null
    return {
      ...base,
      value: null,
      loading: false,
      error,
      last: isReleasedHandle(kept) ? null : kept,
    }
  }
  if (loading) {
    return { ...base, value, loading: true, error: null }
  }
  return { ...base, value, loading: false, error: null }
}

/**
 * resourceContent returns what a viewer draws from resource: its value, or
 * the last value a failed resource kept, which may be stale. Besides
 * narrowing to the failed variant, it is the only way to read last. A caller
 * that draws its result owes a failure notice whenever resource.error is
 * set, so stale content never reads as current, and never draws an empty
 * claim from a failed resource.
 */
export function resourceContent<T>(resource: Resource<T>): T | null {
  return resource.error ? resource.last : resource.value
}

/** lastOf returns the value a failure of state would keep as last. */
export function lastOf<T>(state: ResourceTransitionState<T>): T | null {
  const last =
    state.value === null ? state.last : state.keepable ? state.value : null
  return isReleasedHandle(last) ? null : last
}

export function useResourceTransitionVersion(
  values: readonly unknown[],
): number {
  const previousValuesRef = useRef<readonly unknown[] | null>(null)
  const versionRef = useRef(0)
  if (
    previousValuesRef.current === null ||
    !(
      previousValuesRef.current.length === values.length &&
      previousValuesRef.current.every((value, index) => value === values[index])
    )
  ) {
    previousValuesRef.current = [...values]
    versionRef.current += 1
  }

  return versionRef.current
}

// fresh is the state of a key that has produced nothing yet.
function fresh<T>(key: number, loading: boolean): ResourceTransitionState<T> {
  return {
    key,
    owner: 0,
    value: null,
    last: null,
    keepable: true,
    loading,
    error: null,
  }
}

/**
 * enter returns prev as the state of key. A state written under another key
 * starts key loading with no last. With carry it keeps the other key's value
 * while key loads, marked so it never becomes last; the first value, failure
 * or empty settle under key replaces it.
 */
function enter<T>(
  prev: ResourceTransitionState<T>,
  key: number,
  carry: boolean,
): ResourceTransitionState<T> {
  if (prev.key === key) {
    return prev
  }
  const next = fresh<T>(key, true)
  return carry && prev.value !== null
    ? { ...next, value: prev.value, keepable: false }
    : next
}

/**
 * useResourceTransitionState holds the state of a resource hook under key.
 *
 * The returned state already belongs to key: when key differs from the key
 * the state was written under, it reads as loading with no last, on the same
 * render that changes key. Its value is null, so a value never shows under
 * another key, unless carry is set: then the previous key's value shows
 * while key loads, and only then.
 */
export function useResourceTransitionState<T>(
  initialLoading: boolean,
  key: number,
  carry = false,
) {
  const [held, setState] = useState<ResourceTransitionState<T>>(() =>
    fresh(key, initialLoading),
  )
  const state = useMemo(() => enter(held, key, carry), [held, key, carry])
  const currentGenerationRef = useRef(0)
  const startedVersionRef = useRef(0)

  const isPending = useCallback(
    (version: number) => startedVersionRef.current !== version,
    [],
  )

  const isCurrent = useCallback(
    (run: ResourceTransitionRun) =>
      !run.signal.aborted && run.generation === currentGenerationRef.current,
    [],
  )

  // update applies next to the state held for key, entering key first.
  const update = useCallback(
    (
      key: number,
      next: (prev: ResourceTransitionState<T>) => ResourceTransitionState<T>,
    ) => {
      setState((prev) => next(enter(prev, key, carry)))
    },
    [carry],
  )

  // settle marks version started and ends the current run.
  const settle = useCallback((version: number) => {
    startedVersionRef.current = version
    currentGenerationRef.current += 1
  }, [])

  /**
   * waitAgain marks the resource loading, keeping its value. A quiet wait
   * keeps a failed state as it is.
   */
  const waitAgain = useCallback(
    (key: number, quiet = false) =>
      update(key, (prev) =>
        (quiet && prev.error !== null) || (prev.loading && prev.error === null)
          ? prev
          : { ...prev, loading: true, error: null },
      ),
    [update],
  )

  // toLast moves the value to last, leaving loading and error as given.
  const toLast = useCallback(
    (key: number, loading: boolean, error: Error | null) =>
      update(key, (prev) => ({
        ...prev,
        value: null,
        last: lastOf(prev),
        loading,
        error,
      })),
    [update],
  )

  /**
   * restart starts over on request: the value goes null as the hook reports a
   * fresh load, and last keeps it for a failure of the new run.
   */
  const restart = useCallback(
    (key: number) => toLast(key, true, null),
    [toLast],
  )

  /** settleEmpty settles to no value: disabled, or a parent resolved to null. */
  const settleEmpty = useCallback(
    (version: number, key: number) => {
      settle(version)
      update(key, (prev) =>
        prev.value === null &&
        prev.last === null &&
        !prev.loading &&
        prev.error === null
          ? prev
          : fresh(key, false),
      )
    },
    [settle, update],
  )

  /**
   * begin starts a run under key, keeping the value while it loads. A quiet
   * run keeps a failed state as it is until the run yields a value.
   */
  const begin = useCallback(
    (version: number, key: number, quiet = false): ResourceTransitionRun => {
      const abortController = new AbortController()
      settle(version)
      waitAgain(key, quiet)
      return {
        generation: currentGenerationRef.current,
        key,
        signal: abortController.signal,
        abort: () => abortController.abort(),
      }
    },
    [settle, waitAgain],
  )

  /**
   * startAfter begins a run once every parent is ready. Otherwise it holds
   * the child: under a failed parent its value moves to last and it waits,
   * so it reports the parent's failure and, once the parent turns ready,
   * loading until its run starts, never empty; loading under a loading or
   * released parent; and empty under a parent that settled with no value.
   */
  const startAfter = useCallback(
    (
      parents: readonly ParentState[],
      version: number,
      key: number,
      quiet = false,
    ): ResourceTransitionRun | null => {
      if (parents.every(resourceReady)) {
        return begin(version, key, quiet)
      }
      if (parents.some((parent) => parent.error !== null)) {
        settle(version)
        toLast(key, true, null)
        return null
      }
      if (
        parents.some(
          (parent) => parent.loading || isReleasedHandle(parent.value),
        )
      ) {
        settle(version)
        waitAgain(key)
        return null
      }
      settleEmpty(version, key)
      return null
    },
    [begin, settle, settleEmpty, toLast, waitAgain],
  )

  const complete = useCallback(
    (run: ResourceTransitionRun, value: T | null): boolean => {
      if (!isCurrent(run)) {
        return false
      }
      setState({
        key: run.key,
        owner: run.generation,
        value,
        last: null,
        keepable: true,
        loading: false,
        error: null,
      })
      return true
    },
    [isCurrent],
  )

  /** fail reports error, moving the value to last. */
  const fail = useCallback(
    (run: ResourceTransitionRun, error: Error): boolean => {
      if (!isCurrent(run)) {
        return false
      }
      toLast(run.key, false, error)
      return true
    },
    [isCurrent, toLast],
  )

  /**
   * disposed records that run disposed the handles its value holds. If that
   * value is still held, it may stay while loading, as before, but never
   * becomes last.
   */
  const disposed = useCallback((run: ResourceTransitionRun) => {
    setState((prev) => {
      if (prev.owner !== run.generation || !prev.keepable) {
        return prev
      }
      if (prev.value === null) {
        return prev.last === null ? prev : { ...prev, last: null }
      }
      return { ...prev, keepable: false }
    })
  }, [])

  return {
    state,
    isPending,
    isCurrent,
    waitAgain,
    restart,
    settleEmpty,
    startAfter,
    complete,
    fail,
    disposed,
  }
}
