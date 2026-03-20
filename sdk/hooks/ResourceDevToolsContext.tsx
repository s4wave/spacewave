import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
  type ReactNode,
} from 'react'
import type { ResourceDebugInfo } from '../resource/resource.js'

export type TrackingId = string

export type ResourceState = 'loading' | 'ready' | 'error'

export interface TrackedResource {
  id: TrackingId
  resourceId: number | null
  resourceType: string | null
  released: boolean
  parentIds: TrackingId[]
  state: ResourceState
  error: Error | null
  retry: () => void
  createdAt: number
  debugLabel?: string
  debugDetails?: Record<string, string | number | boolean | null>
}

export interface ResourceDevToolsContextValue {
  register: (id: TrackingId, parentIds: TrackingId[], retry: () => void) => void
  update: (
    id: TrackingId,
    state: ResourceState,
    value: unknown,
    error: Error | null,
  ) => void
  unregister: (id: TrackingId) => void
  subscribe: (callback: () => void) => () => void
  subscribeSelectedId: (callback: () => void) => () => void
  getResources: () => Map<TrackingId, TrackedResource>
  getSelectedId: () => TrackingId | null
  setSelectedId: (id: TrackingId | null) => void
}

const ResourceDevToolsContext =
  createContext<ResourceDevToolsContextValue | null>(null)

// extractResourceType extracts the constructor name from a resource value.
function extractResourceType(value: unknown): string | null {
  if (!value || typeof value !== 'object') return null
  return value.constructor?.name ?? null
}

// extractResourceId extracts the resourceId from a resource value.
function extractResourceId(value: unknown): number | null {
  if (!value || typeof value !== 'object') return null
  const ref = (value as { resourceRef?: { resourceId?: number } }).resourceRef
  return ref?.resourceId ?? null
}

// extractReleased extracts the released status from a resource value.
function extractReleased(value: unknown): boolean {
  if (!value || typeof value !== 'object') return false
  const ref = (value as { resourceRef?: { released?: boolean } }).resourceRef
  return ref?.released ?? false
}

// extractDebugInfo extracts debug info from a resource value if available.
function extractDebugInfo(value: unknown): ResourceDebugInfo | null {
  if (!value || typeof value !== 'object') return null
  const resource = value as { getDebugInfo?: () => ResourceDebugInfo }
  return resource.getDebugInfo?.() ?? null
}

// hasWatchDebugInfo checks if a resource has a watchDebugInfo method.
function hasWatchDebugInfo(
  value: unknown,
): value is { watchDebugInfo: () => AsyncIterable<ResourceDebugInfo> } {
  if (!value || typeof value !== 'object') return false
  return (
    typeof (value as { watchDebugInfo?: unknown }).watchDebugInfo === 'function'
  )
}

// ResourceDevToolsProvider provides resource tracking for DevTools.
// Always renders the same element structure to avoid remounting children when enabled changes.
export function ResourceDevToolsProvider({
  children,
  enabled = true,
}: {
  children: ReactNode
  enabled?: boolean
}) {
  return (
    <ResourceDevToolsProviderInner enabled={enabled}>
      {children}
    </ResourceDevToolsProviderInner>
  )
}

