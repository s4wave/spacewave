import {
  useCallback,
  useEffect,
  useEffectEvent,
  useMemo,
  useRef,
  useState,
} from 'react'
import type { ResourceReleaseReason } from '../resource/client.js'
import { Resource as SDKResource } from '../resource/resource.js'
import { castToError } from 'starpc'
import { useResourceDevToolsContext } from './ResourceDevToolsContext.js'
import { useResourcesClient } from './useResourcesClient.js'
import {
  lastOf,
  resourceOf,
  useResourceTransitionState,
  useResourceTransitionVersion,
  type ResourceTransitionRun,
} from './resourceTransitionState.js'

// Global counter for generating unique tracking IDs.
// useId() is not suitable because it generates IDs based on component tree position,
// which can cause duplicates when components unmount/remount in different tree positions
// (e.g., when switching FlexLayout tabs).
let nextTrackingId = 1

// useTrackingId returns a stable, globally unique ID for resource tracking.
function useTrackingId(): string {
  const idRef = useRef<string | null>(null)
  if (idRef.current === null) {
    idRef.current = `_r_${nextTrackingId++}_`
  }
  return idRef.current
}

/**
 * Function type for registering resources that need cleanup.
 * Called during resource loading to track resources that should be
 * released when the component unmounts or the resource is reloaded.
 * Returns the resource for convenient chaining.
 */
export type RegisterCleanup = <
  T extends { [Symbol.dispose](): void } | null | undefined,
>(
  resource: T,
) => T

/**
 * Configuration options for useResource hook.
 */
export interface UseResourceOptions<TValue> {
  /** If false, the resource won't be loaded (useful for conditional loading) */
  enabled?: boolean
  /** Callback invoked when the resource loads successfully */
  onSuccess?: (data: TValue) => void
  /** Callback invoked when the resource fails to load */
  onError?: (error: Error) => void
  /** Retry when a loaded SDK resource is later released by the server. Defaults to true. */
  retryOnReleasedResource?:
    | boolean
    | {
        /** Release reasons that should trigger a retry. Defaults to ['server-released']. */
        reasons?: ResourceReleaseReason[]
        /** Extract resource IDs from non-Resource or composite return values. */
        getResourceIds?: (value: TValue) => number[]
      }
}

/** ResourceBase holds the fields every Resource state carries. */
interface ResourceBase {
  /** Function to retry loading the resource */
  retry: () => void
  /** DevTools tracking metadata - only present when DevTools context is active */
  __devtools?: { id: string }
}

/**
 * ResourceLoading is a resource being loaded. value is the previous value
 * while the resource loads again for the same dependencies, or null.
 */
export interface ResourceLoading<T> extends ResourceBase {
  value: T | null
  loading: true
  error: null
}

/** ResourceSettled is a loaded resource; value is null when it has none. */
export interface ResourceSettled<T> extends ResourceBase {
  value: T | null
  loading: false
  error: null
}

/**
 * ResourceFailed is a resource whose load, or whose parent's load, failed.
 * value is always null, so a caller that reads only value draws nothing.
 * last is the most recent value for the current dependencies, kept for a
 * caller that draws the failure over what had arrived. It is never a released
 * handle, and a change of dependencies clears it.
 */
export interface ResourceFailed<T> extends ResourceBase {
  value: null
  loading: false
  error: Error
  last: T | null
}

/**
 * Resource is the state of a resource hook. No state has a value beside an
 * error, and only the failed state carries last.
 */
export type Resource<T> = ResourceLoading<T> | ResourceSettled<T> | ResourceFailed<T>

/** ResourceFields are the loose fields resourceOf builds a Resource from. */
export interface ResourceFields<T> extends ResourceBase {
  value: T | null
  loading: boolean
  error: Error | null
  last?: T | null
}


type NoParentFactory<T> = (
  signal: AbortSignal,
  cleanup: RegisterCleanup,
) => Promise<T | null>

type SingleParentFactory<P, T> = (
  parent: P,
  signal: AbortSignal,
  cleanup: RegisterCleanup,
) => Promise<T | null>

type MultiParentFactory<P extends readonly Resource<unknown>[], T> = (
  parents: {
    readonly [K in keyof P]: P[K] extends Resource<infer V> ? V : never
  },
  signal: AbortSignal,
  cleanup: RegisterCleanup,
) => Promise<T | null>

