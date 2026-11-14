import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useSyncExternalStore,
} from 'react'
import {
  SharedResources,
  sharedResourceKey,
  type SharedParent,
} from './SharedResources.js'
import { useResourcesContext } from './ResourcesContext.js'
import { useResourceDevToolsContext } from './ResourceDevToolsContext.js'
import type { RegisterCleanup, Resource } from './useResource.js'
import { resourceOf, resourceReady } from './resourceTransitionState.js'

// detachedShared serves components rendered outside a ResourcesProvider,
// such as isolated tests. It has no client, so it does not follow releases.
const detachedShared = new SharedResources(null)

// nextTrackingId numbers DevTools tracking ids, like useResource does.
let nextTrackingId = 1

/** useSharedResources returns the shared mounts of the current client. */
export function useSharedResources(): SharedResources {
  return useResourcesContext()?.shared ?? detachedShared
}

/** UseSharedResourceOptions configures useSharedResource. */
export interface UseSharedResourceOptions<T> {
  /**
   * generation remounts the entry once when it differs from the generation
   * the entry was mounted under. Empty accepts any generation.
   */
  generation?: string
  /** onSuccess runs when this component first sees each loaded value. */
  onSuccess?: (value: T) => void
}

// noSubscription is the subscription of a hook with no key.
function noSubscription(): () => void {
  return () => {}
}

/**
 * useSharedResource mounts the child named name under parent once for every
 * component that asks for it. A component that arrives while the mount is
 * held renders its value on the first pass instead of mounting again.
 *
 * The mount must depend only on parent and name: a component that passes a
 * different mount under the same key gets the value of the first. A null
 * name disables the hook. The value belongs to the shared entry; mount
 * registers its handles through cleanup and consumers never dispose it.
 */
export function useSharedResource<P extends SharedParent, T>(
  parent: Resource<P>,
  name: string | null,
  mount: (
    parent: P,
    signal: AbortSignal,
    cleanup: RegisterCleanup,
  ) => Promise<T | null>,
  options?: UseSharedResourceOptions<T>,
): Resource<T> {
  // Key the entry under a ready parent; a released parent has no key yet.
  const shared = useSharedResources()
  const parentValue = resourceReady(parent) ? parent.value : null
  const parentReleased = parent.value?.released ?? false
  const key =
    parentValue && name !== null ? sharedResourceKey(parentValue, name) : null
  const generation = options?.generation ?? ''

  // Read the entry synchronously so a held mount renders on the first pass.
  const subscribe = useCallback(
    (cb: () => void) => (key ? shared.subscribe(key, cb) : noSubscription()),
    [key, shared],
  )
  const getSnapshot = useCallback(
    () => (key ? shared.getSnapshot<T>(key) : null),
    [key, shared],
  )
  const snapshot = useSyncExternalStore(subscribe, getSnapshot, getSnapshot)

  // Lease the entry while mounted; the latest mount serves later remounts.
  const mountRef = useRef(mount)
  useLayoutEffect(() => {
    mountRef.current = mount
  })
  useLayoutEffect(() => {
    if (!key || !parentValue) return
    return shared.acquire(
      key,
      (signal, cleanup) => mountRef.current(parentValue, signal, cleanup),
      parentValue,
      generation,
    )
  }, [key, parentValue, generation, shared])

  // Report each loaded value to onSuccess.
  const value = snapshot?.value ?? null
  const onSuccessRef = useRef(options?.onSuccess)
  useLayoutEffect(() => {
    onSuccessRef.current = options?.onSuccess
  })
  useEffect(() => {
    if (value !== null) {
      onSuccessRef.current?.(value)
    }
  }, [value])

  // Retry failed parents, then remount this entry.
  const parentError = parent.error
  const parentRetry = parent.retry
  const retry = useCallback(() => {
    if (parentError) {
      parentRetry()
      return
    }
    if (key) {
      shared.remount(key)
    }
  }, [key, parentError, parentRetry, shared])

  // Combine the parent's state with the entry's.
  const loading =
    parent.loading || parentReleased || (snapshot?.loading ?? false)
  const error = parent.error ?? snapshot?.error ?? null
  const devtoolsId = useSharedResourceDevTools(
    parent,
    retry,
    loading,
    value,
    error,
  )

  // A shared entry owns its value's handles, so a failure keeps no last.
  return useMemo(
    () =>
      resourceOf<T>({
        value,
        loading,
        error,
        retry,
        __devtools: devtoolsId ? { id: devtoolsId } : undefined,
      }),
    [value, loading, error, retry, devtoolsId],
  )
}

// useSharedResourceDevTools registers the hook with the resource DevTools
// when they are open and returns its tracking id.
function useSharedResourceDevTools(
  parent: Resource<unknown>,
  retry: () => void,
  loading: boolean,
  value: unknown,
  error: Error | null,
): string | null {
  // Assign this hook a stable tracking id.
  const devtools = useResourceDevToolsContext()
  const idRef = useRef<string | null>(null)
  if (idRef.current === null) {
    idRef.current = `_s_${nextTrackingId++}_`
  }
  const id = idRef.current
  const parentId = parent.__devtools?.id

  // Register under the parent and report each state change.
  const retryRef = useRef(retry)
  useLayoutEffect(() => {
    retryRef.current = retry
  })
  useEffect(() => {
    if (!devtools) return
    devtools.register(id, parentId ? [parentId] : [], () => retryRef.current())
    return () => devtools.unregister(id)
  }, [devtools, id, parentId])
  useEffect(() => {
    if (!devtools) return
    const state = loading ? 'loading' : error ? 'error' : 'ready'
    devtools.update(id, state, value, error)
  }, [devtools, id, loading, value, error, parentId])

  return devtools ? id : null
}
