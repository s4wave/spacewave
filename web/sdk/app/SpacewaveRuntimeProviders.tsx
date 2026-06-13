import { useBldrContext } from '@aptre/bldr-react'
import { ResourcesProvider } from '@aptre/bldr-sdk/hooks/ResourcesContext.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import {
  ResourceServiceClient,
  ResourceServiceServiceName,
} from '@aptre/bldr-sdk/resource/resource_srpc.pb.js'
import { Client as ResourceClient } from '@aptre/bldr-sdk/resource/index.js'
import { Client as SRPCClient } from 'starpc'
import { useEffect, useMemo, useState, type ReactNode } from 'react'

import type { Root } from '@s4wave/sdk/root'
import type { StaticConfigTypeRegistration } from '@s4wave/web/configtype/configtype.js'
import { ConfigTypeRegistryProvider } from '@s4wave/web/configtype/ConfigTypeRegistryContext.js'
import { CommandProvider } from '@s4wave/web/command/index.js'
import { RootContext } from '@s4wave/web/contexts/contexts.js'
import {
  ResourceDevToolsProvider,
  StateDevToolsProvider,
} from '@s4wave/web/devtools/index.js'
import { FloatingWindowManagerProvider } from '@s4wave/web/ui/FloatingWindow.js'
import { useRootResourceWithClient } from '@s4wave/web/hooks/useRootResource.js'
import { ViewerRegistryProvider } from '@s4wave/web/hooks/useViewerRegistry.js'
import type { ObjectViewerComponent } from '@s4wave/web/object/object.js'
import {
  StateNamespaceProvider,
  type StateAtomAccessor,
} from '@s4wave/web/state/index.js'
import { ErrorState } from '@s4wave/web/ui/ErrorState.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

const defaultResourceService =
  'plugin/spacewave-core/' + ResourceServiceServiceName

export interface SpacewaveRuntimeContext {
  resourceClient: ResourceClient
  rootResource: Resource<Root>
}

export interface SpacewaveRuntimeProvidersProps {
  staticViewers: ObjectViewerComponent[]
  staticConfigTypes: StaticConfigTypeRegistration[]
  resourceService?: string
  children: ReactNode | ((ctx: SpacewaveRuntimeContext) => ReactNode)
}

export function SpacewaveRuntimeProviders({
  staticViewers,
  staticConfigTypes,
  resourceService = defaultResourceService,
  children,
}: SpacewaveRuntimeProvidersProps) {
  const resourceClient = useSpacewaveResourceClient(resourceService)

  return (
    <ViewerRegistryProvider staticViewers={staticViewers}>
      <ConfigTypeRegistryProvider staticConfigTypes={staticConfigTypes}>
        <FloatingWindowManagerProvider>
          <StateDevToolsProvider>
            <ResourceDevToolsProvider>
              <ResourcesProvider client={resourceClient}>
                <SpacewaveRuntimeRoot
                  resourceClient={resourceClient}
                  children={children}
                />
              </ResourcesProvider>
            </ResourceDevToolsProvider>
          </StateDevToolsProvider>
        </FloatingWindowManagerProvider>
      </ConfigTypeRegistryProvider>
    </ViewerRegistryProvider>
  )
}

function useSpacewaveResourceClient(
  resourceServiceName: string,
): ResourceClient | null {
  const bldrContext = useBldrContext()
  const webViewUuid = bldrContext?.webView?.getUuid() || null
  const webDocument = bldrContext?.webDocument
  const [resourceClient, setResourceClient] = useState<ResourceClient | null>(
    null,
  )

  useEffect(() => {
    if (!webViewUuid || !webDocument) return

    const abortController = new AbortController()
    const rpcClient = new SRPCClient(
      webDocument.buildWebViewHostOpenStream(webViewUuid),
    )
    const service = new ResourceServiceClient(rpcClient, {
      service: resourceServiceName,
    })
    const client = new ResourceClient(service, abortController.signal)
    setResourceClient(client)

    return () => {
      client.dispose()
      setResourceClient(null)
      abortController.abort()
    }
  }, [webViewUuid, webDocument, resourceServiceName])

  return resourceClient
}

function SpacewaveRuntimeRoot({
  resourceClient,
  children,
}: {
  resourceClient: ResourceClient | null
  children: ReactNode | ((ctx: SpacewaveRuntimeContext) => ReactNode)
}) {
  const rootResource = useRootResourceWithClient(resourceClient)
  const rootStateAccessor: StateAtomAccessor = useMemo(() => {
    const root = rootResource.value
    if (!root) {
      return {
        value: null,
        loading: true,
        error: null,
        retry: () => rootResource.retry(),
      }
    }
    return {
      value: (storeId: string, signal?: AbortSignal) =>
        root.accessStateAtom({ storeId }, signal),
      loading: false,
      error: null,
      retry: () => {},
    }
  }, [rootResource])

  if (rootResource.error) {
    return (
      <ErrorState
        variant="fullscreen"
        title="Failed to load"
        message={rootResource.error.message}
        onRetry={rootResource.retry}
      />
    )
  }

  if (!resourceClient || rootResource.loading || !rootResource.value) {
    return (
      <div className="bg-background/80 flex h-full min-h-0 w-full flex-1 items-center justify-center p-6 backdrop-blur-sm">
        <div className="w-full max-w-sm">
          <LoadingCard
            view={{
              state: 'loading',
              title: 'Initializing',
              detail: 'Preparing the Spacewave runtime.',
            }}
          />
        </div>
      </div>
    )
  }

  const renderedChildren =
    typeof children === 'function'
      ? children({ resourceClient, rootResource })
      : children

  return (
    <RootContext.Provider resource={rootResource}>
      <StateNamespaceProvider stateAtomAccessor={rootStateAccessor}>
        <CommandProvider rootResource={rootResource}>
          {renderedChildren}
        </CommandProvider>
      </StateNamespaceProvider>
    </RootContext.Provider>
  )
}