// Internal parsed args - types are erased at runtime, safety is enforced at public API
type ParsedArgs<T> = {
  type: 'no-parent' | 'single-parent' | 'multi-parent'
  factory: unknown
  deps: React.DependencyList
  options?: UseResourceOptions<T>
  parents: Resource<unknown>[]
}

// parseArgs parses and validates the arguments to useResource.
function parseArgs<T>(args: unknown[]): ParsedArgs<T> {
  const [firstArg, secondArg, thirdArg, fourthArg] = args

  if (typeof firstArg === 'function') {
    // No parent: factory, deps, options?
    if (!Array.isArray(secondArg)) {
      throw new Error('useResource: deps array is required as second argument')
    }
    return {
      type: 'no-parent',
      factory: firstArg,
      deps: secondArg,
      options: thirdArg as UseResourceOptions<T> | undefined,
      parents: [],
    }
  }

  if (Array.isArray(firstArg)) {
    // Multi-parent: parents, factory, deps, options?
    if (!Array.isArray(thirdArg)) {
      throw new Error('useResource: deps array is required as third argument')
    }
    return {
      type: 'multi-parent',

      factory: secondArg,
      deps: thirdArg,
      options: fourthArg as UseResourceOptions<T> | undefined,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      parents: firstArg as Resource<any>[],
    }
  }

  // Single parent: parent, factory, deps, options?
  if (!Array.isArray(thirdArg)) {
    throw new Error('useResource: deps array is required as third argument')
  }
  return {
    type: 'single-parent',

    factory: secondArg,
    deps: thirdArg,
    options: fourthArg as UseResourceOptions<T> | undefined,
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    parents: [firstArg as Resource<any>],
  }
}

// useParentState extracts and memoizes parent resource state.
function useParentState(parents: Resource<unknown>[]) {
  // Extract values from parent Resources - must depend on actual values, not just Resource identity
  // Otherwise we won't detect when a parent loads and its .value changes from null to something
  const parentValues = parents.map((p) => p.value)
  const values = useMemo(
    () => parentValues,
    // eslint-disable-next-line react-hooks/exhaustive-deps
    parentValues,
  )
  const loading = parents.some((p) => p.loading)
  const error = parents.find((p) => p.error)?.error ?? null
  // Only failed parents are retried: retrying a healthy parent would reload
  // the whole chain up to the root.
  const parentRetryDeps = parents.flatMap((p) => [p.retry, p.error != null])
  const failedRetries = useMemo(
    () => parents.filter((p) => p.error != null).map((p) => p.retry),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    parentRetryDeps,
  )
  return { values, loading, error, failedRetries }
}

// callFactory invokes the factory function with appropriate arguments.
async function callFactory<T>(
  parsed: ParsedArgs<T>,
  parentValues: unknown[],
  signal: AbortSignal,
  cleanup: RegisterCleanup,
): Promise<T | null> {
  if (parsed.type === 'no-parent') {
    return (parsed.factory as NoParentFactory<T>)(signal, cleanup)
  }

  if (parsed.type === 'single-parent') {
    return (parsed.factory as SingleParentFactory<unknown, T>)(
      parentValues[0],
      signal,
      cleanup,
    )
  }

  return (
    parsed.factory as MultiParentFactory<readonly Resource<unknown>[], T>
  )(parentValues, signal, cleanup)
}

function getReleasedResourceRetryReasons<T>(
  options?: UseResourceOptions<T>,
): ResourceReleaseReason[] {
  const retryConfig = options?.retryOnReleasedResource
  if (retryConfig === false) {
    return []
  }
  if (retryConfig === undefined || retryConfig === true) {
    return ['server-released']
  }
  return retryConfig.reasons ?? ['server-released']
}

function getReleasedResourceRetryIds<T>(
  value: T,
  options?: UseResourceOptions<T>,
): number[] {
  const retryConfig = options?.retryOnReleasedResource
  if (retryConfig === false) {
    return []
  }
  if (retryConfig && retryConfig !== true && retryConfig.getResourceIds) {
    return retryConfig.getResourceIds(value)
  }
  return value instanceof SDKResource ? [value.id] : []
}

