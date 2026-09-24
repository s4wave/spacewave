import { Root } from '@s4wave/sdk/root'
import { Client as ResourceClient } from '@aptre/bldr-sdk/resource/client.js'
import {
  useResource,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { useConnectionGeneration } from '@aptre/bldr-sdk/hooks/ResourcesContext.js'
import { RootContext } from '@s4wave/web/contexts/contexts.js'

/**
 * Hook to access and manage the root resource lifecycle with a specific client.
 *
 * @param client - The ResourceClient instance to use
 * @returns Resource object containing the Root resource
 *
 * @example
 * ```tsx
 * function MyComponent() {
 *   const client = useResourcesClient()
 *   const root = useRootResourceWithClient(client)
 *
 *   if (root.loading) return <div>Loading root...</div>
 *   if (root.error) return <div>Error: {root.error.message}</div>
 *   if (!root.value) return null
 *
 *   // Use root to create sessions, etc.
 *   return <div>Root loaded!</div>
 * }
 * ```
 */
export function useRootResourceWithClient(
  client: ResourceClient | null,
): Resource<Root> {
  // Track connection generation so the root resource is re-created after reconnection.
  // When the connection drops, all server-side resources are invalidated.
  // This dep change cascades through the useResource parent tree,
  // causing all downstream resources to be re-created with fresh IDs.
  const generation = useConnectionGeneration(client)

  return useResource(
    async (signal, cleanup) => {
      if (!client) return null

      const ref = await client.accessRootResource()
      return cleanup(new Root(ref))
    },
    [client, generation],
  )
}

/**
 * Hook to access the root resource from RootContext.
 *
 * @returns Resource object containing the Root resource
 *
 * @example
 * ```tsx
 * function MyComponent() {
 *   const rootResource = useRootResource()
 *
 *   if (rootResource.loading) return <div>Loading root...</div>
 *   if (rootResource.error) return <div>Error: {rootResource.error.message}</div>
 *   if (!rootResource.value) return null
 *
 *   // Use root to create sessions, etc.
 *   return <div>Root loaded!</div>
 * }
 * ```
 */
export function useRootResource(): Resource<Root> {
  return RootContext.useContext()
}
