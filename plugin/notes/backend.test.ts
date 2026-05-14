import { beforeEach, describe, expect, it, vi } from 'vitest'

const h = vi.hoisted(() => ({
  buildPluginOpenStream: vi.fn(),
  handleStreamSet: vi.fn(),
  pluginAssetHttpPath: vi.fn(),
  objectTypeRegistrations: [] as Array<Record<string, unknown>>,
  worldOpRegistrations: [] as Array<Record<string, unknown>>,
  quickstartRegistrations: [] as Array<Record<string, unknown>>,
  wizardRegistrations: [] as Array<Record<string, unknown>>,
  viewerRegistrations: [] as Array<Record<string, unknown>>,
  retainedRefs: [] as Array<{
    resourceId: number
    ref: { [Symbol.dispose](): void }
  }>,
  nextResourceId: 1,
  rootRef: undefined as unknown as {
    client: Record<string, never>
    createRef(resourceId: number): { [Symbol.dispose](): void }
    [Symbol.dispose](): void
  },
}))

vi.mock('starpc', () => ({
  createMux: vi.fn(() => ({ lookupMethod: vi.fn() })),
  createHandler: vi.fn((definition: unknown, handler: unknown) => ({
    definition,
    handler,
  })),
  Server: class {
    handlePacketStream = vi.fn()
  },
  Client: class {
    constructor(stream: unknown) {
      h.buildPluginOpenStream(stream)
    }
  },
}))

vi.mock('@aptre/bldr-sdk/resource/index.js', () => ({
  ResourceServiceClient: class {
    constructor(_client: unknown) {}
  },
  Client: class {
    constructor(_service: unknown, _signal: AbortSignal) {}

    accessRootResource() {
      return Promise.resolve(h.rootRef)
    }
  },
}))

vi.mock('@aptre/bldr-sdk/resource/server/index.js', () => ({
  ResourceServer: class {
    constructor(_mux: unknown) {}

    register(_mux: unknown) {}
  },
  constructChildResource: vi.fn((factory: () => unknown) => {
    factory()
    return { resourceId: 9001 }
  }),
  getCurrentResourceClient: vi.fn(() => ({
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

import main from './backend.js'

function buildApi(pluginId: string) {
  return {
    startInfo: { pluginId },
    client: {},
    handleStreamCtr: { set: h.handleStreamSet },
    buildPluginOpenStream: vi.fn((target: string) => target),
    utils: {
      pluginAssetHttpPath: h.pluginAssetHttpPath,
    },
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
    h.nextResourceId = 1
    h.pluginAssetHttpPath.mockImplementation(
      (pluginId: string, path: string) => `/asset/${pluginId}/${path}`,
    )
    h.rootRef = {
      client: {},
      createRef: vi.fn((resourceId: number) => {
        const ref = { [Symbol.dispose]: vi.fn() }
        h.retainedRefs.push({ resourceId, ref })
        return ref
      }),
      [Symbol.dispose]: vi.fn(),
    }
  })

  it('registers notes interfaces with the startInfo plugin id and retained lifetimes', async () => {
    const abort = new AbortController()

    await main(buildApi('spacewave-notes') as never, abort.signal)

    expect(h.objectTypeRegistrations).toEqual([
      { typeId: 'notes/notebook', pluginId: 'spacewave-notes' },
      { typeId: 'notes/blog', pluginId: 'spacewave-notes' },
      { typeId: 'notes/docs', pluginId: 'spacewave-notes' },
    ])
    expect(h.worldOpRegistrations).toEqual([
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
        createOpId: 'notes/notebook/init',
        defaultNamePattern: 'Notebook',
        keyPrefix: 'notebook/',
        persistent: true,
        wizardTypeId: 'wizard/notes/notebook',
        experimental: true,
      },
      {
        typeId: 'notes/docs',
        pluginId: 'spacewave-notes',
        displayName: 'Documentation',
        category: 'Content',
        iconName: 'LuBookOpen',
        createOpId: 'notes/docs/create',
        defaultNamePattern: 'Documentation',
        keyPrefix: 'docs/',
        persistent: true,
        wizardTypeId: 'wizard/notes/docs',
        experimental: true,
      },
      {
        typeId: 'notes/blog',
        pluginId: 'spacewave-notes',
        displayName: 'Blog',
        category: 'Content',
        iconName: 'LuPenLine',
        createOpId: 'notes/blog/create',
        defaultNamePattern: 'Blog',
        keyPrefix: 'blog/',
        persistent: true,
        wizardTypeId: 'wizard/notes/blog',
        experimental: true,
      },
    ])
    expect(h.viewerRegistrations).toEqual([
      {
        typeId: 'notes/notebook',
        viewerName: 'Notebook',
        scriptPath: '/asset/spacewave-notes/v/b/fe/assets/notebook.mjs',
      },
      {
        typeId: 'notes/blog',
        viewerName: 'Blog',
        scriptPath: '/asset/spacewave-notes/v/b/fe/assets/blog.mjs',
      },
      {
        typeId: 'notes/docs',
        viewerName: 'Documentation',
        scriptPath: '/asset/spacewave-notes/v/b/fe/assets/docs.mjs',
      },
      {
        typeId: 'wizard/notes/notebook',
        viewerName: 'Notebook Wizard',
        scriptPath: '/asset/spacewave-notes/v/b/fe/assets/notes-wizard.mjs',
      },
      {
        typeId: 'wizard/notes/docs',
        viewerName: 'Documentation Wizard',
        scriptPath: '/asset/spacewave-notes/v/b/fe/assets/notes-wizard.mjs',
      },
      {
        typeId: 'wizard/notes/blog',
        viewerName: 'Blog Wizard',
        scriptPath: '/asset/spacewave-notes/v/b/fe/assets/notes-wizard.mjs',
      },
    ])
    expect(h.rootRef.createRef).toHaveBeenCalledTimes(18)
    expect(h.retainedRefs.map((entry) => entry.resourceId)).toEqual([
      1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18,
    ])
    for (const entry of h.retainedRefs) {
      expect(entry.ref[Symbol.dispose]).not.toHaveBeenCalled()
    }
    expect(h.rootRef[Symbol.dispose]).not.toHaveBeenCalled()

    abort.abort()

    for (const entry of h.retainedRefs) {
      expect(entry.ref[Symbol.dispose]).toHaveBeenCalledTimes(1)
    }
    expect(h.rootRef[Symbol.dispose]).toHaveBeenCalledTimes(1)
  })

  it('requires a backend startInfo plugin id', async () => {
    await expect(
      main(buildApi('') as never, new AbortController().signal),
    ).rejects.toThrow('missing plugin id in backend start info')
    expect(h.objectTypeRegistrations).toHaveLength(0)
    expect(h.worldOpRegistrations).toHaveLength(0)
    expect(h.quickstartRegistrations).toHaveLength(0)
    expect(h.wizardRegistrations).toHaveLength(0)
    expect(h.viewerRegistrations).toHaveLength(0)
  })
})
