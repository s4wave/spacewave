import { HandleStreamCtr } from 'starpc'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { BackendAPI, BackendEntrypointLifecycle } from '@aptre/bldr-sdk'
import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { withResourceCall } from '@aptre/bldr-sdk/resource/server/context.js'
import { SqlQueryResourceServiceDefinition } from '@s4wave/sdk/sql/query/query_srpc.pb.js'
import { CounterResourceServiceDefinition } from './testdata/counter_srpc.pb.js'

const fakes = vi.hoisted(() => ({
  events: [] as string[],
  hostRoot: undefined as FakeRef | undefined,
  coreRoot: undefined as FakeRef | undefined,
  nextObjectResourceId: 10,
  nextViewerResourceId: 20,
}))

vi.mock(
  '@go/github.com/s4wave/spacewave/bldr/sdk/plugin/host/host_srpc.pb.js',
  () => ({
    PluginHostResourceServiceClient: class {
      async RegisterObjectType(request: { typeId?: string }) {
        fakes.events.push('object:' + request.typeId)
        return { resourceId: fakes.nextObjectResourceId }
      }
    },
  }),
)

vi.mock('@s4wave/sdk/viewer/registry/registry_srpc.pb.js', () => ({
  ViewerRegistryResourceServiceClient: class {
    async RegisterViewer(request: { registration?: { componentId?: string } }) {
      fakes.events.push('viewer:' + request.registration?.componentId)
      return { resourceId: fakes.nextViewerResourceId }
    }
  },
}))

vi.mock('@aptre/bldr-sdk/resource/index.js', async (importOriginal) => {
  const original =
    await importOriginal<typeof import('@aptre/bldr-sdk/resource/index.js')>()
  return {
    ...original,
    Client: class {
      accessRootResource() {
        return Promise.resolve(fakes.coreRoot)
      }
    },
    ResourceServiceClient: class {},
  }
})

import {
  DeclaredObjectTypeHandler,
  definePlugin,
  type ObjectTypeDeclaration,
} from './definition.js'

class FakeRef implements ClientResourceRef {
  readonly client = {} as ClientResourceRef['client']
  released = false

  constructor(
    readonly resourceId: number,
    private readonly label: string,
  ) {}

  createRef(id: number): ClientResourceRef {
    const ref = new FakeRef(id, this.label + ':' + id)
    fakes.events.push('retain:' + this.label + ':' + id)
    return ref
  }

  createResource<T, Args extends unknown[]>(
    _id: number,
    _ResourceClass: new (ref: ClientResourceRef, ...args: Args) => T,
    ..._args: Args
  ): T {
    throw new Error('not used')
  }

  release(): void {
    if (this.released) return
    this.released = true
    fakes.events.push('release:' + this.label)
  }

  [Symbol.dispose](): void {
    this.release()
  }
}

function backendAPI(hostRoot: FakeRef): BackendAPI {
  return {
    startInfo: { pluginId: 'fixture' },
    handleStreamCtr: new HandleStreamCtr(),
    resourceClient: {
      accessRootResource: () => Promise.resolve(hostRoot),
    },
    buildPluginOpenStream: () => () => Promise.reject(new Error('unused')),
  } as unknown as BackendAPI
}

function sqlHandler() {
  return {
    Initialize: () => Promise.resolve({}),
    GetQueryText: () => Promise.resolve({}),
    SetQueryText: () => Promise.resolve({}),
    SetParameters: () => Promise.resolve({}),
    Run: () => Promise.resolve({}),
  }
}

function lifecycle(result: unknown): BackendEntrypointLifecycle {
  return result as BackendEntrypointLifecycle
}

