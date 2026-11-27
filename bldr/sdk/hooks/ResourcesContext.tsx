import React, {
  createContext,
  use,
  useEffect,
  useMemo,
  useSyncExternalStore,
  ReactNode,
} from 'react'
import { Client as ResourceClient } from '../resource/client.js'
import { SharedResources } from './SharedResources.js'

interface ResourcesContextValue {
  client: ResourceClient | null
  // shared holds the mounts components share through the client.
  shared: SharedResources
}

const ResourcesContext = createContext<ResourcesContextValue | undefined>(
  undefined,
)

export function useResourcesContext(): ResourcesContextValue | undefined {
  return use(ResourcesContext)
}

/**
 * useConnectionGeneration tracks the resource client's connection generation.
 * Returns a counter that increments each time the connection is lost and
 * resources are released. Use this counter as a dependency in useResource to
 * trigger re-creation of resources after reconnection.
 */
export function useConnectionGeneration(client: ResourceClient | null): number {
  return useSyncExternalStore(
    (onStoreChange) => {
      if (!client) {
        return () => {}
      }
      return client.onConnectionLost(() => {
        onStoreChange()
      })
    },
    () => client?.connectionGeneration ?? 0,
    () => client?.connectionGeneration ?? 0,
  )
}

interface ResourcesProviderProps {
  children?: ReactNode
  client: ResourceClient | null
}

export function ResourcesProvider({
  children,
  client,
}: ResourcesProviderProps) {
  const value = useMemo(
    () => ({ client, shared: new SharedResources(client) }),
    [client],
  )

  // Dispose the shared mounts of a client this provider no longer serves.
  useEffect(() => {
    return () => value.shared.clear()
  }, [value])

  return (
    <ResourcesContext.Provider value={value}>
      {children}
    </ResourcesContext.Provider>
  )
}
