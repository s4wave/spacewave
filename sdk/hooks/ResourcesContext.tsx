import React, {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useState,
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
  const [generation, setGeneration] = useState(
    () => client?.connectionGeneration ?? 0,
  )
  useEffect(() => {
    if (!client) return
    // Sync in case generation changed before subscription
    setGeneration(client.connectionGeneration)
    return client.onConnectionLost(() => {
      setGeneration(client.connectionGeneration)
    })
  }, [client])
  return generation
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