describe('definePlugin', () => {
  beforeEach(() => {
    fakes.events.length = 0
    fakes.hostRoot = new FakeRef(1, 'host-root')
    fakes.coreRoot = new FakeRef(2, 'core-root')
    fakes.nextObjectResourceId = 10
    fakes.nextViewerResourceId = 20
  })

  it('completes empty declaration startup without opening registration roots', async () => {
    const controller = new AbortController()
    const api = backendAPI(fakes.hostRoot!)
    const result = lifecycle(definePlugin({})(api, controller.signal))

    await result.startup
    expect(fakes.events).toEqual([])
    expect(api.handleStreamCtr.value).toBeTypeOf('function')

    controller.abort()
    await result.done
    expect(api.handleStreamCtr.value).toBeUndefined()
  })

  it('registers ObjectTypes before viewers and retains references until abort', async () => {
    const controller = new AbortController()
    const api = backendAPI(fakes.hostRoot!)
    const entrypoint = definePlugin({
      objectTypes: [
        {
          typeId: 'example/counter',
          service: SqlQueryResourceServiceDefinition,
          create: sqlHandler,
        },
      ],
      viewers: [
        {
          typeId: 'example/counter',
          componentId: 'example.counter.viewer',
          viewerName: 'Counter',
          scriptPath: './CounterViewer.js',
        },
      ],
    })
    const result = lifecycle(entrypoint(api, controller.signal))

    await result.startup
    expect(fakes.events).toEqual([
      'object:example/counter',
      'retain:host-root:10',
      'viewer:example.counter.viewer',
      'retain:core-root:20',
    ])
    expect(fakes.hostRoot!.released).toBe(false)
    expect(fakes.coreRoot!.released).toBe(false)

    controller.abort()
    await result.done
    expect(fakes.events.slice(-4)).toEqual([
      'release:core-root:20',
      'release:core-root',
      'release:host-root:10',
      'release:host-root',
    ])
  })

  it('releases retained state when startup registration fails', async () => {
    fakes.nextObjectResourceId = 0
    const controller = new AbortController()
    const api = backendAPI(fakes.hostRoot!)
    const result = lifecycle(
      definePlugin({
        objectTypes: [
          {
            typeId: 'example/counter',
            service: SqlQueryResourceServiceDefinition,
            create: sqlHandler,
          },
        ],
      })(api, controller.signal),
    )

    await expect(result.startup).rejects.toThrow(
      'example/counter object type registration did not return a resource id',
    )
    expect(fakes.hostRoot!.released).toBe(true)
    expect(api.handleStreamCtr.value).toBeUndefined()
    controller.abort()
    await result.done
  })
})

describe('DeclaredObjectTypeHandler', () => {
  it('dispatches the generated service and releases its attached Engine', async () => {
    const invocationController = new AbortController()
    const engineRef = new FakeRef(30, 'engine')
    let childRelease: (() => void) | undefined
    let childMux:
      | { lookupMethod(service: string, method: string): Promise<unknown> }
      | undefined
    let receivedObjectKey = ''
    let receivedSignal: AbortSignal | undefined
    let handlerDisposed = false
    const declaration: ObjectTypeDeclaration = {
      typeId: 'example/counter',
      service: CounterResourceServiceDefinition,
      create: ({ objectKey, signal }) => {
        receivedObjectKey = objectKey
        receivedSignal = signal
        return {
          Initialize: () => Promise.resolve({}),
          GetCounter: () => Promise.resolve({ value: 7n }),
          [Symbol.dispose]: () => {
            handlerDisposed = true
          },
        }
      },
    }
    const client = {
      signal: invocationController.signal,
      getAttachedRef: () => engineRef,
      addResource: (mux: typeof childMux, release: () => void) => {
        childMux = mux
        childRelease = release
        return 41
      },
    }
    const context = withResourceCall(
      { signal: invocationController.signal },
      {
        client: client as never,
        parentResourceId: 1,
        serviceId: 'object-type',
        methodId: 'invoke',
      },
    )
    const handler = new DeclaredObjectTypeHandler(
      new Map([[declaration.typeId, declaration]]),
    )

    const response = await handler.InvokeObjectType(
      {
        typeId: declaration.typeId,
        objectKey: 'counter/main',
        attachedEngineResourceId: engineRef.resourceId,
      },
      invocationController.signal,
      context,
    )

    expect(response.resourceId).toBe(41)
    expect(receivedObjectKey).toBe('counter/main')
    expect(receivedSignal).toBeInstanceOf(AbortSignal)
    expect(
      await childMux!.lookupMethod(
        CounterResourceServiceDefinition.typeName,
        CounterResourceServiceDefinition.methods.GetCounter.name,
      ),
    ).toBeTypeOf('function')

    childRelease!()
    expect(handlerDisposed).toBe(true)
    expect(engineRef.released).toBe(true)
    expect(receivedSignal!.aborted).toBe(true)
  })
})
