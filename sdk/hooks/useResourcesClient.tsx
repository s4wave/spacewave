// useResourcesClient accesses the ResourceClient from context.
import { Client as ResourceClient } from '../resource/client.js'
import { useResourcesContext } from './ResourcesContext.js'

// useResourcesClient returns the ResourceClient instance from the ResourcesProvider context.
export function useResourcesClient(): ResourceClient | null {
  return useResourcesContext()?.client ?? null
}
