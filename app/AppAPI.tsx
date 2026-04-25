import { useBldrContext } from '@aptre/bldr-react'
import { Client as SRPCClient } from 'starpc'
import { useEffect, useMemo, useState } from 'react'

import { setDebugContext } from '@s4wave/sdk/debug/context.js'
import {
  createLocalSession,
  createDrive,
  createQuickstartSetup,
} from '@s4wave/app/quickstart/create.js'
import { mountSpace } from '@s4wave/app/space/space.js'
import { runSOPerfTest } from '@s4wave/app/quickstart/perf-test.js'
import { SpacewaveProvider } from '@s4wave/sdk/provider/spacewave/spacewave.js'
import { FSHandle } from '@s4wave/sdk/unixfs/handle.js'
import { MknodType } from '@s4wave/sdk/unixfs/index.js'
import { UNIXFS_OBJECT_KEY } from '@s4wave/core/space/world/ops/init-unixfs.js'

import {
  ResourceServiceClient,
  ResourceServiceServiceName,
} from '@aptre/bldr-sdk/resource/resource_srpc.pb.js'
import { Client as ResourceClient } from '@aptre/bldr-sdk/resource/index.js'
import { ResourcesProvider } from '@aptre/bldr-sdk/hooks/ResourcesContext.js'
import { RootContext } from '@s4wave/web/contexts/contexts.js'
import { useRootResourceWithClient } from '@s4wave/web/hooks/useRootResource.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { ErrorState } from '@s4wave/web/ui/ErrorState.js'
import { FloatingWindowManagerProvider } from '@s4wave/web/ui/FloatingWindow.js'
import {
  ResourceDevToolsProvider,
  StateDevToolsProvider,
} from '@s4wave/web/devtools/index.js'
import { CommandProvider } from '@s4wave/web/command/index.js'
import { UpdateNotifier } from '@s4wave/web/launcher/UpdateNotifier.js'
import { ListenerYieldNotifier } from '@s4wave/app/listener/ListenerYieldNotifier.js'
import { ViewerRegistryProvider } from '@s4wave/web/hooks/useViewerRegistry.js'
import { ConfigTypeRegistryProvider } from '@s4wave/web/configtype/ConfigTypeRegistryContext.js'
import { getAllObjectViewers } from '@s4wave/app/viewers.js'
import { staticConfigTypes } from '@s4wave/app/configtypes.js'
import {
  StateNamespaceProvider,
  type StateAtomAccessor,
} from '@s4wave/web/state/index.js'

const staticViewers = getAllObjectViewers()

export interface AppAPIProps {
  children: React.ReactNode
}

export function AppAPI({ children }: AppAPIProps) {
  const bldrContext = useBldrContext()
  const webView = bldrContext?.webView
  const webDocument = bldrContext?.webDocument
  const webViewUuid = webView?.getUuid() || null

  const [resourceClient, setResourceClient] = useState<ResourceClient | null>(
    null,
  )
  useEffect(() => {
    if (!webViewUuid || !webDocument) return

    const abortController = new AbortController()
    const rpcClient = new SRPCClient(
      webDocument.buildWebViewHostOpenStream(webViewUuid),
    )
    const resourceService = new ResourceServiceClient(rpcClient, {
      service: 'plugin/spacewave-core/' + ResourceServiceServiceName,
    })
    const resourceClient = new ResourceClient(
      resourceService,
      abortController.signal,
    )
    // eslint-disable-next-line react-hooks/set-state-in-effect -- initialization requires synchronous state update
    setResourceClient(resourceClient)

    return () => {
      resourceClient.dispose()
      setResourceClient(null)
      abortController.abort()
    }
  }, [webViewUuid, webDocument])

  // Wrap everything in providers first, then create root resource inside
  // This ensures ResourceDevToolsProvider is available when useRootResourceWithClient runs
  return (
    <ViewerRegistryProvider staticViewers={staticViewers}>
      <ConfigTypeRegistryProvider staticConfigTypes={staticConfigTypes}>
        <FloatingWindowManagerProvider>
          <StateDevToolsProvider>
            <ResourceDevToolsProvider>
              <ResourcesProvider client={resourceClient}>
                <AppAPIInner resourceClient={resourceClient}>
                  {children}
                </AppAPIInner>
              </ResourcesProvider>
            </ResourceDevToolsProvider>
          </StateDevToolsProvider>
        </FloatingWindowManagerProvider>
      </ConfigTypeRegistryProvider>
    </ViewerRegistryProvider>
  )
}

// AppAPIInner creates the root resource inside the DevTools context
function AppAPIInner({
  resourceClient,
  children,
}: {
  resourceClient: ResourceClient | null
  children: React.ReactNode
}) {
  const rootResource = useRootResourceWithClient(resourceClient)

  // Expose resources to debug eval scripts via globalThis.
  useEffect(() => {
    if (resourceClient && rootResource.value) {
      setDebugContext({
        client: resourceClient,
        root: rootResource.value,
        createLocalSession,
        createDrive,
        createQuickstartSetup,
        mountSpace,
        FSHandle,
        MknodType,
        SpacewaveProvider,
        UNIXFS_OBJECT_KEY,
        runSOPerfTest,
      })
    }
  }, [resourceClient, rootResource.value])

  // Must be before early returns to satisfy React hooks rules.
  const rootStateAccessor: StateAtomAccessor = useMemo(() => {
    const root = rootResource.value
    if (!root)
      return {
        value: null,
        loading: true,
        error: null,
        retry: () => rootResource.retry(),
      }
    return {
      value: (storeId: string, signal?: AbortSignal) =>
        root.accessStateAtom({ storeId }, signal),
      loading: false,
      error: null,
      retry: () => {},
    }
  }, [rootResource])

  if (rootResource.loading || !rootResource.value) {
    return (
      <div className="bg-background/80 flex min-h-screen w-full items-center justify-center p-6 backdrop-blur-sm">
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

  return (
    <RootContext.Provider resource={rootResource}>
      <StateNamespaceProvider stateAtomAccessor={rootStateAccessor}>
        <CommandProvider rootResource={rootResource}>
          <UpdateNotifier rootResource={rootResource} />
          <ListenerYieldNotifier rootResource={rootResource}>
            {children}
          </ListenerYieldNotifier>
        </CommandProvider>
      </StateNamespaceProvider>
    </RootContext.Provider>
  )
}
