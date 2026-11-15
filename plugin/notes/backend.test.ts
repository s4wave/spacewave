import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ViewerSurface } from '@s4wave/sdk/viewer/registry/registry.pb.js'

const h = vi.hoisted(() => ({
  buildPluginOpenStream: vi.fn(),
  handleStreamSet: vi.fn(),
  handleRpcStream: vi.fn(),
  pluginAssetHttpPath: vi.fn(),
  pluginDefinition: { typeName: 'bldr.plugin.Plugin' },
  createdHandlers: [] as Array<{ definition: unknown; handler: unknown }>,
  serverInstances: [] as Array<{
    rpcStreamHandler: unknown
    handlePacketStream: ReturnType<typeof vi.fn>
  }>,
  handleRpcStreamCalls: [] as Array<{
    iterator: unknown
    getter: (componentId: string) => Promise<unknown>
  }>,
  objectTypeRegistrations: [] as Array<Record<string, unknown>>,
  worldOpRegistrations: [] as Array<Record<string, unknown>>,
  quickstartRegistrations: [] as Array<Record<string, unknown>>,
  wizardRegistrations: [] as Array<Record<string, unknown>>,
  viewerRegistrations: [] as Array<Record<string, unknown>>,
  retainedRefs: [] as Array<{
    resourceId: number
    ref: { [Symbol.dispose](): void }
  }>,
  accessRootResource: undefined as undefined | (() => Promise<unknown>),
  prepareRequests: [] as Array<Record<string, unknown>>,
  activatedScopes: [] as unknown[],
  activationReady: undefined as undefined | (() => Promise<unknown>),
  quickstartRegistrationFailureId: undefined as string | undefined,
  viewerRegistrationFailure: undefined as Error | undefined,
  nextResourceId: 1,
  executeWorldBatch: vi.fn(),
  engineRelease: vi.fn(),
  stageRelease: vi.fn(),
  // staged lists what each seed built in its World stage, in order.
  staged: [] as string[],
  // world maps each object key in the seed test's Space to its type, '' when
  // untyped; seedWrites lists every seed write in the order it was made.
  world: new Map<string, string>(),
  seedWrites: [] as string[],
  rootRef: undefined as unknown as TestRef,
  scopeRef: undefined as unknown as TestRef,
}))

type TestRef = {
  client: Record<string, never>
  createRef(resourceId: number): { [Symbol.dispose](): void }
  [Symbol.dispose](): void
}

vi.mock('starpc', async (importOriginal) => ({
  ...(await importOriginal<typeof import('starpc')>()),
  createMux: vi.fn(() => ({ lookupMethod: vi.fn(), register: vi.fn() })),
  createHandler: vi.fn((definition: unknown, handler: unknown) => {
    const created = { definition, handler }
    h.createdHandlers.push(created)
    return created
  }),
  handleRpcStream: vi.fn(
    (iterator: unknown, getter: (componentId: string) => Promise<unknown>) => {
      h.handleRpcStream(iterator, getter)
      h.handleRpcStreamCalls.push({ iterator, getter })
      return { [Symbol.asyncIterator]: async function* () {} }
    },
  ),
  Server: class {
    rpcStreamHandler = vi.fn()
    handlePacketStream = vi.fn()

    constructor(_lookupMethod: unknown) {
      h.serverInstances.push(this)
    }
  },
  Client: class {
    constructor(stream: unknown) {
      h.buildPluginOpenStream(stream)
    }
  },
}))

vi.mock(
  '@go/github.com/s4wave/spacewave/bldr/plugin/plugin_srpc.pb.js',
  () => ({
    PluginDefinition: h.pluginDefinition,
    ActivationDefinition: { typeName: 'bldr.plugin.Activation' },
  }),
)

vi.mock('@s4wave/sdk/plugin/registration/generation.js', () => ({
  prepareRegistrations: vi.fn(
    (_core: unknown, request: Record<string, unknown>) => {
      h.prepareRequests.push(request)
      return Promise.resolve(h.scopeRef)
    },
  ),
  activateRegistrations: vi.fn((scope: unknown) => {
    h.activatedScopes.push(scope)
    return Promise.resolve()
  }),
  createActivationHandler: vi.fn((ready: () => Promise<unknown>) => {
    h.activationReady = ready
    return {}
  }),
}))

vi.mock('@aptre/bldr-sdk/resource/index.js', () => ({
  ResourceServiceClient: class {
    constructor(_client: unknown) {}
  },
  Client: class {
    constructor(_service: unknown, _signal: AbortSignal) {}

    accessRootResource() {
      if (h.accessRootResource) {
        return h.accessRootResource()
      }
      return Promise.resolve(h.rootRef)
    }
  },
}))

vi.mock('@aptre/bldr-sdk/resource/server/index.js', () => ({
  ResourceServer: class {
    constructor(_mux: unknown) {}

    register(_mux: unknown) {}
  },
  getResourceCall: vi.fn(() => ({
    constructChildResource: vi.fn(
      (factory: (signal: AbortSignal) => unknown) => {
        factory(new AbortController().signal)
        return { resourceId: 9001 }
      },
    ),
    getAttachedRef: vi.fn(() => ({ release: vi.fn() })),
  })),
  newResourceMux: vi.fn((...handlers: unknown[]) => ({ handlers })),
}))

vi.mock('@s4wave/sdk/objecttype/registry/registry_srpc.pb.js', () => ({
  ObjectTypeHandlerServiceDefinition: {},
  ObjectTypeRegistryResourceServiceClient: class {
    constructor(_client: unknown) {}

    RegisterObjectType(req: Record<string, unknown>) {
      h.objectTypeRegistrations.push(req)
      return Promise.resolve({ resourceId: h.nextResourceId++ })
    }
  },
}))

