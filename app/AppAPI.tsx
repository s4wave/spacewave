import { useEffect } from 'react'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { Client as ResourceClient } from '@aptre/bldr-sdk/resource/index.js'

import type { Root } from '@s4wave/sdk/root/index.js'
import {
  clearDebugContext,
  setDebugContext,
} from '@s4wave/sdk/debug/context.js'
import { SpacewaveProvider } from '@s4wave/sdk/provider/spacewave/spacewave.js'
import { FSHandle } from '@s4wave/sdk/unixfs/handle.js'
import { MknodType } from '@s4wave/sdk/unixfs/index.js'
import { UNIXFS_OBJECT_KEY } from '@s4wave/core/space/world/ops/init-unixfs.js'
import { downloadURL } from '@s4wave/web/download.js'
import { SpacewaveRuntimeProviders } from '@s4wave/web/sdk/app/index.js'

import { staticConfigTypes } from './configtypes.js'
import { UpdateNotifier } from '../web/launcher/UpdateNotifier.js'
import { ListenerYieldNotifier } from './listener/ListenerYieldNotifier.js'
import {
  createLocalSession,
  createDrive,
  createQuickstartSetup,
} from './quickstart/create.js'
import { runPostLoadSOPerfTest, runSOPerfTest } from './quickstart/perf-test.js'
import { QuickstartOptionsProvider } from './quickstart/useQuickstartOptions.js'
import { mountSpace } from './space/space.js'
import { getAllObjectViewers } from './viewers.js'

const staticViewers = getAllObjectViewers()

export interface AppAPIProps {
  children: React.ReactNode
}

export function AppAPI({ children }: AppAPIProps) {
  return (
    <SpacewaveRuntimeProviders
      staticViewers={staticViewers}
      staticConfigTypes={staticConfigTypes}
    >
      {({ resourceClient, rootResource }) => (
        <SpacewaveProductRuntime
          resourceClient={resourceClient}
          rootResource={rootResource}
        >
          {children}
        </SpacewaveProductRuntime>
      )}
    </SpacewaveRuntimeProviders>
  )
}

function SpacewaveProductRuntime({
  rootResource,
  resourceClient,
  children,
}: {
  rootResource: Resource<Root>
  resourceClient: ResourceClient
  children: React.ReactNode
}) {
  useEffect(() => {
    const root = rootResource.value
    if (!root) return

    const debugContext = {
      client: resourceClient,
      root,
      createLocalSession,
      createDrive,
      createQuickstartSetup,
      mountSpace,
      FSHandle,
      downloadURL,
      MknodType,
      SpacewaveProvider,
      UNIXFS_OBJECT_KEY,
      runSOPerfTest,
      runPostLoadSOPerfTest,
    }
    setDebugContext(debugContext)
    return () => {
      clearDebugContext(debugContext)
    }
  }, [resourceClient, rootResource.value])

  return (
    <QuickstartOptionsProvider rootResource={rootResource}>
      <UpdateNotifier rootResource={rootResource} />
      <ListenerYieldNotifier rootResource={rootResource}>
        {children}
      </ListenerYieldNotifier>
    </QuickstartOptionsProvider>
  )
}