function ResourceDevToolsProviderInner({
  children,
  enabled,
}: {
  children: ReactNode
  enabled: boolean
}) {
  const [resources, setResources] = useState(
    () => new Map<TrackingId, TrackedResource>(),
  )
  const [selectedId, setSelectedIdState] = useState<TrackingId | null>(null)

  // Use a ref to store resources for getSnapshot.
  // This ensures getSnapshot always returns the same reference until state changes.
  const resourcesRef = useRef(resources)
  resourcesRef.current = resources

  // Subscription system for components that need to re-render on resource changes.
  const subscribersRef = useRef(new Set<() => void>())

  const subscribe = useCallback((callback: () => void) => {
    subscribersRef.current.add(callback)
    return () => subscribersRef.current.delete(callback)
  }, [])

  // Notify subscribers synchronously - they will then call getSnapshot
  // which will return the updated resourcesRef.current
  const notifySubscribers = useCallback(() => {
    subscribersRef.current.forEach((cb) => cb())
  }, [])

  // Notify subscribers whenever resources change
  useEffect(() => {
    notifySubscribers()
  }, [resources, notifySubscribers])

  // Subscription system for selectedId changes
  const selectedIdSubscribersRef = useRef(new Set<() => void>())

  // Track AbortControllers for watchDebugInfo subscriptions
  const watchAbortControllersRef = useRef(
    new Map<TrackingId, AbortController>(),
  )

  const subscribeSelectedId = useCallback((callback: () => void) => {
    selectedIdSubscribersRef.current.add(callback)
    return () => selectedIdSubscribersRef.current.delete(callback)
  }, [])

  const notifySelectedIdSubscribers = useCallback(() => {
    selectedIdSubscribersRef.current.forEach((cb) => cb())
  }, [])

  // Store selectedId in a ref for stable access via getSelectedId
  const selectedIdRef = useRef(selectedId)
  selectedIdRef.current = selectedId

  // Notify selectedId subscribers when it changes
  useEffect(() => {
    notifySelectedIdSubscribers()
  }, [selectedId, notifySelectedIdSubscribers])

  // Wrapper to update both state and ref
  const setSelectedId = useCallback((id: TrackingId | null) => {
    setSelectedIdState(id)
  }, [])

  const register = useCallback(
    (id: TrackingId, parentIds: TrackingId[], retry: () => void) => {
      setResources((prev) => {
        const existing = prev.get(id)
        const next = new Map(prev)
        // Preserve existing state/resourceId/resourceType/debug info if re-registering
        // This can happen when parentTrackingIds changes but the resource
        // has already loaded
        next.set(id, {
          id,
          resourceId: existing?.resourceId ?? null,
          resourceType: existing?.resourceType ?? null,
          released: existing?.released ?? false,
          parentIds,
          state: existing?.state ?? 'loading',
          error: existing?.error ?? null,
          retry,
          createdAt: existing?.createdAt ?? Date.now(),
          debugLabel: existing?.debugLabel,
          debugDetails: existing?.debugDetails,
        })
        return next
      })
    },
    [],
  )

  const update = useCallback(
    (
      id: TrackingId,
      state: ResourceState,
      value: unknown,
      error: Error | null,
    ) => {
      // Extract initial debug info
      const debugInfo = extractDebugInfo(value)

      setResources((prev) => {
        const existing = prev.get(id)
        if (!existing) return prev

        const next = new Map(prev)
        next.set(id, {
          ...existing,
          state,
          error,
          resourceType: extractResourceType(value),
          resourceId: extractResourceId(value),
          released: extractReleased(value),
          debugLabel: debugInfo?.label,
          debugDetails: debugInfo?.details,
        })
        return next
      })

      // Subscribe to watchDebugInfo if available and not already subscribed
      if (
        hasWatchDebugInfo(value) &&
        !watchAbortControllersRef.current.has(id)
      ) {
        const abortController = new AbortController()
        watchAbortControllersRef.current.set(id, abortController)

        // Start watching in background
        void (async () => {
          try {
            for await (const info of value.watchDebugInfo()) {
              if (abortController.signal.aborted) break
              setResources((prev) => {
                const existing = prev.get(id)
                if (!existing) return prev
                const next = new Map(prev)
                next.set(id, {
                  ...existing,
                  debugLabel: info.label,
                  debugDetails: info.details,
                })
                return next
              })
            }
          } catch {
            // Ignore errors from aborted iteration
          }
        })()
      }
    },
    [],
  )

  const unregister = useCallback((id: TrackingId) => {
    // Abort any watch subscription for this resource
    const abortController = watchAbortControllersRef.current.get(id)
    if (abortController) {
      abortController.abort()
      watchAbortControllersRef.current.delete(id)
    }

    setResources((prev) => {
      const next = new Map(prev)
      next.delete(id)
      return next
    })
    setSelectedIdState((prev) => (prev === id ? null : prev))
  }, [])

  // getResources returns the current resources map.
  const getResources = useCallback(() => resourcesRef.current, [])

  // Stable context value - does NOT include selectedId to avoid re-render cascade
  const contextValue = useMemo(
    () => ({
      register,
      update,
      unregister,
      subscribe,
      subscribeSelectedId,
      getResources,
      getSelectedId: () => selectedIdRef.current,
      setSelectedId,
    }),
    [
      register,
      update,
      unregister,
      subscribe,
      subscribeSelectedId,
      getResources,
      setSelectedId,
    ],
  )

  // Provide null context when disabled so hooks gracefully degrade
  return (
    <ResourceDevToolsContext.Provider value={enabled ? contextValue : null}>
      {children}
    </ResourceDevToolsContext.Provider>
  )
}

// useResourceDevToolsContext returns the DevTools context, or null if not available.
export function useResourceDevToolsContext() {
  return useContext(ResourceDevToolsContext)
}

// Empty map singleton for when devtools is not available
const EMPTY_RESOURCES_MAP = new Map<TrackingId, TrackedResource>()

// useTrackedResources subscribes to resource changes and returns the current resources.
// Use this in DevTools UI components that need to re-render when resources change.
export function useTrackedResources(): Map<TrackingId, TrackedResource> {
  const devtools = useResourceDevToolsContext()

  const subscribe = useCallback(
    (callback: () => void) => {
      if (!devtools) return () => {}
      return devtools.subscribe(callback)
    },
    [devtools],
  )

  const getSnapshot = useCallback(() => {
    if (!devtools) return EMPTY_RESOURCES_MAP
    return devtools.getResources()
  }, [devtools])

  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
}

// useSelectedResourceId subscribes to selectedId changes and returns the current value.
export function useSelectedResourceId(): TrackingId | null {
  const devtools = useResourceDevToolsContext()

  const subscribe = useCallback(
    (callback: () => void) => {
      if (!devtools) return () => {}
      return devtools.subscribeSelectedId(callback)
    },
    [devtools],
  )

  const getSnapshot = useCallback(() => {
    if (!devtools) return null
    return devtools.getSelectedId()
  }, [devtools])

  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
}

// useErrorCount returns the count of resources in error state.
export function useErrorCount(): number {
  const resources = useTrackedResources()
  return useMemo(
    () =>
      Array.from(resources.values()).filter((r) => r.state === 'error').length,
    [resources],
  )
}

// getResourceLabel returns a display label for a resource.
export function getResourceLabel(r: TrackedResource): string {
  if (r.state === 'loading') {
    // Show how long it's been loading for debugging stuck resources
    const elapsed = Date.now() - r.createdAt
    if (elapsed > 5000) {
      const secs = Math.floor(elapsed / 1000)
      return `(loading ${secs}s)`
    }
    return '(loading)'
  }
  if (r.state === 'error' && !r.resourceType) return '(error)'
  const type = r.resourceType ?? 'Resource'
  const id = r.resourceId != null ? ` #${r.resourceId}` : ''
  const label = r.debugLabel ? `: ${r.debugLabel}` : ''
  return `${type}${id}${label}`
}