vi.mock('@s4wave/sdk/worldop/registry/registry_srpc.pb.js', () => ({
  WorldOpHandlerServiceDefinition: {},
  WorldOpRegistryResourceServiceClient: class {
    constructor(_client: unknown) {}

    RegisterWorldOp(req: Record<string, unknown>) {
      h.worldOpRegistrations.push(req)
      return Promise.resolve({ resourceId: h.nextResourceId++ })
    }
  },
}))

vi.mock('@s4wave/sdk/quickstart/registry/registry_srpc.pb.js', () => ({
  QuickstartHandlerServiceDefinition: {},
  QuickstartRegistryResourceServiceClient: class {
    constructor(_client: unknown) {}

    RegisterQuickstart(req: { registration?: Record<string, unknown> }) {
      h.quickstartRegistrations.push(req.registration ?? {})
      if (
        h.quickstartRegistrationFailureId &&
        req.registration?.quickstartId === h.quickstartRegistrationFailureId
      ) {
        return Promise.reject(new Error('quickstart registry unavailable'))
      }
      return Promise.resolve({ resourceId: h.nextResourceId++ })
    }
  },
}))

vi.mock('@s4wave/sdk/world/wizard/wizard_srpc.pb.js', () => ({
  ObjectWizardRegistryResourceServiceClient: class {
    constructor(_client: unknown) {}

    RegisterWizard(req: { wizard?: Record<string, unknown> }) {
      h.wizardRegistrations.push(req.wizard ?? {})
      return Promise.resolve({ resourceId: h.nextResourceId++ })
    }
  },
}))

vi.mock('@s4wave/sdk/viewer/registry/registry_srpc.pb.js', () => ({
  ViewerRegistryResourceServiceClient: class {
    constructor(_client: unknown) {}

    RegisterViewer(req: { registration: Record<string, unknown> }) {
      h.viewerRegistrations.push(req.registration)
      if (h.viewerRegistrationFailure) {
        return Promise.reject(h.viewerRegistrationFailure)
      }
      return Promise.resolve({ resourceId: h.nextResourceId++ })
    }
  },
}))

vi.mock('@go/github.com/s4wave/spacewave/db/unixfs/rpc/rpc_srpc.pb.js', () => ({
  FSCursorServiceClient: class {
    constructor(_client: unknown, _options: unknown) {}
  },
}))

vi.mock(
  '@go/github.com/s4wave/spacewave/db/unixfs/rpc/client/fs-handle.js',
  () => ({
    buildFSHandle: vi.fn(() => {
      const manifest = JSON.stringify({
        'plugin/notes/NotebookViewer.tsx': { file: 'assets/notebook.mjs' },
        'plugin/notes/BlogViewer.tsx': { file: 'assets/blog.mjs' },
        'plugin/notes/DocsViewer.tsx': { file: 'assets/docs.mjs' },
        'plugin/notes/NotesWizardViewer.tsx': {
          file: 'assets/notes-wizard.mjs',
        },
      })
      const data = new TextEncoder().encode(manifest)
      const handle = {
        getSize: vi.fn(() => Promise.resolve(BigInt(data.byteLength))),
        readAt: vi.fn(() => Promise.resolve({ data })),
        [Symbol.dispose]: vi.fn(),
      }
      return Promise.resolve({
        lookupPath: vi.fn(() => Promise.resolve({ handle })),
        [Symbol.dispose]: vi.fn(),
      })
    }),
  }),
)

vi.mock('@s4wave/sdk/world/engine.js', () => ({
  Engine: class {
    constructor(_ref: unknown) {}
    executeWorldBatch = h.executeWorldBatch
    release = h.engineRelease
    stageWorldState() {
      const stage = {
        buildStorageCursor: () => Promise.resolve({ [Symbol.dispose]() {} }),
        release: h.stageRelease,
        [Symbol.dispose]: () => stage.release(),
      }
      return Promise.resolve(stage)
    }
  },
}))

// stagedRoot is the root of content staged under name.
function stagedRoot(name: string) {
  h.staged.push(name)
  return { rootRef: { hash: { hash: new TextEncoder().encode(name) } } }
}

vi.mock('@s4wave/sdk/unixfs/world/stage.js', () => ({
  buildStagedTree: vi.fn((_stage: unknown, files: Array<{ path: string }>) =>
    Promise.resolve(stagedRoot('tree ' + files.map((f) => f.path).join(','))),
  ),
}))

// The seed writes act on h.world the way the World does: a create or a UnixFS
// init on a key that exists fails, and a UnixFS init sets the UnixFS type.
function createSeedObject(key: string, kind: string, typeId = '') {
  if (h.world.has(key)) throw new Error('object already exists: ' + key)
  h.world.set(key, typeId)
  h.seedWrites.push(kind + ' ' + key)
}

vi.mock('@s4wave/sdk/world/world-state.js', async (importOriginal) => ({
  ...(await importOriginal<
    typeof import('@s4wave/sdk/world/world-state.js')
  >()),
  WorldStateResource: class {
    constructor(_ref: unknown) {}
    getObject(key: string) {
      return Promise.resolve(h.world.has(key) ? { release: vi.fn() } : null)
    }
    createObject(key: string) {
      createSeedObject(key, 'create')
      return Promise.resolve({ [Symbol.dispose]() {} })
    }
    applyWorldOp(_opTypeId: string, opData: Uint8Array) {
      const key = InitUnixFSOp.fromBinary(opData).objectKey ?? ''
      createSeedObject(key, 'init', 'unixfs/fs-node')
      return Promise.resolve()
    }
    release() {}
  },
}))

