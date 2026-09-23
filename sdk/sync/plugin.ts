import {
  Client as RPCClient,
  createHandler,
  createMux,
  handleRpcStream,
  Server,
} from 'starpc'
import type {
  BackendAPI,
  BackendEntrypointFunc,
} from '../../bldr/sdk/plugin.js'
import {
  Client,
  type ClientResourceRef,
} from '../../bldr/sdk/resource/client.js'
import { ResourceServiceClient } from '../../bldr/sdk/resource/resource_srpc.pb.js'
import { getResourceCall } from '../../bldr/sdk/resource/server/context.js'
import { newResourceMux } from '../../bldr/sdk/resource/server/mux.js'
import { ResourceServer } from '../../bldr/sdk/resource/server/server.js'
import {
  ActivationDefinition,
  PluginDefinition,
} from '../../bldr/plugin/plugin_srpc.pb.js'
import {
  GenerationServiceClient,
  RegistrationServiceClient,
} from '../plugin/registration/registration_srpc.pb.js'
import { frontendBindingPath } from '../../bldr/frontend/binding.js'
import type { Binding } from '../../bldr/frontend/frontend.pb.js'
import { FSCursorServiceClient } from '../../db/unixfs/rpc/rpc_srpc.pb.js'
import { buildFSHandle } from '../../db/unixfs/rpc/client/fs-handle.js'
import {
  ObjectTypeHandlerServiceDefinition,
  ObjectTypeRegistryResourceServiceClient,
} from '../objecttype/registry/registry_srpc.pb.js'
import { WorldOpHandlerServiceDefinition } from '../worldop/registry/registry_srpc.pb.js'
import { ViewerRegistryResourceServiceClient } from '../viewer/registry/registry_srpc.pb.js'
import { ViewerSurface } from '../viewer/registry/registry.pb.js'
import { Engine } from '../world/engine.js'
import {
  QuickstartHandlerServiceDefinition,
  QuickstartRegistryResourceServiceClient,
} from '../quickstart/registry/registry_srpc.pb.js'

import type { AppDefinition, AppMutation } from './app.js'
import { createAppInstance } from './attachment.js'
import { appObjectTypeID } from './instance.js'
import type { Schema } from './schema.js'
import { createAppPluginOperations } from './plugin-operations.js'

/** AppPlugin declares the type and viewer served by one typed application module. */
export interface AppPlugin<S extends Schema> {
  readonly app: AppDefinition<S>
  readonly displayName: string
  readonly description?: string
  readonly iconName?: string
  readonly viewer: {
    readonly entry: string
    readonly componentId: string
  }
  readonly quickstart?: {
    readonly objectKey: string
    readonly initial?: AppMutation<S>
  }
}

/**
 * definePlugin adapts a typed app to Bldr's existing backend entrypoint.
 * Immutable historical workers serve operations without replacing current type
 * or viewer registrations. Every registration belongs to this worker lifetime.
 */
