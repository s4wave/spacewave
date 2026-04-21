import React, {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useSyncExternalStore,
  ReactNode,
} from 'react'
import { Client as ResourceClient } from '../resource/client.js'

interface ResourcesContextValue {
  client: ResourceClient | null
}

const ResourcesContext = createContext<ResourcesContextValue | undefined>(
  undefined,
)

export function useResourcesContext(): ResourcesContextValue | undefined {
  return useContext(ResourcesContext)
}

/**
 * Hook that tracks the resource client's connection generation.
 * Returns a counter that increments each time the connection is lost and
 * resources are released. Use this as a dependency in useResource to trigger
 * re-creation of resources after reconnection.
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
  children: ReactNode
  client: ResourceClient | null
}

export function ResourcesProvider({
  children,
  client,
}: ResourcesProviderProps) {
  const value = useMemo(() => ({ client }), [client])

  return (
    <ResourcesContext.Provider value={value}>
      {children}
    </ResourcesContext.Provider>
  )
}
