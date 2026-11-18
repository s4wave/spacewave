import { Client as RPCClient, createHandler, createMux, Server } from 'starpc'
import type { BackendAPI, BackendEntrypointFunc } from '@aptre/bldr-sdk'
import {
  Client,
  type ClientResourceRef,
} from '@aptre/bldr-sdk/resource/client.js'
import { ResourceServiceClient } from '@aptre/bldr-sdk/resource/resource_srpc.pb.js'

import { frontendBindingPath } from '../../bldr/frontend/binding.js'
import type { Binding } from '../../bldr/frontend/frontend.pb.js'
import { ActivationDefinition } from '../../bldr/plugin/plugin_srpc.pb.js'
import { buildFSHandle } from '../../db/unixfs/rpc/client/fs-handle.js'
import { FSCursorServiceClient } from '../../db/unixfs/rpc/rpc_srpc.pb.js'
import { Hash } from '../../net/hash/hash.pb.js'
import { base58Encode } from '../../net/peer/base58.js'
import { ViewerSurface } from '../viewer/registry/registry.pb.js'
import { ViewerRegistryResourceServiceClient } from '../viewer/registry/registry_srpc.pb.js'
import {
  activateRegistrations,
  createActivationHandler,
  prepareRegistrations,
} from './registration/generation.js'

/** PluginViewer declares one web viewer a plugin serves. */
export interface PluginViewer {
  /** typeId is the object type it opens; a trailing /* matches a prefix. */
  readonly typeId: string
  /** viewerName names the viewer in the selector. */
  readonly viewerName: string
  /** componentId is the stable ID of a persisted viewer selection. */
  readonly componentId: string
  /** entry is the viewer module's source path in the plugin's build. */
  readonly entry: string
  /** category groups the viewer in the selector. */
  readonly category?: string
  /** defaultForType opens this viewer for its type when none is chosen. */
  readonly defaultForType?: boolean
}

/**
 * defineViewerPlugin is the backend entrypoint of a plugin that only serves
 * viewers. It registers every viewer in one registration generation, which
 * the host shows together and hides when the worker stops. A historical
 * worker registers nothing.
 */
export function defineViewerPlugin(
  viewers: readonly PluginViewer[],
): BackendEntrypointFunc {
  return (api, signal) => {
    const controller = new AbortController()
    const lifetime = AbortSignal.any([signal, controller.signal])
    const refs: ClientResourceRef[] = []
    let resources: Client | undefined
    let generation: ClientResourceRef | undefined

    // Serve the scheduler's activation of a staged replacement.
    const pluginMux = createMux()
    pluginMux.register(
      createHandler(
        ActivationDefinition,
        createActivationHandler(async () => {
          await startup
          return generation
        }, lifetime),
      ),
    )
    const pluginServer = new Server(pluginMux.lookupMethod)
    api.handleStreamCtr.set(async (channel) => {
      pluginServer.handlePacketStream(channel)
    })

    // Register the viewers in this worker's generation once the core is reachable.
    const startup = (async () => {
      const host = await api.pluginHost.GetPluginInfo({}, lifetime)
      if (host.historical) return
      const hash = host.manifestRef?.manifestRef?.rootRef?.hash
      if (!host.pluginId || !hash) {
        throw new Error('Plugin has no immutable manifest')
      }
      const pluginId = host.pluginId
      const manifestRoot = base58Encode(Hash.toBinary(hash))
      const rpc = new RPCClient(api.buildPluginOpenStream('spacewave-core'))
      resources = new Client(new ResourceServiceClient(rpc), lifetime)
      const core = await resources.accessRootResource()
      refs.push(core)
      generation = await prepareRegistrations(
        core,
        { pluginId, manifestRoot, instanceKey: host.instanceKey },
        lifetime,
      )
      const scope = generation

      // Each registration lives as long as its retained Resource.
      const registry = new ViewerRegistryResourceServiceClient(scope.client)
      await Promise.all(
        viewers.map(async (viewer) => {
          const { resourceId } = await registry.RegisterViewer(
            {
              registration: {
                typeId: viewer.typeId,
                viewerName: viewer.viewerName,
                componentId: viewer.componentId,
                category: viewer.category,
                defaultForType: viewer.defaultForType,
                scriptPath: await resolveViewerPath(
                  api,
                  viewer.entry,
                  manifestRoot,
                  lifetime,
                ),
                surface: ViewerSurface.WEB,
              },
            },
            lifetime,
          )
          if (!resourceId) {
            throw new Error('Viewer registration returned no Resource')
          }
          refs.push(scope.createRef(resourceId))
        }),
      )

      // An initial load publishes now; a replacement waits for the scheduler.
      if (!host.prepared) await activateRegistrations(scope, lifetime)
    })()

    // Hold the registrations until the worker stops.
    const done = startup
      .then(() => waitForAbort(lifetime))
      .finally(() => {
        generation?.release()
        for (const ref of refs.reverse()) ref.release()
        controller.abort()
        resources?.dispose()
      })
    return { startup, done }
  }
}

/**
 * resolveViewerPath returns the script path of a viewer module: its frontend
 * service binding when the build serves one, or the immutable asset of the
 * manifest at manifestRoot.
 */
export async function resolveViewerPath(
  api: BackendAPI,
  entry: string,
  manifestRoot: string,
  signal: AbortSignal,
): Promise<string> {
  // Read the plugin's Vite manifest from its assets.
  const service = new FSCursorServiceClient(api.client, {
    service: 'plugin-assets/unixfs.rpc.FSCursorService',
  })
  using root = await buildFSHandle(service, signal)
  const result = await root.lookupPath(signal, 'v/b/fe/.vite/manifest.json')
  using manifest = result.handle
  const size = await manifest.getSize(signal)
  const { data } = await manifest.readAt(signal, 0n, size)
  const entries = JSON.parse(new TextDecoder().decode(data)) as Record<
    string,
    { frontendBinding?: Binding; file?: string }
  >

  // Prefer the frontend binding, then the built asset.
  const asset = entries[entry.replace(/^\.\//, '')]
  if (asset?.frontendBinding) return frontendBindingPath(asset.frontendBinding)
  if (!asset?.file) {
    throw new Error(`Viewer ${entry} is absent from the build manifest`)
  }
  return api.utils.pluginAssetHttpPath(
    `${api.startInfo.pluginId!}/manifest/${manifestRoot}`,
    `v/b/fe/${asset.file}`,
  )
}

/** waitForAbort resolves when signal aborts. */
export function waitForAbort(signal: AbortSignal): Promise<void> {
  if (signal.aborted) return Promise.resolve()
  return new Promise((resolve) =>
    signal.addEventListener('abort', () => resolve(), { once: true }),
  )
}