export function definePlugin<S extends Schema>(
  definition: AppPlugin<S>,
): BackendEntrypointFunc {
  return (api, signal) => {
    const controller = new AbortController()
    const lifetime = AbortSignal.any([signal, controller.signal])
    const refs: ClientResourceRef[] = []
    let resources: Client | undefined
    let generation: ClientResourceRef | undefined
    let detachHandlers: (() => void) | undefined
    const operations = createAppPluginOperations(api, definition.app, lifetime)
    const { info, revision } = operations

    // Publish handlers before registration can cause the host to resolve them.
    const resourceServer = new ResourceServer(
      newResourceMux(
        createHandler(QuickstartHandlerServiceDefinition, {
          SeedQuickstart: async (request, abort, context) => {
            if (
              !definition.quickstart ||
              request.quickstartId !== definition.app.schema.id
            ) {
              throw new Error('Unknown application Quickstart')
            }
            const executable = await revision
            using engine = new Engine(
              getResourceCall(context).getAttachedRef(
                request.attachedEngineResourceId ?? 0,
              ),
            )
            const app = await createAppInstance(
              definition.app,
              engine,
              executable,
              definition.quickstart.objectKey,
              {
                signal: AbortSignal.any([lifetime, abort]),
                requestId: `quickstart:${definition.app.schema.id}`,
                initial: definition.quickstart.initial,
              },
            )
            await app.close()
            return {
              indexPath: definition.quickstart.objectKey,
              pluginIds: [executable.pluginId],
            }
          },
        }),
        createHandler(WorldOpHandlerServiceDefinition, operations.handler),
        createHandler(ObjectTypeHandlerServiceDefinition, {
          // Collections remain World-backed capabilities. This type marks the app
          // object; its viewer attaches through the authorized World it receives.
          InvokeObjectType: async (request, _abort, context) => {
            if (request.typeId !== appObjectTypeID(definition.app.schema))
              throw new Error('Unknown application type')
            const { resourceId } = getResourceCall(
              context,
            ).constructChildResource(() => ({
              mux: newResourceMux(),
              result: undefined,
            }))
            return { resourceId }
          },
        }),
      ),
    )
    const resourceMux = createMux()
    resourceServer.register(resourceMux)
    const server = new Server(resourceMux.lookupMethod)
    const pluginMux = createMux()
    resourceServer.register(pluginMux)
    pluginMux.register(
      createHandler(PluginDefinition, {
        PluginRpc: (request) =>
          handleRpcStream(
            request[Symbol.asyncIterator](),
            async () => server.rpcStreamHandler,
          ),
      }),
    )
    pluginMux.register(
      createHandler(ActivationDefinition, {
        Check: async () => ({}),
        Activate: async (_request, abort) => {
          // Only a completely initialized candidate may publish its registrations.
          await startup
          if (!generation)
            throw new Error('Historical plugins cannot be activated')
          await new GenerationServiceClient(generation.client).Activate(
            {},
            AbortSignal.any([lifetime, abort]),
          )
          return {}
        },
      }),
    )
    const pluginServer = new Server(pluginMux.lookupMethod)
    api.handleStreamCtr.set(async (channel) => {
      pluginServer.handlePacketStream(channel)
    })

    // Registration depends on the core Resource tree, after RPC startup is ready.
    const startup = (async () => {
      const executable = await revision
      const host = await info
      if (!host.historical) {
        const rpc = new RPCClient(api.buildPluginOpenStream('spacewave-core'))
        resources = new Client(new ResourceServiceClient(rpc), lifetime)
        const core = await resources.accessRootResource()
        refs.push(core)
        const prepared = await new RegistrationServiceClient(
          core.client,
        ).Prepare({ ...executable, instanceKey: host.instanceKey }, lifetime)
        if (!prepared.resourceId)
          throw new Error('Plugin preparation returned no Resource')
        generation = core.createRef(prepared.resourceId)
        const root = generation

        // Bind ObjectType handlers to this worker instead of resolving the family again.
        const attached = await resources.attachResourceTree(
          'application-handlers',
          resourceMux.lookupMethod,
          lifetime,
        )
        detachHandlers = attached.cleanup
        const retain = (id?: number) => {
          if (!id) throw new Error('Plugin registration returned no Resource')
          refs.push(root.createRef(id))
        }
        const types = new ObjectTypeRegistryResourceServiceClient(root.client)
        retain(
          (
            await types.RegisterObjectType(
              {
                typeId: appObjectTypeID(definition.app.schema),
                pluginId: executable.pluginId,
                attachedHandlerResourceId: attached.resourceId,
                metadata: {
                  displayName: definition.displayName,
                  description: definition.description,
                  iconName: definition.iconName,
                },
              },
              lifetime,
            )
          ).resourceId,
        )
        await operations.register(root, (ref) => refs.push(ref))
        const viewers = new ViewerRegistryResourceServiceClient(root.client)
        retain(
          (
            await viewers.RegisterViewer(
              {
                registration: {
                  typeId: appObjectTypeID(definition.app.schema),
                  viewerName: definition.displayName,
                  componentId: definition.viewer.componentId,
                  scriptPath: await appViewerPath(
                    api,
                    definition.viewer.entry,
                    executable.manifestRoot,
                    lifetime,
                  ),
                  surface: ViewerSurface.WEB,
                },
              },
              lifetime,
            )
          ).resourceId,
        )
        if (definition.quickstart) {
          const quickstarts = new QuickstartRegistryResourceServiceClient(
            root.client,
          )
          retain(
            (
              await quickstarts.RegisterQuickstart(
                {
                  registration: {
                    quickstartId: definition.app.schema.id,
                    pluginId: executable.pluginId,
                    name: definition.displayName,
                    description:
                      definition.description ?? definition.displayName,
                    iconName: definition.iconName,
                    category: 'apps',
                    spaceName: definition.displayName,
                    requiredPluginIds: [executable.pluginId],
                  },
                },
                lifetime,
              )
            ).resourceId,
          )
        }

        // Initial loads publish only after every registration succeeds. Replacements
        // wait for the scheduler, which still retains the previous worker.
        if (!host.prepared) {
          await new GenerationServiceClient(root.client).Activate({}, lifetime)
        }
      }
    })()
    const done = startup
      .then(() => waitForAbort(lifetime))
      .finally(() => {
        generation?.release()
        for (const ref of refs.reverse()) ref.release()
        detachHandlers?.()
        controller.abort()
        resources?.dispose()
      })
    return { startup, done }
  }
}

/** appViewerPath resolves the frontend service binding or immutable build asset. */
async function appViewerPath(
  api: BackendAPI,
  entry: string,
  manifestRoot: string,
  signal: AbortSignal,
): Promise<string> {
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
  const asset = entries[entry.replace(/^\.\//, '')]
  if (asset?.frontendBinding) return frontendBindingPath(asset.frontendBinding)
  if (!asset?.file)
    throw new Error('Application viewer is absent from the build manifest')
  return api.utils.pluginAssetHttpPath(
    `${api.startInfo.pluginId!}/manifest/${manifestRoot}`,
    `v/b/fe/${asset.file}`,
  )
}

/** waitForAbort joins the worker lifetime without holding a transaction. */
function waitForAbort(signal: AbortSignal): Promise<void> {
  if (signal.aborted) return Promise.resolve()
  return new Promise((resolve) =>
    signal.addEventListener('abort', () => resolve(), { once: true }),
  )
}