vi.mock('./object-block.js', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./object-block.js')>()),
  createObjectWithBlockData: vi.fn((_ws: unknown, key: string) => {
    createSeedObject(key, 'create')
    return Promise.resolve()
  }),
  stageObjectBlock: vi.fn(() => Promise.resolve(stagedRoot('block'))),
}))

vi.mock('./unixfs-seed.js', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./unixfs-seed.js')>()),
  uploadSeedTree: vi.fn((_ws: unknown, key: string) => {
    h.seedWrites.push('upload ' + key)
    return Promise.resolve()
  }),
}))

vi.mock('@s4wave/sdk/world/types/types.js', async (importOriginal) => ({
  ...(await importOriginal<
    typeof import('@s4wave/sdk/world/types/types.js')
  >()),
  getObjectType: vi.fn((_ws: unknown, key: string) =>
    Promise.resolve(h.world.get(key) ?? ''),
  ),
  setObjectType: vi.fn((_ws: unknown, key: string, typeId: string) => {
    h.world.set(key, typeId)
    h.seedWrites.push('type ' + key)
    return Promise.resolve()
  }),
}))

import main from './backend.js'
import { InitUnixFSOp } from '@s4wave/core/space/world/ops/ops.pb.js'
import { WorldErrorCode } from '../../sdk/world/world.pb.js'
import {
  PluginOpTimeoutError,
  WorldBatchRefusal,
  WorldOperationRejection,
} from '../../sdk/world/errors.js'
import { InitNotebookOp, InitNotebookStagedOp } from './proto/notebook.pb.js'
import { CreateBlogOp } from './proto/blog.pb.js'
import { CreateDocumentationOp } from './proto/docs.pb.js'
import { CREATE_BLOG_OP_ID } from './proto/create-blog.js'
import { CREATE_DOCS_OP_ID } from './proto/create-docs.js'
import { INIT_NOTEBOOK_OP_ID } from './proto/init-notebook.js'
import { Hash } from '../../net/hash/hash.pb.js'
import { base58Encode } from '../../net/peer/base58.js'
import { pinnedOperationID } from '../../sdk/sync/instance.js'
import { WorldOpHandlerServiceDefinition } from '@s4wave/sdk/worldop/registry/registry_srpc.pb.js'
import { SEED_KEY_TAKEN_CODE } from './seed-steps.js'

const manifestHash = { hash: new Uint8Array([1]) }
const manifestRoot = base58Encode(Hash.toBinary(manifestHash))

function buildApi(
  pluginId: string,
  host: { historical?: boolean; prepared?: boolean; instanceKey?: string } = {},
) {
  return {
    startInfo: { pluginId },
    pluginHost: {
      GetPluginInfo: vi.fn(async () => ({
        pluginId,
        manifestRef: { manifestRef: { rootRef: { hash: manifestHash } } },
        historical: false,
        ...host,
      })),
    },
    client: {},
    handleStreamCtr: { set: h.handleStreamSet },
    buildPluginOpenStream: vi.fn((target: string) => target),
    utils: {
      pluginAssetHttpPath: h.pluginAssetHttpPath,
    },
  }
}

async function startMain(
  api: Parameters<typeof main>[0],
  signal: AbortSignal,
): Promise<ReturnType<typeof main>> {
  const lifecycle = main(api, signal)
  await lifecycle.startup
  return lifecycle
}

function buildRef(): TestRef {
  return {
    client: {},
    createRef: vi.fn((resourceId: number) => {
      const ref = { [Symbol.dispose]: vi.fn() }
      h.retainedRefs.push({ resourceId, ref })
      return ref
    }),
    [Symbol.dispose]: vi.fn(),
  }
}