/**
 * useResource manages async resource loading with automatic cleanup and dependency management.
 *
 * @example
 * ```tsx
 * // Without parent dependency:
 * const root = useResource(
 *   async (signal, cleanup) => {
 *     const ref = await client.accessRootResource()
 *     return cleanup(new Root(ref))
 *   },
 *   [], // deps array is required
 * )
 *
 * // With single parent dependency - types are inferred:
 * const session = useResource(
 *   root,
 *   async (rootValue, signal, cleanup) =>
 *     rootValue ?
 *       cleanup(await rootValue.mountSession({}, signal))
 *     : null,
 *   [], // deps array is required
 * )
 *
 * // With multiple parent dependencies - automatic type inference:
 * const objectInfo = useResource(
 *   [spaceWorld, objectState] as const,
 *   async ([world, obj], signal) => {
 *     if (!world || !obj) return null
 *     const key = obj.getKey()
 *     const type = await getObjectType(world, key, signal)
 *     return { key, type }
 *   },
 *   [], // deps array is required
 * )
 *
 * // With dependencies (e.g., accessing component state/props):
 * const objectState = useResource(
 *   spaceWorldResource,
 *   async (world, signal, cleanup) =>
 *     world && objectKey ?
 *       cleanup(await world.getObject(objectKey, signal))
 *     : null,
 *   [objectKey], // factory depends on objectKey
 * )
 *
 * // Direct property access:
 * if (root.loading) return <Loading />
 * if (root.error) return <Error error={root.error} />
 * if (!root.value) return <NoData />
 * return <div>{root.value.id}</div>
 *
 * // Parent state is inherited:
 * // - If parent is loading, child loading = true
 * // - If parent has error, child error = parent.error
 * // - Calling retry() retries the child and any parent in an error state
 * ```
 */
export function useResource<T>(
  factory: (signal: AbortSignal, cleanup: RegisterCleanup) => Promise<T | null>,
  deps: React.DependencyList,
  options?: UseResourceOptions<T>,
): Resource<T>
export function useResource<P, T>(
  parent: Resource<P>,
  factory: (
    parent: P,
    signal: AbortSignal,
    cleanup: RegisterCleanup,
  ) => Promise<T | null>,
  deps: React.DependencyList,
  options?: UseResourceOptions<T>,
): Resource<T>
export function useResource<
  const P extends readonly Resource<unknown>[],
  T = unknown,
