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
  atom,
  StateNamespaceProvider,
  type Atom,
  type StateAtomAccessor,
  type StateType,
} from '@s4wave/web/state/index.js'
import { ErrorState } from '@s4wave/web/ui/ErrorState.js'
import { LoadingScreen } from '@s4wave/web/ui/loading/LoadingScreen.js'

const defaultResourceService =
  'plugin/spacewave-core/' + ResourceServiceServiceName

export interface SpacewaveRuntimeContext {
  resourceClient: ResourceClient
  rootResource: Resource<Root>
}

export interface SpacewaveRuntimeProvidersProps {
  staticViewers: ObjectViewerComponent[]
  staticConfigTypes: StaticConfigTypeRegistration[]
  /**
   * ResourceClient for an explicit runtime connection. Present (including
   * null while pending) bypasses the Bldr WebView transport entirely; the
   * caller owns the client lifetime. Omitted keeps the default Bldr context
   * transport.
   */
  resourceClient?: ResourceClient | null
  /**
   * rootAtom scopes legacy local UI state. Explicit connections default to
   * a fresh atom so nested state never reaches the outer app's global atom;
   * the default connection omits it and inherits from the outer tree.
   */
  rootAtom?: Atom<StateType>
  /** resourceService is the service name for the default Bldr transport. */
  resourceService?: string
  children: ReactNode | ((ctx: SpacewaveRuntimeContext) => ReactNode)
}

export function SpacewaveRuntimeProviders(
  props: SpacewaveRuntimeProvidersProps,
) {
  // Presence of resourceClient selects the explicit runtime connection; see
  // SpacewaveRuntimeProvidersProps.resourceClient.
  if ('resourceClient' in props) {
    return <ExplicitSpacewaveRuntime {...props} />
  }
  return <BldrSpacewaveRuntime {...props} />
}

/**
 * ExplicitSpacewaveRuntime renders the provider tree for a caller-owned
 * ResourceClient, defaulting the root UI atom to a fresh in-memory atom.
 */
function ExplicitSpacewaveRuntime({
  staticViewers,
  staticConfigTypes,
  resourceClient,
  rootAtom,
  children,
}: SpacewaveRuntimeProvidersProps) {
  const scopedRootAtom = useMemo(
    () => rootAtom ?? atom<StateType>({}),
    [rootAtom],
  )
  return (
    <RuntimeProviderTree
      resourceClient={resourceClient ?? null}
      staticViewers={staticViewers}
      staticConfigTypes={staticConfigTypes}
      rootAtom={scopedRootAtom}
    >
      {children}
    </RuntimeProviderTree>
  )
}

/**
 * BldrSpacewaveRuntime renders the provider tree over the Bldr WebView
 * transport, creating and disposing the ResourceClient itself.
 */
function BldrSpacewaveRuntime({
  staticViewers,
  staticConfigTypes,
  resourceService = defaultResourceService,
  rootAtom,
  children,
}: SpacewaveRuntimeProvidersProps) {
  const resourceClient = useSpacewaveResourceClient(resourceService)
  return (
    <RuntimeProviderTree
      resourceClient={resourceClient}
      staticViewers={staticViewers}
      staticConfigTypes={staticConfigTypes}
      rootAtom={rootAtom}
    >
      {children}
    </RuntimeProviderTree>
  )
}

/**
 * RuntimeProviderTree is the shared provider composition for both runtime
 * connection modes.
 */
function RuntimeProviderTree({
  resourceClient,
  staticViewers,
  staticConfigTypes,
  rootAtom,
  children,
}: {
  resourceClient: ResourceClient | null
  staticViewers: ObjectViewerComponent[]
  staticConfigTypes: StaticConfigTypeRegistration[]
  rootAtom?: Atom<StateType>
  children: ReactNode | ((ctx: SpacewaveRuntimeContext) => ReactNode)
}) {
  return (
    <ViewerRegistryProvider staticViewers={staticViewers}>
      <ConfigTypeRegistryProvider staticConfigTypes={staticConfigTypes}>
        <FloatingWindowManagerProvider>
          <StateDevToolsProvider>
            <ResourceDevToolsProvider>
              <ResourcesProvider client={resourceClient}>
                <SpacewaveRuntimeRoot
                  resourceClient={resourceClient}
                  rootAtom={rootAtom}
                >
                  {children}
                </SpacewaveRuntimeRoot>
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
  rootAtom,
  children,
}: {
  resourceClient: ResourceClient | null
  rootAtom?: Atom<StateType>
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
      <LoadingScreen
        view={{
          state: 'loading',
          title: 'Starting Spacewave',
          detail: 'Preparing the Spacewave runtime.',
        }}
      />
    )
  }

  const renderedChildren =
    typeof children === 'function'
      ? children({ resourceClient, rootResource })
      : children

  return (
    <RootContext.Provider resource={rootResource}>
      <StateNamespaceProvider
        rootAtom={rootAtom}
        stateAtomAccessor={rootStateAccessor}
      >
        <CommandProvider rootResource={rootResource}>
          {renderedChildren}
        </CommandProvider>
      </StateNamespaceProvider>
    </RootContext.Provider>
  )
}
