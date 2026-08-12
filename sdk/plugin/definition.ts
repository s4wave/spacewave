import {
  Client as SRPCClient,
  Server,
  createHandler,
  createMux,
  handleRpcStream,
  type MessageStream,
  type RpcStreamPacket,
  type ServerContext,
} from 'starpc'
import { MethodKind, type MessageType } from '@aptre/protobuf-es-lite'
import type {
  BackendEntrypointFunc,
  BackendEntrypointLifecycle,
} from '@aptre/bldr-sdk'
import {
  Client as ResourcesClient,
  ResourceServiceClient,
  type ClientResourceRef,
} from '@aptre/bldr-sdk/resource/index.js'
import {
  ResourceServer,
  getResourceCall,
  newResourceMux,
} from '@aptre/bldr-sdk/resource/server/index.js'
import {
  PluginDefinition,
  type PluginHandler,
} from '@go/github.com/s4wave/spacewave/bldr/plugin/plugin_srpc.pb.js'
import { PluginHostResourceServiceClient } from '@go/github.com/s4wave/spacewave/bldr/sdk/plugin/host/host_srpc.pb.js'
import type { ObjectTypeMetadata } from '@s4wave/sdk/objecttype/registry/registry.pb.js'
import {
  ObjectTypeHandlerServiceDefinition,
  type ObjectTypeHandlerServiceHandler,
} from '@s4wave/sdk/objecttype/registry/registry_srpc.pb.js'
import type { ViewerRegistration } from '@s4wave/sdk/viewer/registry/registry.pb.js'
import { ViewerRegistryResourceServiceClient } from '@s4wave/sdk/viewer/registry/registry_srpc.pb.js'
import { Engine } from '@s4wave/sdk/world/engine.js'

// ResourceServiceDefinition is a generated StarRPC Resource service definition.
export type ResourceServiceDefinition = Parameters<typeof createHandler>[0]

type MessageOf<T> = T extends MessageType<infer Message> ? Message : never

type ResourceServiceMethod<Method> = Method extends {
  I: infer Request
  O: infer Response
  kind: infer Kind
}
  ? Kind extends MethodKind.Unary
    ? (
        request: MessageOf<Request>,
        signal: AbortSignal,
        context: ServerContext,
      ) => Promise<MessageOf<Response>>
    : Kind extends MethodKind.ServerStreaming
      ? (
          request: MessageOf<Request>,
          signal: AbortSignal,
          context: ServerContext,
        ) => MessageStream<MessageOf<Response>>
      : Kind extends MethodKind.ClientStreaming
        ? (
            request: MessageStream<MessageOf<Request>>,
            signal: AbortSignal,
            context: ServerContext,
          ) => Promise<MessageOf<Response>>
        : (
            request: MessageStream<MessageOf<Request>>,
            signal: AbortSignal,
            context: ServerContext,
          ) => MessageStream<MessageOf<Response>>
  : never

// ResourceServiceHandler implements the methods in a generated Resource service definition.
export type ResourceServiceHandler<Service extends ResourceServiceDefinition> =
  {
    [Method in keyof Service['methods']]: ResourceServiceMethod<
      Service['methods'][Method]
    >
  } & Partial<Disposable>

// ObjectTypeHandlerContext contains the invocation-scoped values supplied to an ObjectType handler.
export interface ObjectTypeHandlerContext {
  // engine is the attached World Engine for the requested object.
  readonly engine: Engine
  // objectKey identifies the requested World object.
  readonly objectKey: string
  // signal ends when the returned Resource is released.
  readonly signal: AbortSignal
}

// ObjectTypeDeclaration declares one ObjectType and its generated Resource service.
export interface ObjectTypeDeclaration<
  Service extends ResourceServiceDefinition = ResourceServiceDefinition,
> {
  // typeId is the stable ObjectType identifier.
  readonly typeId: string
  // metadata describes the ObjectType in user-facing registries.
  readonly metadata?: ObjectTypeMetadata
  // service is the generated Resource service served for the ObjectType.
  readonly service: Service
  // create constructs one invocation-scoped Resource service handler.
  readonly create: (
    context: ObjectTypeHandlerContext,
  ) => ResourceServiceHandler<Service>
}

// PluginDefinitionConfig contains the global definitions registered by one plugin.
export interface PluginDefinitionConfig<
  Services extends readonly ResourceServiceDefinition[] =
    readonly ResourceServiceDefinition[],
> {
  // objectTypes contains the ObjectTypes served by the plugin.
  readonly objectTypes?: {
    readonly [Index in keyof Services]: ObjectTypeDeclaration<Services[Index]>
  }
  // viewers contains the web or terminal viewers supplied by the plugin.
  readonly viewers?: readonly ViewerRegistration[]
}

type ObjectTypeRuntimeDeclaration = {
  readonly typeId: string
  readonly metadata?: ObjectTypeMetadata
  readonly service: ResourceServiceDefinition
  readonly create: (
    context: ObjectTypeHandlerContext,
  ) => Parameters<typeof createHandler>[1] & Partial<Disposable>
}

export class DeclaredObjectTypeHandler implements ObjectTypeHandlerServiceHandler {
  constructor(
    private readonly declarations: ReadonlyMap<
      string,
      ObjectTypeRuntimeDeclaration
    >,
  ) {}

