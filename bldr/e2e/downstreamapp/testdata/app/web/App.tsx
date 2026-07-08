import { useEffect, useMemo, useState } from 'react'
import { useWebViewHostClient } from '@aptre/bldr-react'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import {
  Client as ResourceClient,
  ResourceServiceClient,
  type ClientResourceRef,
} from '@aptre/bldr-sdk/resource/index.js'
import * as NonIndexRootPkg from 'non-index-root-pkg'
import type { Client as SRPCClient } from 'starpc'
import { toast as sonnerToast } from 'sonner'

import type { ViewerRegistration } from '@s4wave/sdk/viewer/registry/registry.pb.js'
import { ViewerRegistryResourceServiceClient } from '@s4wave/sdk/viewer/registry/registry_srpc.pb.js'
import { ObjectViewerContent } from '@s4wave/web/object/ObjectViewerContent.js'
import type { ObjectViewerComponent } from '@s4wave/web/object/object.js'
import type { ObjectInfo } from '@s4wave/web/object/object.pb.js'
import { pluginLifecycleLabel } from '@s4wave/web/sdk/app/lifecycle.js'
import { createViewerCatalog } from '@s4wave/web/sdk/app/viewer-catalog.js'
import { Toaster } from '@s4wave/web/ui/toaster.js'

interface ResourceProof {
  generatedResourceService: boolean
  rootResource: boolean
  registeredViewer: boolean
  releasedViewerResource: boolean
  releaseRemovedViewer: boolean
  releasedRootResource: boolean
  failureReason: string
}

declare global {
  interface Window {
    __downstreamE2E?: {
      ready: boolean
      toastText?: string
      resources?: string[]
      sdkAppImport?: boolean
      catalogComponentIDs?: string[]
      productViewerImplicit?: boolean
      nonIndexRootMarker?: string
      lifecycleLabels?: string[]
      fallbackRendered?: boolean
      resourceProof?: ResourceProof
    }
  }
}

const toastText = 'Downstream Sonner loaded through Bldr'
const resourceReadyText = 'ResourceService proof ready'
const registeredViewerComponentID = 'mercury.dynamic.viewer'
const registeredViewerRegistration: ViewerRegistration = {
  typeId: 'mercury/dynamic',
  viewerName: 'Mercury Dynamic Viewer',
  scriptPath: '/b/pa/downstream-web/v/App.mjs',
  category: 'Downstream',
  componentId: registeredViewerComponentID,
}
const pendingResourceProof: ResourceProof = {
  generatedResourceService: false,
  rootResource: false,
  registeredViewer: false,
  releasedViewerResource: false,
  releaseRemovedViewer: false,
  releasedRootResource: false,
  failureReason: '',
}
const downstreamViewer: ObjectViewerComponent = {
  componentID: 'mercury.note.viewer',
  typeID: 'mercury/note',
  name: 'Mercury Note',
  component: () => <div>Mercury Note Viewer</div>,
}
const baseViewers: ObjectViewerComponent[] = [
  {
    componentID: 'spacewave.object-layout.viewer',
    typeID: '*',
    name: 'Object Layout',
    component: () => <div>Object Layout Viewer</div>,
  },
  {
    componentID: 'spacewave.debug.viewer',
    typeID: '*',
    name: 'Debug Viewer',
    component: () => <div>Debug Viewer</div>,
  },
]
const catalog = createViewerCatalog({
  base: baseViewers,
  downstream: [downstreamViewer],
})
const catalogComponentIDs = catalog.map((viewer) => viewer.componentID)
const lifecycleLabels = [1, 2, 3, 4, 5, 6, 7].map((state) =>
  pluginLifecycleLabel({ state }),
)
const objectInfo: ObjectInfo = {
  info: {
    case: 'worldObjectInfo',
    value: {
      engineId: 'fixture-engine',
      objectKey: 'mercury/fixture-note',
      objectType: 'mercury/missing',
    },
  },
}
const worldState = {
  value: null,
  loading: true,
  error: null,
  retry: () => {},
} satisfies Resource<unknown>

function resourceProofPassed(proof: ResourceProof): boolean {
  return (
    proof.generatedResourceService &&
    proof.rootResource &&
    proof.registeredViewer &&
    proof.releasedViewerResource &&
    proof.releaseRemovedViewer &&
    proof.releasedRootResource &&
    proof.failureReason === ''
  )
}

function hasRegisteredViewer(
  registrations: ViewerRegistration[] = [],
): boolean {
  return registrations.some(
    (registration) =>
      registration.componentId === registeredViewerComponentID &&
      registration.typeId === registeredViewerRegistration.typeId,
  )
}