>(
  parents: P,
  factory: (
    parents: {
      readonly [K in keyof P]: P[K] extends Resource<infer V> ? V : never
    },
    signal: AbortSignal,
    cleanup: RegisterCleanup,
  ) => Promise<T | null>,
  deps: React.DependencyList,
  options?: UseResourceOptions<T>,
): Resource<T>
export function useResource<T>(
  ...args:
    | [
        factory: (
          signal: AbortSignal,
          cleanup: RegisterCleanup,
        ) => Promise<T | null>,
        deps: React.DependencyList,
        options?: UseResourceOptions<T>,
      ]
    | [
        parent: Resource<unknown>,
        factory: (
          parent: unknown,
          signal: AbortSignal,
          cleanup: RegisterCleanup,
        ) => Promise<T | null>,
        deps: React.DependencyList,
        options?: UseResourceOptions<T>,
      ]
    | [
        parents: Resource<unknown>[],
        factory: (
          parents: unknown[],
          signal: AbortSignal,
          cleanup: RegisterCleanup,
        ) => Promise<T | null>,
        deps: React.DependencyList,
        options?: UseResourceOptions<T>,
      ]
): Resource<T> {
  const [firstArg, secondArg, thirdArg, fourthArg] = args

  // DevTools tracking - get stable ID and context
  const trackingId = useTrackingId()
  const devtools = useResourceDevToolsContext()
  const resourcesClient = useResourcesClient()

  const parsed = useMemo<ParsedArgs<T>>(
    () => parseArgs<T>(args),
    // Using deconstructed args to avoid whole array changing identity
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [firstArg, secondArg, thirdArg, fourthArg],
  )
  const enabled = parsed.options?.enabled ?? true

  const parent = useParentState(parsed.parents)

  const parentValuesChangeCount = useResourceTransitionVersion(parent.values)

  // Wrap factory with useCallback to ensure stable identity between renders
  // When deps change, useCallback returns a new function, triggering a reload
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const stableFactory = useCallback(
    parsed.factory as (...args: unknown[]) => unknown,
    parsed.deps,
  )

  // key changes with the dependencies: a value never outlives its key.
  const key = useResourceTransitionVersion([stableFactory])
  const transitionVersion = useResourceTransitionVersion([
    stableFactory,
    parentValuesChangeCount,
  ])
  const {
    state,
    startAfter,
    complete,
    fail,
    disposed,
    isPending,
    restart,
    settleEmpty,
  } = useResourceTransitionState<T>(enabled && !parent.loading, key)
  const effectiveLoading = isPending(transitionVersion) || state.loading
  const [retryCount, setRetryCount] = useState(0)

  const onSuccess = useEffectEvent((data: T | null) => {
    if (data !== null) {
      parsed.options?.onSuccess?.(data)
    }
  })

  const onError = useEffectEvent((error: Error) => {
    parsed.options?.onError?.(error)
  })

  // Retry reads again. A failure that kept a last value stays failed over it
  // until the new read settles, so a Retry press never blanks what the person
  // was reading; otherwise the value moves to last and the read shows loading.
  // quietRef holds that choice until the run it starts settles.
  const kept = state.error !== null && lastOf(state) !== null
  const quietRef = useRef(false)
  const retry = useCallback(() => {
    parent.failedRetries.forEach((r) => r())
    if (kept) {
      quietRef.current = true
    } else {
      restart(key)
    }
    setRetryCount((c) => c + 1)
  }, [parent.failedRetries, restart, key, kept])

  const releasedResourceRetryReasons = useMemo(
    () => getReleasedResourceRetryReasons(parsed.options),
    [parsed.options],
  )
  const releasedResourceRetryIds = useMemo(() => {
    if (!enabled || state.value === null) {
      return []
    }
    return getReleasedResourceRetryIds(state.value, parsed.options)
  }, [enabled, state.value, parsed.options])
  const pendingReleasedResourceRetryRef = useRef(false)

  const activeRunRef = useRef<ResourceTransitionRun | null>(null)

  const parentResourceIds = useMemo(
    () =>
      parent.values.flatMap((value) =>
        value instanceof SDKResource ? [value.id] : [],
      ),
    [parent.values],
  )

  useEffect(() => {
    pendingReleasedResourceRetryRef.current = false
  }, [retryCount, state.value])

  useEffect(() => {
    if (!resourcesClient || parentResourceIds.length === 0) return

    return resourcesClient.onResourceReleased((event) => {
      if (
        event.reason !== 'server-released' &&
        event.reason !== 'connection-lost' &&
        event.reason !== 'client-released'
      ) {
        return
      }
      if (!parentResourceIds.includes(event.resourceId)) {
        return
      }
      activeRunRef.current?.abort()
    })
  }, [parentResourceIds, resourcesClient])

  // DevTools: Extract parent tracking IDs from parent Resource objects.
  // Use spread-deps pattern (like useParentState's failedRetries) so the array
  // reference is stable when the actual IDs haven't changed, even if
  // parsed.parents is a new array (e.g. when factory changes reference).
  const parentTrackingIdValues = parsed.parents.flatMap((p) => {
    const id = p.__devtools?.id
    return id == null ? [] : [id]
  })
  const parentTrackingIds = useMemo(
    () => parentTrackingIdValues,
    // eslint-disable-next-line react-hooks/exhaustive-deps
    parentTrackingIdValues,
  )

  // DevTools: Store retry in a ref so we can update it without re-registering
  const retryRef = useRef(retry)
  retryRef.current = retry

  // DevTools: Store current state in ref for access without adding to effect deps
  const stateRef = useRef(state)
  stateRef.current = state

  // DevTools: Register on mount, unregister on unmount
  // Separate from state updates to avoid clearing selection on state changes
  useEffect(() => {
    if (!devtools) return
    devtools.register(trackingId, parentTrackingIds, () => retryRef.current())
    // Immediately update with current state after registration
    const s = stateRef.current
    const currentState = s.loading ? 'loading' : s.error ? 'error' : 'ready'
    devtools.update(trackingId, currentState, s.value, s.error)
    return () => {
      devtools.unregister(trackingId)
    }
  }, [devtools, trackingId, parentTrackingIds])

  // DevTools: Update state when it changes (does not trigger unregister/re-register)
  const prevStateRef = useRef({
    loading: state.loading,
    error: state.error,
    value: state.value,
  })
  useEffect(() => {
    if (!devtools) return
    // Only update if state actually changed (not just on mount)
    const prev = prevStateRef.current
    if (
      prev.loading === state.loading &&
      prev.error === state.error &&
      prev.value === state.value
    ) {
      return
    }
    prevStateRef.current = {
      loading: state.loading,
      error: state.error,
      value: state.value,
    }
    const currentState = state.loading
      ? 'loading'
      : state.error
        ? 'error'
        : 'ready'
    devtools.update(trackingId, currentState, state.value, state.error)
  }, [devtools, trackingId, state.loading, state.error, state.value])

  const parentFailed = parent.error !== null
  useEffect(() => {
    if (!enabled) {
      settleEmpty(transitionVersion, key)
      return
    }
    const started = startAfter(
      parsed.parents,
      transitionVersion,
      key,
      quietRef.current,
    )
    if (!started) {
      return
    }
    const run: ResourceTransitionRun = started

    const cleanupResources: { [Symbol.dispose](): void }[] = []
    activeRunRef.current = run
    let cleanedUp = false

    const registerCleanup: RegisterCleanup = (resource) => {
      if (!resource) return resource
      if (cleanedUp) {
        queueMicrotask(() => resource[Symbol.dispose]())
        return resource
      }
      cleanupResources.push(resource)
      return resource
    }

    const disposeAll = () => {
      cleanedUp = true
      cleanupResources.forEach((r) => r[Symbol.dispose]())
    }
    async function load() {
      try {
        const result = await callFactory(
          parsed,
          parent.values,
          run.signal,
          registerCleanup,
        )

        if (!complete(run, result)) {
          disposeAll()
          return
        }

        onSuccess(result)
      } catch (err) {
        disposeAll()

        const errorObj = castToError(err)
        if (fail(run, errorObj)) {
          onError(errorObj)
        }
      } finally {
        if (activeRunRef.current === run) {
          activeRunRef.current = null
          quietRef.current = false
        }
      }
    }

    void load()

    return () => {
      run.abort()
      if (activeRunRef.current === run) {
        activeRunRef.current = null
      }
      const owned = cleanupResources.length !== 0
      disposeAll()
      if (owned) {
        disposed(run)
      }
    }
    // Intentionally excluding parsed, parent.values, onSuccess, onError from deps:
    // - stableFactory: Changes when deps change (via useCallback), and so key
    // - parent.values: Changes tracked via parentValuesChangeCount
    // - onSuccess/onError: useEffectEvent makes them stable
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    retryCount,
    enabled,
    parent.loading,
    parentFailed,
    transitionVersion,
    key,
    startAfter,
    complete,
    fail,
    disposed,
    settleEmpty,
  ])

  useEffect(() => {
    if (
      !resourcesClient ||
      state.loading ||
      releasedResourceRetryReasons.length === 0 ||
      releasedResourceRetryIds.length === 0
    ) {
      return
    }

    return resourcesClient.onResourceReleased((event) => {
      if (!releasedResourceRetryReasons.includes(event.reason)) {
        return
      }
      if (!releasedResourceRetryIds.includes(event.resourceId)) {
        return
      }
      if (pendingReleasedResourceRetryRef.current) {
        return
      }

      pendingReleasedResourceRetryRef.current = true
      retry()
    })
  }, [
    releasedResourceRetryIds,
    releasedResourceRetryReasons,
    resourcesClient,
    retry,
    state.loading,
  ])

  // Report the parent's error ahead of the resource's own.
  return useMemo(
    () =>
      resourceOf<T>({
        value: enabled ? state.value : null,
        last: enabled ? lastOf(state) : null,
        loading: enabled && (parent.loading || effectiveLoading),
        error: enabled ? parent.error || state.error : null,
        retry,
        __devtools: devtools ? { id: trackingId } : undefined,
      }),
    [
      enabled,
      state,
      effectiveLoading,
      parent.loading,
      parent.error,
      retry,
      devtools,
      trackingId,
    ],
  )
}

/**
 * Extracts values from multiple resources as a tuple.
 * Returns null if any resource is not ready (loading, error, or no value).
 *
 * @example
 * ```tsx
 * const [root, session, space] = useResourceResults(rootResource, sessionResource, spaceResource) ?? []
 * if (!root || !session || !space) return <Loading />
 * ```
 */
export function useResourceResults<T extends unknown[]>(
  ...resources: { [K in keyof T]: Resource<T[K]> }
): T | null {
  return useMemo(() => {
    if (resources.some((r) => r.loading || r.error || r.value === null)) {
      return null
    }
    return resources.map((r) => r.value) as T
  }, [resources])
}

/**
 * Extracts the value from a resource with simplified semantics.
 * Returns the value when ready, null while loading, undefined if error occurred.
 * A failed resource's last value is not returned: undefined means failed.
 *
 * @example
 * ```tsx
 * const root = useResourceValue(rootResource)
 * if (root === null) return <Loading />
 * if (root === undefined) return <Error />
 * return <div>{root.id}</div>
 * ```
 */
export function useResourceValue<T>(
  resource: Resource<T>,
): T | null | undefined {
  if (resource.error) return undefined
  if (resource.loading || resource.value === null) return null
  return resource.value
}
