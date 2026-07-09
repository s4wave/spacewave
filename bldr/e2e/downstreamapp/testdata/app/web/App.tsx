import React, { useEffect } from 'react'
import { Toaster, toast } from 'sonner'
import * as NonIndexRootPkg from 'non-index-root-pkg'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SpacePluginLifecycleState } from '@s4wave/sdk/space/space.pb.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import {
  createViewerCatalog,
  getBaseObjectViewers,
  pluginLifecycleLabel,
} from '@s4wave/web/sdk/app/index.js'
import { ObjectViewerContent } from '@s4wave/web/object/ObjectViewerContent.js'
import type { ObjectViewerComponent } from '@s4wave/web/object/object.js'
import type { ObjectInfo } from '@s4wave/web/object/object.pb.js'
import '@s4wave/web/style/app.css'

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
    }
  }
}

const toastText = 'Downstream Sonner loaded through Bldr'
const downstreamViewer: ObjectViewerComponent = {
  componentID: 'mercury.note.viewer',
  typeID: 'mercury/note',
  name: 'Mercury Note',
  component: () => <div>Mercury Note Viewer</div>,
}
const catalog = createViewerCatalog({
  base: getBaseObjectViewers(),
  downstream: [downstreamViewer],
})
const catalogComponentIDs = catalog.map((viewer) => viewer.componentID)
const lifecycleLabels = [
  SpacePluginLifecycleState.SpacePluginLifecycleState_CONFIGURED,
  SpacePluginLifecycleState.SpacePluginLifecycleState_LOADING,
  SpacePluginLifecycleState.SpacePluginLifecycleState_LOADED,
  SpacePluginLifecycleState.SpacePluginLifecycleState_FAILED,
  SpacePluginLifecycleState.SpacePluginLifecycleState_RETRYING,
  SpacePluginLifecycleState.SpacePluginLifecycleState_REMOVED,
  SpacePluginLifecycleState.SpacePluginLifecycleState_UPGRADED,
].map((state) => pluginLifecycleLabel({ state }))
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
} satisfies Resource<IWorldState>

function publishProbe(ready: boolean) {
  window.__downstreamE2E = {
    ready,
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
  }
}

export default function DownstreamApp() {
  useEffect(() => {
    publishProbe(false)
    toast(toastText)
    publishProbe(true)
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