async function waitForViewerRemoval(
  service: ViewerRegistryResourceServiceClient,
  signal: AbortSignal,
): Promise<boolean> {
  const controller = new AbortController()
  const timeout = window.setTimeout(() => {
    controller.abort()
  }, 5000)
  const abort = () => {
    controller.abort()
  }
  signal.addEventListener('abort', abort, { once: true })
  try {
    for await (const snapshot of service.WatchViewers({}, controller.signal)) {
      if (!hasRegisteredViewer(snapshot.registrations ?? [])) {
        return true
      }
    }
    return false
  } catch (err) {
    if (controller.signal.aborted) {
      return false
    }
    throw err
  } finally {
    window.clearTimeout(timeout)
    signal.removeEventListener('abort', abort)
  }
}

function publishProbe(resourceProof: ResourceProof) {
  window.__downstreamE2E = {
    ready: resourceProofPassed(resourceProof),
    toastText,
    resources: performance
      .getEntriesByType('resource')
      .map((entry) => entry.name),
    sdkAppImport: true,
    catalogComponentIDs,
    productViewerImplicit: catalogComponentIDs.includes(
      'spacewave.unixfs.viewer',
    ),
    nonIndexRootMarker: NonIndexRootPkg.marker,
    lifecycleLabels,
    fallbackRendered: !!document.body?.innerText.includes(
      "Can't open this object yet",
    ),
    resourceProof,
  }
}

async function proveResourceService(
  client: SRPCClient,
  signal: AbortSignal,
): Promise<ResourceProof> {
  const service = new ResourceServiceClient(client)
  const resourceClient = new ResourceClient(service, signal)
  let rootRef: ClientResourceRef | null = null
  let registrationRef: ClientResourceRef | null = null
  let registeredViewer = false
  let releasedViewerResource = false
  let releaseRemovedViewer = false
  let releasedRootResource = false
  try {
    rootRef = await resourceClient.accessRootResource()
    const viewerRegistry = new ViewerRegistryResourceServiceClient(
      rootRef.client,
    )
    const registration = await viewerRegistry.RegisterViewer(
      { registration: registeredViewerRegistration },
      signal,
    )
    const registrationResourceID = registration.resourceId ?? 0
    if (!registrationResourceID) {
      throw new Error('viewer registration did not return a resource id')
    }
    const registered = await viewerRegistry.ListViewers({}, signal)
    registeredViewer = hasRegisteredViewer(registered.registrations ?? [])
    const removed = waitForViewerRemoval(viewerRegistry, signal)
    registrationRef = rootRef.createRef(registrationResourceID)
    registrationRef.release()
    registrationRef = null
    releasedViewerResource = true
    releaseRemovedViewer = await removed
    rootRef.release()
    rootRef = null
    releasedRootResource = true
    return {
      generatedResourceService: true,
      rootResource: true,
      registeredViewer,
      releasedViewerResource,
      releaseRemovedViewer,
      releasedRootResource,
      failureReason: '',
    }
  } catch (err) {
    return {
      generatedResourceService: true,
      rootResource: rootRef !== null,
      registeredViewer,
      releasedViewerResource,
      releaseRemovedViewer,
      releasedRootResource,
      failureReason: String(err),
    }
  } finally {
    registrationRef?.release()
    rootRef?.release()
    resourceClient.dispose()
  }
}

export default function DownstreamApp() {
  const [resourceProof, setResourceProof] =
    useState<ResourceProof>(pendingResourceProof)
  const resourceReady = useMemo(
    () => resourceProofPassed(resourceProof),
    [resourceProof],
  )

  useEffect(() => {
    sonnerToast(toastText)
  }, [])

  useEffect(() => {
    publishProbe(resourceProof)
  }, [resourceProof])

  useWebViewHostClient((client, abortSignal) => {
    let active = true
    proveResourceService(client, abortSignal).then((proof) => {
      if (!active) {
        return
      }
      setResourceProof(proof)
    })
    return () => {
      active = false
    }
  }, [])

  return (
    <main
      style={{
        fontFamily: 'system-ui, sans-serif',
        padding: 24,
      }}
    >
      <Toaster />
      <h1>Downstream GoScript E2E</h1>
      <p data-testid="downstream-ready">{toastText}</p>
      <p data-testid="resource-service-proof">
        {resourceReady
          ? resourceReadyText
          : `ResourceService route pending ${resourceProof.failureReason}`}
      </p>
      <section data-testid="sdk-app-proof">
        <h2>SDK app catalog proof</h2>
        <p>{catalogComponentIDs.join(', ')}</p>
        <p>{lifecycleLabels.join(', ')}</p>
        <div style={{ minHeight: 220 }}>
          <ObjectViewerContent
            objectInfo={objectInfo}
            worldState={worldState}
            typeID="mercury/missing"
            availableComponents={catalog}
            missingComponentID="mercury.missing.viewer"
            standalone
          />
        </div>
      </section>
    </main>
  )
}