describe('notes backend registration', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    h.objectTypeRegistrations.length = 0
    h.worldOpRegistrations.length = 0
    h.quickstartRegistrations.length = 0
    h.wizardRegistrations.length = 0
    h.viewerRegistrations.length = 0
    h.retainedRefs.length = 0
    h.createdHandlers.length = 0
    h.serverInstances.length = 0
    h.handleRpcStreamCalls.length = 0
    h.accessRootResource = undefined
    h.prepareRequests.length = 0
    h.activatedScopes.length = 0
    h.activationReady = undefined
    h.quickstartRegistrationFailureId = undefined
    h.viewerRegistrationFailure = undefined
    h.nextResourceId = 1
    h.pluginAssetHttpPath.mockImplementation(
      (pluginId: string, path: string) => `/asset/${pluginId}/${path}`,
    )
    h.rootRef = buildRef()
    h.scopeRef = buildRef()
  })

  it('registers notes interfaces with the startInfo plugin id and retained lifetimes', async () => {
    const abort = new AbortController()

    await startMain(buildApi('spacewave-notes') as never, abort.signal)

    expect(h.objectTypeRegistrations).toEqual([
      { typeId: 'notes/notebook', pluginId: 'spacewave-notes' },
      { typeId: 'notes/blog', pluginId: 'spacewave-notes' },
      { typeId: 'notes/docs', pluginId: 'spacewave-notes' },
    ])
    expect(h.worldOpRegistrations).toEqual([
      ...['create', 'mutate', 'upgrade'].map((action) => ({
        operationTypeId: pinnedOperationID(
          { pluginId: 'spacewave-notes', manifestRoot },
          `notes/saved-views/${action}`,
        ),
        pluginId: 'spacewave-notes',
      })),
      {
        operationTypeId: 'notes/notebook/init-staged',
        pluginId: 'spacewave-notes',
      },
      {
        operationTypeId: 'notes/blog/create-staged',
        pluginId: 'spacewave-notes',
      },
      {
        operationTypeId: 'notes/docs/create-staged',
        pluginId: 'spacewave-notes',
      },
      { operationTypeId: 'notes/notebook/edit', pluginId: 'spacewave-notes' },
      { operationTypeId: 'notes/notebook/init', pluginId: 'spacewave-notes' },
      { operationTypeId: 'notes/blog/create', pluginId: 'spacewave-notes' },
      { operationTypeId: 'notes/docs/create', pluginId: 'spacewave-notes' },
    ])
    expect(h.quickstartRegistrations).toEqual([
      {
        quickstartId: 'notebook',
        pluginId: 'spacewave-notes',
        name: 'Create a Notebook',
        description: 'Markdown notes with folders, tags, and sync',
        category: 'storage',
        iconName: 'notebook',
        hidden: true,
        experimental: true,
        spaceName: 'My Notebook',
        requiredPluginIds: ['spacewave-notes'],
      },
      {
        quickstartId: 'docs',
        pluginId: 'spacewave-notes',
        name: 'Create Documentation',
        description: 'Markdown documentation site',
        category: 'content',
        iconName: 'notebook',
        hidden: true,
        experimental: true,
        spaceName: 'My Docs',
        requiredPluginIds: ['spacewave-notes'],
      },
      {
        quickstartId: 'blog',
        pluginId: 'spacewave-notes',
        name: 'Create a Blog',
        description: 'Date-based markdown blog',
        category: 'content',
        iconName: 'pen',
        hidden: true,
        experimental: true,
        spaceName: 'My Blog',
        requiredPluginIds: ['spacewave-notes'],
      },
    ])
    expect(h.wizardRegistrations).toEqual([
      {
        typeId: 'notes/notebook',
        pluginId: 'spacewave-notes',
        displayName: 'Notebook',
        category: 'Content',
        iconName: 'LuNotebookPen',
        description: 'Notebook of markdown pages and notes',
        createOpId: 'notes/notebook/init-staged',
        defaultNamePattern: 'Notebook',
        keyPrefix: 'notebook/',
        persistent: true,
        wizardTypeId: 'wizard/notes/notebook',
      },
      {
        typeId: 'notes/docs',
        pluginId: 'spacewave-notes',
        displayName: 'Documentation',
        category: 'Content',
        iconName: 'LuBookOpen',
        description: 'Structured documentation site with sections',
        createOpId: 'notes/docs/create-staged',
        defaultNamePattern: 'Documentation',
        keyPrefix: 'docs/',
        persistent: true,
        wizardTypeId: 'wizard/notes/docs',
      },
      {
        typeId: 'notes/blog',
        pluginId: 'spacewave-notes',
        displayName: 'Blog',
        category: 'Content',
        iconName: 'LuPenLine',
        description: 'Blog of dated posts with an index page',
        createOpId: 'notes/blog/create-staged',
        defaultNamePattern: 'Blog',
        keyPrefix: 'blog/',
        persistent: true,
        wizardTypeId: 'wizard/notes/blog',
      },
    ])
    expect(h.viewerRegistrations).toEqual([
      {
        typeId: 'notes/notebook',
        viewerName: 'Notebook',
        componentId: 'notes.notebook.viewer',
        scriptPath: `/asset/spacewave-notes/manifest/${manifestRoot}/v/b/fe/assets/notebook.mjs`,
        surface: ViewerSurface.WEB,
      },
      {
        typeId: 'notes/blog',
        viewerName: 'Blog',
        componentId: 'notes.blog.viewer',
        scriptPath: `/asset/spacewave-notes/manifest/${manifestRoot}/v/b/fe/assets/blog.mjs`,
        surface: ViewerSurface.WEB,
      },
      {
        typeId: 'notes/docs',
        viewerName: 'Documentation',
        componentId: 'notes.docs.viewer',
        scriptPath: `/asset/spacewave-notes/manifest/${manifestRoot}/v/b/fe/assets/docs.mjs`,
        surface: ViewerSurface.WEB,
      },
      {
        typeId: 'wizard/notes/notebook',
        viewerName: 'Notebook Wizard',
        componentId: 'notes.notebook-wizard.viewer',
        scriptPath: `/asset/spacewave-notes/manifest/${manifestRoot}/v/b/fe/assets/notes-wizard.mjs`,
        surface: ViewerSurface.WEB,
      },
      {
        typeId: 'wizard/notes/docs',
        viewerName: 'Documentation Wizard',
        componentId: 'notes.docs-wizard.viewer',
        scriptPath: `/asset/spacewave-notes/manifest/${manifestRoot}/v/b/fe/assets/notes-wizard.mjs`,
        surface: ViewerSurface.WEB,
      },
      {
        typeId: 'wizard/notes/blog',
        viewerName: 'Blog Wizard',
        componentId: 'notes.blog-wizard.viewer',
        scriptPath: `/asset/spacewave-notes/manifest/${manifestRoot}/v/b/fe/assets/notes-wizard.mjs`,
        surface: ViewerSurface.WEB,
      },
    ])
    expect(h.prepareRequests).toEqual([
      { pluginId: 'spacewave-notes', manifestRoot, instanceKey: undefined },
    ])
    expect(h.activatedScopes).toEqual([h.scopeRef])
    expect(h.rootRef.createRef).not.toHaveBeenCalled()
    expect(h.scopeRef.createRef).toHaveBeenCalledTimes(25)
    expect(h.retainedRefs.map((entry) => entry.resourceId)).toEqual(
      Array.from({ length: 25 }, (_, i) => i + 1),
    )
    for (const entry of h.retainedRefs) {
      expect(entry.ref[Symbol.dispose]).not.toHaveBeenCalled()
    }
    expect(h.scopeRef[Symbol.dispose]).not.toHaveBeenCalled()
    expect(h.rootRef[Symbol.dispose]).not.toHaveBeenCalled()

    abort.abort()

    for (const entry of h.retainedRefs) {
      expect(entry.ref[Symbol.dispose]).toHaveBeenCalledTimes(1)
    }
    expect(h.scopeRef[Symbol.dispose]).toHaveBeenCalledTimes(1)
    expect(h.rootRef[Symbol.dispose]).toHaveBeenCalledTimes(1)
  })

  it('leaves a prepared generation to the scheduler Activation', async () => {
    const abort = new AbortController()

    await startMain(
      buildApi('spacewave-notes', {
        prepared: true,
        instanceKey: 'space-engine',
      }) as never,
      abort.signal,
    )

    expect(h.prepareRequests).toEqual([
      {
        pluginId: 'spacewave-notes',
        manifestRoot,
        instanceKey: 'space-engine',
      },
    ])
    expect(h.activatedScopes).toHaveLength(0)
    await expect(h.activationReady?.()).resolves.toBe(h.scopeRef)

    abort.abort()
  })

  it('registers nothing from a historical worker', async () => {
    const abort = new AbortController()

    await startMain(
      buildApi('spacewave-notes', { historical: true }) as never,
      abort.signal,
    )

    expect(h.prepareRequests).toHaveLength(0)
    expect(h.objectTypeRegistrations).toHaveLength(0)
    expect(h.quickstartRegistrations).toHaveLength(0)
    await expect(h.activationReady?.()).resolves.toBeUndefined()

    abort.abort()
  })

  it('routes incoming plugin RPC streams to the notes ResourceServer', async () => {
    const abort = new AbortController()

    await startMain(buildApi('spacewave-notes') as never, abort.signal)

    expect(h.handleStreamSet).toHaveBeenCalledTimes(1)
    const pluginHandler = h.createdHandlers.find(
      (entry) => entry.definition === h.pluginDefinition,
    )?.handler as {
      PluginRpc(request: unknown, signal?: AbortSignal): unknown
    }
    expect(pluginHandler).toBeDefined()

    const iterator = {} as AsyncIterator<unknown>
    const request = {
      [Symbol.asyncIterator]: vi.fn(() => iterator),
    }
    const rpcResponse = pluginHandler.PluginRpc(request, abort.signal)

    expect(request[Symbol.asyncIterator]).toHaveBeenCalledTimes(1)
    expect(h.handleRpcStream).toHaveBeenCalledTimes(1)
    expect(h.handleRpcStreamCalls).toHaveLength(1)
    expect(h.handleRpcStreamCalls[0].iterator).toBe(iterator)
    await expect(
      h.handleRpcStreamCalls[0].getter('spacewave-core'),
    ).resolves.toBe(h.serverInstances[0].rpcStreamHandler)
    expect(rpcResponse).toBeDefined()

    const streamHandler: unknown = h.handleStreamSet.mock.calls[0]?.[0]
    const channel = {}
    expect(streamHandler).toBeTypeOf('function')
    await (streamHandler as (channel: unknown) => Promise<void>)(channel)

    expect(h.serverInstances[1].handlePacketStream).toHaveBeenCalledWith(
      channel,
    )

    abort.abort()
  })

  it('completes startup only after every registration', async () => {
    const abort = new AbortController()
    let resolveRoot!: () => void
    h.accessRootResource = () =>
      new Promise((resolve) => {
        resolveRoot = () => resolve(h.rootRef)
      })
    const lifecycle = main(buildApi('spacewave-notes') as never, abort.signal)
    let started = false
    const startup = Promise.resolve(lifecycle.startup).then(() => {
      started = true
    })
    let doneResolved = false
    const done = Promise.resolve(lifecycle.done).then(() => {
      doneResolved = true
    })

    await vi.waitFor(() => expect(resolveRoot).toBeDefined())
    expect(h.handleStreamSet).toHaveBeenCalledTimes(1)
    expect(started).toBe(false)

    resolveRoot()
    await startup
    expect(h.quickstartRegistrations).toHaveLength(3)
    expect(h.activatedScopes).toEqual([h.scopeRef])
    expect(doneResolved).toBe(false)

    abort.abort()
    await done
    expect(doneResolved).toBe(true)
  })

  it('fails startup and publishes nothing when a viewer fails to register', async () => {
    const abort = new AbortController()
    h.viewerRegistrationFailure = new Error('viewer registry unavailable')

    const lifecycle = main(buildApi('spacewave-notes') as never, abort.signal)
    await expect(lifecycle.startup).rejects.toThrow(
      'viewer registry unavailable',
    )
    await expect(lifecycle.done).rejects.toThrow('viewer registry unavailable')

    expect(h.quickstartRegistrations).toHaveLength(0)
    expect(h.activatedScopes).toHaveLength(0)
    expect(
      h.wizardRegistrations.map((registration) => registration.typeId),
    ).toEqual(['notes/notebook', 'notes/docs', 'notes/blog'])
    expect(h.viewerRegistrations).toHaveLength(1)
    expect(h.scopeRef.createRef).toHaveBeenCalledTimes(16)
    expect(h.retainedRefs.map((entry) => entry.resourceId)).toEqual([
      1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
    ])
    for (const entry of h.retainedRefs) {
      expect(entry.ref[Symbol.dispose]).toHaveBeenCalledTimes(1)
    }
    expect(h.rootRef[Symbol.dispose]).toHaveBeenCalledTimes(1)

    abort.abort()

    for (const entry of h.retainedRefs) {
      expect(entry.ref[Symbol.dispose]).toHaveBeenCalledTimes(1)
    }
    expect(h.rootRef[Symbol.dispose]).toHaveBeenCalledTimes(1)
  })

  it('releases partial registrations when a quickstart fails to register', async () => {
    const abort = new AbortController()
    h.quickstartRegistrationFailureId = 'blog'

    const lifecycle = main(buildApi('spacewave-notes') as never, abort.signal)
    await expect(lifecycle.startup).rejects.toThrow(
      'quickstart registry unavailable',
    )
    await expect(lifecycle.done).rejects.toThrow(
      'quickstart registry unavailable',
    )

    expect(
      h.quickstartRegistrations.map(
        (registration) => registration.quickstartId,
      ),
    ).toEqual(['notebook', 'docs', 'blog'])
    expect(h.activatedScopes).toHaveLength(0)
    expect(h.viewerRegistrations).toHaveLength(6)
    expect(h.scopeRef.createRef).toHaveBeenCalledTimes(24)
    expect(h.retainedRefs.map((entry) => entry.resourceId)).toEqual([
      1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21,
      22, 23, 24,
    ])
    for (const entry of h.retainedRefs) {
      expect(entry.ref[Symbol.dispose]).toHaveBeenCalledTimes(1)
    }
    expect(h.rootRef[Symbol.dispose]).toHaveBeenCalledTimes(1)

    abort.abort()

    for (const entry of h.retainedRefs) {
      expect(entry.ref[Symbol.dispose]).toHaveBeenCalledTimes(1)
    }
    expect(h.rootRef[Symbol.dispose]).toHaveBeenCalledTimes(1)
  })

  it('requires a backend startInfo plugin id', async () => {
    const abort = new AbortController()
    const lifecycle = main(buildApi('') as never, abort.signal)
    await expect(lifecycle.startup).rejects.toThrow(
      'missing plugin id in backend start info',
    )
    abort.abort()
    await lifecycle.done
    expect(h.objectTypeRegistrations).toHaveLength(0)
    expect(h.worldOpRegistrations).toHaveLength(0)
    expect(h.quickstartRegistrations).toHaveLength(0)
    expect(h.wizardRegistrations).toHaveLength(0)
    expect(h.viewerRegistrations).toHaveLength(0)
  })
})