  InvokeObjectType(
    request: {
      typeId?: string
      objectKey?: string
      attachedEngineResourceId?: number
    },
    _abortSignal: AbortSignal,
    context: ServerContext,
  ): Promise<{ resourceId: number }> {
    const typeId = request.typeId ?? ''
    const declaration = this.declarations.get(typeId)
    if (!declaration) {
      return Promise.reject(new Error('unhandled object type: ' + typeId))
    }
    const engineResourceId = request.attachedEngineResourceId ?? 0
    if (!engineResourceId) {
      return Promise.reject(new Error('attachedEngineResourceId is required'))
    }

    const call = getResourceCall(context)
    const { resourceId } = call.constructChildResource((signal) => {
      const engine = new Engine(call.getAttachedRef(engineResourceId))
      let handler: Parameters<typeof createHandler>[1] & Partial<Disposable>
      try {
        handler = declaration.create({
          engine,
          objectKey: request.objectKey ?? '',
          signal,
        })
      } catch (error) {
        engine.release()
        throw error
      }
      return {
        mux: newResourceMux(createHandler(declaration.service, handler)),
        result: undefined,
        releaseFn: () => {
          const disposable = handler as Partial<Disposable>
          try {
            disposable[Symbol.dispose]?.()
          } finally {
            engine.release()
          }
        },
      }
    })
    return Promise.resolve({ resourceId })
  }
}

class DeclaredPlugin implements PluginHandler {
  constructor(private readonly resourceServer: Server) {}

  PluginRpc(
    request: MessageStream<RpcStreamPacket>,
    _abortSignal?: AbortSignal,
  ): MessageStream<RpcStreamPacket> {
    return handleRpcStream(request[Symbol.asyncIterator](), () =>
      Promise.resolve(this.resourceServer.rpcStreamHandler),
    )
  }
}

function waitForAbort(signal: AbortSignal): Promise<void> {
  if (signal.aborted) return Promise.resolve()
  return new Promise((resolve) => {
    signal.addEventListener('abort', () => resolve(), { once: true })
  })
}

function requireResourceRef(
  parent: ClientResourceRef,
  resourceId: number | undefined,
  label: string,
): ClientResourceRef {
  if (!resourceId) {
    throw new Error(label + ' registration did not return a resource id')
  }
  return parent.createRef(resourceId)
}

// definePlugin builds a Bldr backend entrypoint from ObjectType and viewer declarations.
export function definePlugin<
  const Services extends readonly ResourceServiceDefinition[],
>(config: PluginDefinitionConfig<Services>): BackendEntrypointFunc {
  const objectTypes = config.objectTypes ?? []
  const viewers = config.viewers ?? []
  const declarations = new Map<string, ObjectTypeRuntimeDeclaration>()
  for (const declaration of objectTypes) {
    if (declarations.has(declaration.typeId)) {
      throw new Error('duplicate object type: ' + declaration.typeId)
    }
    declarations.set(
      declaration.typeId,
      declaration as ObjectTypeRuntimeDeclaration,
    )
  }

  return (api, signal): BackendEntrypointLifecycle => {
    const handler = new DeclaredObjectTypeHandler(declarations)
    const resourceServer = new ResourceServer(
      newResourceMux(
        createHandler(ObjectTypeHandlerServiceDefinition, handler),
      ),
    )
    const outerMux = createMux()
    resourceServer.register(outerMux)
    const resourceRpcServer = new Server(outerMux.lookupMethod)
    const plugin = new DeclaredPlugin(resourceRpcServer)
    const pluginMux = createMux()
    pluginMux.register(createHandler(PluginDefinition, plugin))
    const pluginServer = new Server(pluginMux.lookupMethod)
    api.handleStreamCtr.set((channel) => {
      pluginServer.handlePacketStream(channel)
      return Promise.resolve()
    })

    const refs: ClientResourceRef[] = []
    let released = false
    const release = () => {
      if (released) return
      released = true
      api.handleStreamCtr.set(undefined)
      for (let i = refs.length - 1; i >= 0; --i) {
        refs[i].release()
      }
    }
    signal.addEventListener('abort', release, { once: true })

    const startup = (async () => {
      try {
        const pluginId = api.startInfo.pluginId
        if (!pluginId)
          throw new Error('missing plugin id in backend start info')

        if (objectTypes.length > 0) {
          const hostRoot = await api.resourceClient.accessRootResource()
          refs.push(hostRoot)
          const host = new PluginHostResourceServiceClient(hostRoot.client)
          await Promise.all(
            objectTypes.map(async (declaration) => {
              const response = await host.RegisterObjectType(
                { typeId: declaration.typeId, metadata: declaration.metadata },
                signal,
              )
              refs.push(
                requireResourceRef(
                  hostRoot,
                  response.resourceId,
                  declaration.typeId + ' object type',
                ),
              )
            }),
          )
        }

        if (viewers.length > 0) {
          const coreClient = new SRPCClient(
            api.buildPluginOpenStream('spacewave-core'),
          )
          const resourcesClient = new ResourcesClient(
            new ResourceServiceClient(coreClient),
            signal,
          )
          const coreRoot = await resourcesClient.accessRootResource()
          refs.push(coreRoot)
          const registry = new ViewerRegistryResourceServiceClient(
            coreRoot.client,
          )
          await Promise.all(
            viewers.map(async (viewer) => {
              const response = await registry.RegisterViewer(
                { registration: viewer },
                signal,
              )
              refs.push(
                requireResourceRef(
                  coreRoot,
                  response.resourceId,
                  viewer.componentId + ' viewer',
                ),
              )
            }),
          )
        }
      } catch (error) {
        release()
        throw error
      }
    })()

    const done = (async () => {
      await waitForAbort(signal)
      release()
    })()

    return { startup, done }
  }
}
