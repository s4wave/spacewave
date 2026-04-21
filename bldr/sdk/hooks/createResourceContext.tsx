import React, {
  createContext,
  useContext,
  useMemo,
  type ReactNode,
} from 'react'
import type { Resource as SDKResource } from '../resource/resource.js'
import type { Resource } from './useResource.js'

export interface ResourceContextType<T extends SDKResource> {
  Provider: React.FC<{ resource: Resource<T>; children: ReactNode }>
  useContext: () => Resource<T>
}

export function createResourceContext<
  T extends SDKResource,
>(): ResourceContextType<T> {
  const Context = createContext<Resource<T> | null>(null)

  const Provider: React.FC<{
    resource: Resource<T>
    children: ReactNode
  }> = ({ resource, children }) => {
    return <Context.Provider value={resource}>{children}</Context.Provider>
  }

  const useResourceContext = (): Resource<T> => {
    const resource = useContext(Context)

    return useMemo(() => {
      if (!resource) {
        return {
          value: null,
          loading: false,
          error: new Error(
            'Resource context not found. Wrap component in the appropriate Provider.',
          ),
          retry: () => {},
          // Include a sentinel devtools ID so child resources don't appear as orphans
          __devtools: { id: '_context_not_found_' },
        }
      }

      return resource
    }, [resource])
  }

  return {
    Provider,
    useContext: useResourceContext,
  }
}