describe('notes quickstart seed', () => {
  type SeedHandler = {
    SeedQuickstart(
      request: { quickstartId: string; attachedEngineResourceId: number },
      abortSignal: AbortSignal,
      context: unknown,
    ): Promise<{ indexPath?: string; pluginIds?: string[] }>
  }

  async function startSeedHandler(): Promise<SeedHandler> {
    h.createdHandlers.length = 0
    h.rootRef = buildRef()
    h.scopeRef = buildRef()
    await startMain(
      buildApi('spacewave-notes') as never,
      new AbortController().signal,
    )
    const created = h.createdHandlers.find(
      (c) =>
        typeof (c.handler as Partial<SeedHandler>).SeedQuickstart ===
        'function',
    )
    if (!created) throw new Error('expected the quickstart handler')
    return created.handler as SeedHandler
  }

  function seed(handler: SeedHandler, quickstartId: string) {
    return handler.SeedQuickstart(
      { quickstartId, attachedEngineResourceId: 7 },
      new AbortController().signal,
      { signal: new AbortController().signal },
    )
  }

  // applyOp applies one op through the plugin's op handler as the host does,
  // writing nothing when the op fails or is rejected. It returns the
  // handler's rejection, if any.
  async function applyOp(
    opTypeId: string,
    opData: Uint8Array,
  ): Promise<{ rejectionCode?: string; rejectionMessage?: string }> {
    const created = h.createdHandlers.find(
      (c) => c.definition === WorldOpHandlerServiceDefinition,
    )
    const opHandler = created?.handler as {
      ApplyWorldOp(
        request: Record<string, unknown>,
        abortSignal: AbortSignal,
        context: unknown,
      ): Promise<{ rejectionCode?: string; rejectionMessage?: string }>
    }
    const before = new Map(h.world)
    const restore = () => {
      h.world.clear()
      for (const [key, typeId] of before) h.world.set(key, typeId)
    }
    const signal = new AbortController().signal
    let resp: { rejectionCode?: string; rejectionMessage?: string }
    try {
      resp = await opHandler.ApplyWorldOp(
        {
          operationTypeId: opTypeId,
          opData,
          attachedWorldStateResourceId: 1,
        },
        signal,
        { signal },
      )
    } catch (err) {
      restore()
      throw err
    }
    if (resp.rejectionCode) restore()
    return resp
  }

  // applyBatchesThroughOpHandler makes the host apply each seed batch's one
  // op with applyOp.
  function applyBatchesThroughOpHandler() {
    h.executeWorldBatch.mockImplementation(
      async (
        mutations: Array<{
          mutation: { value: { opTypeId: string; opData: Uint8Array } }
        }>,
      ) => {
        const op = mutations[0]?.mutation.value
        const resp = await applyOp(
          op?.opTypeId ?? '',
          op?.opData ?? new Uint8Array(),
        )
        if (resp.rejectionCode) return { ...resp, failedIndex: 0 }
        return { seqno: 1n }
      },
    )
  }

  beforeEach(() => {
    h.executeWorldBatch.mockReset()
    h.engineRelease.mockReset()
    h.stageRelease.mockReset()
    h.staged.length = 0
  })

  it('seeds each quickstart with its staged create op as one batch, holding the stage until the batch returns', async () => {
    h.executeWorldBatch.mockImplementation(() => {
      // The batch adopts the staged roots, so the stage is still held.
      expect(h.stageRelease).not.toHaveBeenCalled()
      return Promise.resolve({ seqno: 1n })
    })
    const handler = await startSeedHandler()

    const ops = new Map<string, Uint8Array>()
    for (const [id, opTypeId, indexPath] of [
      ['notebook', 'notes/notebook/init-staged', 'notebook'],
      ['docs', 'notes/docs/create-staged', 'documentation'],
      ['blog', 'notes/blog/create-staged', 'blog/site'],
    ]) {
      h.executeWorldBatch.mockClear()
      h.stageRelease.mockClear()
      await expect(seed(handler, id)).resolves.toEqual({
        indexPath,
        pluginIds: ['spacewave-notes'],
      })
      expect(h.executeWorldBatch).toHaveBeenCalledTimes(1)
      const [mutations] = h.executeWorldBatch.mock.calls[0] ?? []
      expect(mutations).toHaveLength(1)
      expect(mutations[0].mutation.case).toBe('op')
      expect(mutations[0].mutation.value.opTypeId).toBe(opTypeId)
      ops.set(id, mutations[0].mutation.value.opData)
      expect(h.stageRelease).toHaveBeenCalledTimes(1)
    }
    const notebookOp = InitNotebookStagedOp.fromBinary(
      ops.get('notebook') ?? new Uint8Array(),
    )
    expect(notebookOp.notebookObjectKey).toBe('notebook')
    expect(notebookOp.unixfsObjectKey).toBe('files')
    expect(notebookOp.notebookRoot?.rootRef).toBeDefined()
    expect(notebookOp.unixfsRoot?.rootRef).toBeDefined()
    // Each create stages its blocks and tree concurrently, so only the
    // set staged is fixed, not its order.
    expect(h.staged.toSorted()).toEqual([
      'block',
      'block',
      'block',
      'block',
      'tree hello-world.md',
      'tree index.md',
      'tree welcome.md,getting-started.md',
    ])
    expect(h.engineRelease).toHaveBeenCalledTimes(3)
  })

  it('reports a plugin op timeout and a batch refusal typed', async () => {
    const handler = await startSeedHandler()

    h.executeWorldBatch.mockResolvedValueOnce({
      errorCode: WorldErrorCode.PLUGIN_OP_TIMEOUT,
      failedIndex: 0,
    })
    await expect(seed(handler, 'notebook')).rejects.toBeInstanceOf(
      PluginOpTimeoutError,
    )

    h.executeWorldBatch.mockResolvedValueOnce({
      errorCode: WorldErrorCode.OBJECT_NOT_FOUND,
      failedIndex: 0,
    })
    const refusal = await seed(handler, 'blog').catch((err: unknown) => err)
    expect(refusal).toBeInstanceOf(WorldBatchRefusal)
    expect((refusal as WorldBatchRefusal).failedIndex).toBe(0)
    expect(h.engineRelease).toHaveBeenCalledTimes(2)
    expect(h.stageRelease).toHaveBeenCalledTimes(2)
  })

  it('seeds again over its own earlier seed, writing each object once, and rejects a key it cannot keep', async () => {
    const handler = await startSeedHandler()
    applyBatchesThroughOpHandler()

    for (const [id, indexPath, typeId, writes] of [
      [
        'notebook',
        'notebook',
        'notes/notebook',
        ['create files', 'type files', 'create notebook', 'type notebook'],
      ],
      [
        'docs',
        'documentation',
        'notes/docs',
        [
          'create documentation-fs',
          'type documentation-fs',
          'create documentation',
          'type documentation',
        ],
      ],
      [
        'blog',
        'blog/site',
        'notes/blog',
        [
          'create fs/blog/site',
          'type fs/blog/site',
          'create blog/site-notebook',
          'type blog/site-notebook',
          'create blog/site',
          'type blog/site',
        ],
      ],
    ] as const) {
      h.world.clear()
      h.seedWrites.length = 0
      const seeded = await seed(handler, id)
      expect(h.seedWrites).toEqual(writes)

      // The client's index write after the seed failed, so the Space has no
      // index and Retry runs the seed again over its own committed objects.
      await expect(seed(handler, id)).resolves.toEqual(seeded)
      expect(h.seedWrites).toEqual(writes)
      expect(seeded.indexPath).toBe(indexPath)
      expect(h.world.get(indexPath)).toBe(typeId)
    }

    // A seed keeps objects only when its primary object has its type. A
    // primary of another type or none, a kept primary missing a companion,
    // or a free primary whose companion key is taken, even by an object of
    // the right type such as a deleted notebook's files, rejects the whole
    // op before it writes anything, and the Space keeps only what it had.
    for (const [key, foundType, message] of [
      [
        'notebook',
        'notes/docs',
        'The object notebook already exists as a notes/docs, so a notes/notebook cannot be created there. Rename or delete that object, then try again.',
      ],
      [
        'notebook',
        '',
        'The object notebook already exists with no type, so a notes/notebook cannot be created there. Rename or delete that object, then try again.',
      ],
      [
        'notebook',
        'notes/notebook',
        'The notes/notebook notebook already exists, but its unixfs/fs-node files is missing. Rename or delete notebook, then try again.',
      ],
      [
        'files',
        'notes/notebook',
        'The object files was left by an earlier object, so the new notes/notebook notebook cannot use it. Delete or rename files, then try again.',
      ],
      [
        'files',
        'unixfs/fs-node',
        'The object files was left by an earlier object, so the new notes/notebook notebook cannot use it. Delete or rename files, then try again.',
      ],
    ]) {
      h.world.clear()
      h.seedWrites.length = 0
      h.world.set(key, foundType)
      const rejection = await seed(handler, 'notebook').catch(
        (err: unknown) => err,
      )
      expect(rejection).toBeInstanceOf(WorldOperationRejection)
      expect((rejection as WorldOperationRejection).code).toBe(
        SEED_KEY_TAKEN_CODE,
      )
      expect((rejection as WorldOperationRejection).message).toBe(message)
      expect([...h.world]).toEqual([[key, foundType]])
      expect(h.seedWrites).toEqual([])
    }
  })

  it('replays a stored blog op over an object left at its companion notebook key, and refuses every other taken companion key', async () => {
    const handler = await startSeedHandler()
    applyBatchesThroughOpHandler()
    const blogOp = CreateBlogOp.toBinary({
      objectKey: 'blog/site',
      name: 'Blog',
      timestamp: new Date(0),
    })

    // A blog op stored before applySeed decided seeds succeeded over any
    // object at its companion notebook key and kept it; its replay must too.
    for (const foundType of ['notes/notebook', 'notes/docs', '']) {
      h.world.clear()
      h.seedWrites.length = 0
      h.world.set('blog/site-notebook', foundType)
      await expect(applyOp(CREATE_BLOG_OP_ID, blogOp)).resolves.toEqual({})
      expect(h.seedWrites).toEqual([
        'init fs/blog/site',
        'upload fs/blog/site',
        'create blog/site',
        'type blog/site',
      ])
      expect([...h.world].sort()).toEqual([
        ['blog/site', 'notes/blog'],
        ['blog/site-notebook', foundType],
        ['fs/blog/site', 'unixfs/fs-node'],
      ])
    }

    // The staged blog op, the one seeds send now, keeps the full rule: a
    // taken companion notebook key rejects it.
    h.world.clear()
    h.seedWrites.length = 0
    h.world.set('blog/site-notebook', 'notes/notebook')
    const staged = await seed(handler, 'blog').catch((err: unknown) => err)
    expect((staged as WorldOperationRejection).code).toBe(SEED_KEY_TAKEN_CODE)
    expect([...h.world]).toEqual([['blog/site-notebook', 'notes/notebook']])
    expect(h.seedWrites).toEqual([])

    // The UnixFS root of every stored op, and the docs rules, are unchanged:
    // a free primary whose UnixFS key is taken rejects the op.
    for (const [opTypeId, opData, key, primary] of [
      [CREATE_BLOG_OP_ID, blogOp, 'fs/blog/site', 'notes/blog blog/site'],
      [
        CREATE_DOCS_OP_ID,
        CreateDocumentationOp.toBinary({
          objectKey: 'documentation',
          name: 'Docs',
          timestamp: new Date(0),
        }),
        'documentation-fs',
        'notes/docs documentation',
      ],
    ] as const) {
      h.world.clear()
      h.seedWrites.length = 0
      h.world.set(key, 'unixfs/fs-node')
      await expect(applyOp(opTypeId, opData)).resolves.toEqual({
        rejectionCode: SEED_KEY_TAKEN_CODE,
        rejectionMessage: `The object ${key} was left by an earlier object, so the new ${primary} cannot use it. Delete or rename ${key}, then try again.`,
      })
      expect([...h.world]).toEqual([[key, 'unixfs/fs-node']])
      expect(h.seedWrites).toEqual([])
    }
  })

  it('replays a stored op of each old id over the objects an earlier seed left, keeping them', async () => {
    await startSeedHandler()

    // Between the seed rule's introduction and the staged ids, an old-id op
    // whose primary was present with its type and every companion present
    // with its type succeeded and kept them all. Its replay through the
    // current handler must succeed the same way, writing nothing.
    for (const [opTypeId, opData, objects] of [
      [
        INIT_NOTEBOOK_OP_ID,
        InitNotebookOp.toBinary({
          notebookObjectKey: 'notebook',
          unixfsObjectKey: 'files',
          name: 'Notebook',
          timestamp: new Date(0),
        }),
        [
          ['files', 'unixfs/fs-node'],
          ['notebook', 'notes/notebook'],
        ],
      ],
      [
        CREATE_DOCS_OP_ID,
        CreateDocumentationOp.toBinary({
          objectKey: 'documentation',
          name: 'Docs',
          timestamp: new Date(0),
        }),
        [
          ['documentation', 'notes/docs'],
          ['documentation-fs', 'unixfs/fs-node'],
        ],
      ],
      [
        CREATE_BLOG_OP_ID,
        CreateBlogOp.toBinary({
          objectKey: 'blog/site',
          name: 'Blog',
          timestamp: new Date(0),
        }),
        [
          ['blog/site', 'notes/blog'],
          ['blog/site-notebook', 'notes/notebook'],
          ['fs/blog/site', 'unixfs/fs-node'],
        ],
      ],
    ] as const) {
      h.world.clear()
      h.seedWrites.length = 0
      for (const [key, typeId] of objects) h.world.set(key, typeId)
      await expect(applyOp(opTypeId, opData)).resolves.toEqual({})
      expect(h.seedWrites).toEqual([])
      expect([...h.world].sort()).toEqual(objects)
    }
  })

  it('refuses an unknown quickstart before touching the engine', async () => {
    const handler = await startSeedHandler()

    await expect(seed(handler, 'gallery')).rejects.toThrow(
      'unhandled notes quickstart: gallery',
    )
    expect(h.executeWorldBatch).not.toHaveBeenCalled()
  })
})
