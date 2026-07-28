import {
  createMux,
  createHandler,
  Server,
  Client as SRPCClient,
  handleRpcStream,
  type MessageStream,
  type RpcStreamPacket,
} from 'starpc'
import type { BackendAPI, BackendEntrypointLifecycle } from '@aptre/bldr-sdk'
import {
  Client as ResourcesClient,
  ResourceServiceClient,
  type ClientResourceRef,
} from '@aptre/bldr-sdk/resource/index.js'
import {
  ResourceServer,
  constructChildResource,
  getCurrentResourceClient,
  newResourceMux,
} from '@aptre/bldr-sdk/resource/server/index.js'
import {
  ObjectTypeHandlerServiceDefinition,
  ObjectTypeRegistryResourceServiceClient,
} from '@s4wave/sdk/objecttype/registry/registry_srpc.pb.js'
import type {
  InvokeObjectTypeRequest,
  InvokeObjectTypeResponse,
} from '@s4wave/sdk/objecttype/registry/registry.pb.js'
import {
  WorldOpHandlerServiceDefinition,
  WorldOpRegistryResourceServiceClient,
} from '@s4wave/sdk/worldop/registry/registry_srpc.pb.js'
import {
  QuickstartHandlerServiceDefinition,
  QuickstartRegistryResourceServiceClient,
} from '@s4wave/sdk/quickstart/registry/registry_srpc.pb.js'
import { ObjectWizardRegistryResourceServiceClient } from '@s4wave/sdk/world/wizard/wizard_srpc.pb.js'
import type {
  ApplyWorldOpRequest,
  ApplyWorldOpResponse,
  ApplyWorldObjectOpRequest,
  ApplyWorldObjectOpResponse,
  ValidateOpRequest,
  ValidateOpResponse,
} from '@s4wave/sdk/worldop/registry/registry.pb.js'
import type {
  SeedQuickstartRequest,
  SeedQuickstartResponse,
} from '@s4wave/sdk/quickstart/registry/registry.pb.js'
import { ViewerRegistryResourceServiceClient } from '@s4wave/sdk/viewer/registry/registry_srpc.pb.js'
import { ViewerSurface } from '@s4wave/sdk/viewer/registry/registry.pb.js'
import { Engine } from '@s4wave/sdk/world/engine.js'
import { EngineWorldState } from '@s4wave/sdk/world/engine-state.js'
import { WorldStateResource } from '@s4wave/sdk/world/world-state.js'
import { setObjectType } from '@s4wave/sdk/world/types/types.js'
import {
  INIT_UNIXFS_OP_ID,
  UNIXFS_OBJECT_KEY,
} from '@s4wave/core/space/world/ops/init-unixfs.js'
import { InitUnixFSOp } from '@s4wave/core/space/world/ops/ops.pb.js'
import { FSCursorServiceClient } from '@go/github.com/s4wave/spacewave/db/unixfs/rpc/rpc_srpc.pb.js'
import { buildFSHandle } from '@go/github.com/s4wave/spacewave/db/unixfs/rpc/client/fs-handle.js'
import {
  PluginDefinition,
  type Plugin as SRPCPlugin,
} from '@go/github.com/s4wave/spacewave/bldr/plugin/plugin_srpc.pb.js'
import { NotebookResourceServiceDefinition } from './sdk/notebook_srpc.pb.js'
import { BlogResourceServiceDefinition } from './sdk/blog_srpc.pb.js'
import { DocsResourceServiceDefinition } from './sdk/docs_srpc.pb.js'
import { NotebookResource } from './notebook-resource.js'
import { BlogResource } from './blog-resource.js'
import { DocsResource } from './docs-resource.js'
import { createBlogClientSide } from './blog-seed.js'
import {
  createDocsClientSide,
  createNotebookClientSide,
  DOCS_INDEX_MARKDOWN,
  NOTEBOOK_GETTING_STARTED_MARKDOWN,
  NOTEBOOK_WELCOME_MARKDOWN,
} from './content-seed.js'
import { createObjectWithBlockData } from './object-block.js'
import {
  INIT_NOTEBOOK_OP_ID,
  NOTEBOOK_OBJECT_KEY,
} from './proto/init-notebook.js'
import { BLOG_OBJECT_KEY, CREATE_BLOG_OP_ID } from './proto/create-blog.js'
import { CREATE_DOCS_OP_ID } from './proto/create-docs.js'
import { InitNotebookOp, Notebook } from './proto/notebook.pb.js'
import { CreateBlogOp } from './proto/blog.pb.js'
import { CreateDocumentationOp, Documentation } from './proto/docs.pb.js'
import { uploadSeedTree } from './unixfs-seed.js'

type ViteManifestEntry = {
  file?: string
}

class NotesPlugin implements SRPCPlugin {
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

const DOCS_QUICKSTART_OBJECT_KEY = 'documentation'

function retainUntilAbort(
  signal: AbortSignal,
  refs: ClientResourceRef[],
  retained: unknown[],
): () => void {
  let released = false
  const release = () => {
    if (released) return
    released = true
    signal.removeEventListener('abort', release)
    for (let i = refs.length - 1; i >= 0; --i) {
      refs[i][Symbol.dispose]()
    }
    retained.length = 0
  }
  if (signal.aborted) {
    release()
    return release
  }
  signal.addEventListener('abort', release, { once: true })
  return release
}

function waitForAbort(signal: AbortSignal): Promise<void> {
  if (signal.aborted) return Promise.resolve()
  return new Promise((resolve) => {
    const onAbort = () => {
      signal.removeEventListener('abort', onAbort)
      resolve()
    }
    signal.addEventListener('abort', onAbort, { once: true })
  })
}

// resolveAssetPath resolves a source entrypoint path to its built output
// path by reading the Vite manifest from the plugin's assets FS.
async function resolveAssetPath(
  api: BackendAPI,
  signal: AbortSignal,
  srcPath: string,
): Promise<string> {
  const key = srcPath.replace(/^\.\//, '')
  const fsSvc = new FSCursorServiceClient(api.client, {
    service: 'plugin-assets/unixfs.rpc.FSCursorService',
  })
  using root = await buildFSHandle(fsSvc, signal)
  const { handle: manifest } = await root.lookupPath(
    signal,
    'v/b/fe/.vite/manifest.json',
  )
  using _ = manifest
  const size = await manifest.getSize(signal)
  const { data } = await manifest.readAt(signal, 0n, size)
  const parsed = JSON.parse(new TextDecoder().decode(data)) as Record<
    string,
    ViteManifestEntry
  >
  const entry = parsed[key]
  if (entry?.file) {
    const pluginId = api.startInfo.pluginId
    return api.utils.pluginAssetHttpPath(pluginId!, 'v/b/fe/' + entry.file)
  }
  return srcPath
}

// NotesObjectTypeHandler implements ObjectTypeHandlerService for all
// notes plugin object types (notebook, blog, docs). Dispatches on typeId to
// create the appropriate resource handler.
class NotesObjectTypeHandler {
  InvokeObjectType(
    request: InvokeObjectTypeRequest,
    _abortSignal?: AbortSignal,
  ): Promise<InvokeObjectTypeResponse> {
    const typeId = request.typeId ?? ''
    const engineId = request.attachedEngineResourceId ?? 0
    const objectKey = request.objectKey ?? ''
    const { resourceId } = constructChildResource(() => {
      // If engineId is provided, get an attached ref to the world engine.
      // The engine ref wraps the yamux-backed srpc.Client for RPCs
      // back to the Go bridge's EngineResource mux.
      const engineRef =
        engineId > 0
          ? getCurrentResourceClient().getAttachedRef(engineId)
          : undefined

      switch (typeId) {
        case 'notes/blog': {
          const resource = new BlogResource(objectKey, engineRef)
          return {
            mux: newResourceMux(
              createHandler(BlogResourceServiceDefinition, resource),
            ),
            result: undefined,
            releaseFn: () => {
              resource.dispose()
            },
          }
        }
        case 'notes/docs': {
          const resource = new DocsResource(objectKey, engineRef)
          return {
            mux: newResourceMux(
              createHandler(DocsResourceServiceDefinition, resource),
            ),
            result: undefined,
            releaseFn: () => {
              resource.dispose()
            },
          }
        }
        case 'notes/notebook': {
          const resource = new NotebookResource(objectKey, engineRef)
          return {
            mux: newResourceMux(
              createHandler(NotebookResourceServiceDefinition, resource),
            ),
            result: undefined,
            releaseFn: () => {
              resource.dispose()
            },
          }
        }
        default:
          engineRef?.release()
          throw new Error('unhandled notes object type: ' + typeId)
      }
    })
    return Promise.resolve({ resourceId })
  }
}

// NotesWorldOpHandler implements WorldOpHandlerService for the notes plugin.
// Handles world-level and object-level operations for notes types.
class NotesWorldOpHandler {
  async ApplyWorldOp(
    request: ApplyWorldOpRequest,
    _abortSignal?: AbortSignal,
  ): Promise<ApplyWorldOpResponse> {
    const opTypeId = request.operationTypeId ?? ''
    switch (opTypeId) {
      case INIT_NOTEBOOK_OP_ID:
        return this.applyInitNotebook(request)
      case CREATE_BLOG_OP_ID:
        return this.applyCreateBlog(request)
      case CREATE_DOCS_OP_ID:
        return this.applyCreateDocs(request)
      default:
        throw new Error('unhandled world op: ' + opTypeId)
    }
  }

  ApplyWorldObjectOp(
    _request: ApplyWorldObjectOpRequest,
    _abortSignal?: AbortSignal,
  ): Promise<ApplyWorldObjectOpResponse> {
    return Promise.reject(new Error('unhandled object op'))
  }

  ValidateOp(
    _request: ValidateOpRequest,
    _abortSignal?: AbortSignal,
  ): Promise<ValidateOpResponse> {
    return Promise.resolve({})
  }

  // applyInitNotebook handles the init-notebook operation.
  // Creates a UnixFS object with sample files and a Notebook world object.
  private async applyInitNotebook(
    request: ApplyWorldOpRequest,
  ): Promise<ApplyWorldOpResponse> {
    const engineId = request.attachedWorldStateResourceId ?? 0
    if (!engineId) {
      throw new Error('attachedWorldStateResourceId is required')
    }

    // Deserialize the operation data.
    const op = InitNotebookOp.fromBinary(request.opData ?? new Uint8Array())
    const notebookKey = op.notebookObjectKey ?? ''
    const unixfsKey = op.unixfsObjectKey ?? ''
    if (!notebookKey || !unixfsKey) {
      throw new Error('notebookObjectKey and unixfsObjectKey are required')
    }

    // Get the WorldState from the attached resource.
    const wsRef = getCurrentResourceClient().getAttachedRef(engineId)
    const ws = new WorldStateResource(wsRef)
    try {
      // 1. Init UnixFS object via world op.
      const fsInitOp: InitUnixFSOp = {
        objectKey: unixfsKey,
        timestamp: op.timestamp,
      }
      await ws.applyWorldOp(
        INIT_UNIXFS_OP_ID,
        InitUnixFSOp.toBinary(fsInitOp),
        '',
      )

      // 2. Create sample note files via batch tree upload.
      await uploadSeedTree(
        ws,
        unixfsKey,
        [
          { path: 'welcome.md', content: NOTEBOOK_WELCOME_MARKDOWN },
          {
            path: 'getting-started.md',
            content: NOTEBOOK_GETTING_STARTED_MARKDOWN,
          },
        ],
        undefined,
      )

      // 3. Create Notebook world object with block data.
      const notebook: Notebook = {
        name: 'Notes',
        sources: [{ name: 'My Notes', ref: unixfsKey + '/-/' }],
      }
      await createObjectWithBlockData(
        ws,
        notebookKey,
        Notebook.toBinary(notebook),
      )

      // 4. Set the object type graph quad.
      await setObjectType(ws, notebookKey, 'notes/notebook')

      return {}
    } finally {
      ws.release()
      wsRef.release()
    }
  }

  // applyCreateBlog handles the create-blog operation.
  // Creates a Blog world object with inline source and a UnixFS object with an
  // initial post file.
  private async applyCreateBlog(
    request: ApplyWorldOpRequest,
  ): Promise<ApplyWorldOpResponse> {
    const engineId = request.attachedWorldStateResourceId ?? 0
    if (!engineId) {
      throw new Error('attachedWorldStateResourceId is required')
    }

    // Deserialize the operation data.
    const op = CreateBlogOp.fromBinary(request.opData ?? new Uint8Array())
    const blogKey = op.objectKey ?? ''
    if (!blogKey) {
      throw new Error('objectKey is required')
    }

    // Get the WorldState from the attached resource.
    const wsRef = getCurrentResourceClient().getAttachedRef(engineId)
    const ws = new WorldStateResource(wsRef)
    try {
      const blogName = op.name || 'Blog'
      await createBlogClientSide(
        ws,
        blogKey,
        blogName,
        op.description ?? '',
        op.authorRegistryPath ?? '',
        op.timestamp ?? new Date(),
      )

      return {}
    } finally {
      ws.release()
      wsRef.release()
    }
  }

  // applyCreateDocs handles the create-docs operation.
  // Creates a Documentation world object with inline source, a UnixFS object
  // with an initial index.md page, and a companion Notebook.
  private async applyCreateDocs(
    request: ApplyWorldOpRequest,
  ): Promise<ApplyWorldOpResponse> {
    const engineId = request.attachedWorldStateResourceId ?? 0
    if (!engineId) {
      throw new Error('attachedWorldStateResourceId is required')
    }

    // Deserialize the operation data.
    const op = CreateDocumentationOp.fromBinary(
      request.opData ?? new Uint8Array(),
    )
    const docsKey = op.objectKey ?? ''
    if (!docsKey) {
      throw new Error('objectKey is required')
    }

    // Derive the UnixFS key from the docs key.
    const unixfsKey = docsKey + '-fs'

    // Get the WorldState from the attached resource.
    const wsRef = getCurrentResourceClient().getAttachedRef(engineId)
    const ws = new WorldStateResource(wsRef)
    try {
      // 1. Init UnixFS object via world op.
      const fsInitOp: InitUnixFSOp = {
        objectKey: unixfsKey,
        timestamp: op.timestamp,
      }
      await ws.applyWorldOp(
        INIT_UNIXFS_OP_ID,
        InitUnixFSOp.toBinary(fsInitOp),
        '',
      )

      // 2. Create the initial docs tree via batch upload.
      await uploadSeedTree(
        ws,
        unixfsKey,
        [{ path: 'index.md', content: DOCS_INDEX_MARKDOWN }],
        undefined,
      )

      // 3. Create Documentation world object with block data.
      const docsName = op.name || 'Documentation'
      const documentation: Documentation = {
        name: docsName,
        description: op.description,
        sources: [{ name: 'Pages', ref: unixfsKey + '/-/' }],
        createdAt: op.timestamp,
      }
      await createObjectWithBlockData(
        ws,
        docsKey,
        Documentation.toBinary(documentation),
      )

      // 4. Set the docs object type graph quad.
      await setObjectType(ws, docsKey, 'notes/docs')

      return {}
    } finally {
      ws.release()
      wsRef.release()
    }
  }
}

class NotesQuickstartHandler {
  private readonly pluginId: string

  constructor(pluginId: string) {
    this.pluginId = pluginId
  }

  private async runQuickstartStep<T>(
    label: string,
    cb: () => Promise<T>,
  ): Promise<T> {
    try {
      return await cb()
    } catch (err) {
      throw new Error(
        label + ': ' + (err instanceof Error ? err.message : String(err)),
        { cause: err },
      )
    }
  }

  async SeedQuickstart(
    request: SeedQuickstartRequest,
    abortSignal?: AbortSignal,
  ): Promise<SeedQuickstartResponse> {
    const engineId = request.attachedEngineResourceId ?? 0
    if (!engineId) {
      throw new Error('attachedEngineResourceId is required')
    }

    const engineRef = getCurrentResourceClient().getAttachedRef(engineId)
    const engine = new Engine(engineRef)
    const worldState = new EngineWorldState(engine, true)
    try {
      switch (request.quickstartId ?? '') {
        case 'notebook':
          await this.runQuickstartStep('seed notebook quickstart', async () => {
            await createNotebookClientSide(
              worldState,
              NOTEBOOK_OBJECT_KEY,
              UNIXFS_OBJECT_KEY,
              'Notes',
              new Date(),
              abortSignal,
            )
          })
          return {
            indexPath: NOTEBOOK_OBJECT_KEY,
            pluginIds: [this.pluginId],
          }
        case 'docs':
          await this.runQuickstartStep('seed docs quickstart', async () => {
            await createDocsClientSide(
              worldState,
              DOCS_QUICKSTART_OBJECT_KEY,
              'Documentation',
              '',
              new Date(),
              abortSignal,
            )
          })
          return {
            indexPath: DOCS_QUICKSTART_OBJECT_KEY,
            pluginIds: [this.pluginId],
          }
        case 'blog':
          await this.runQuickstartStep('seed blog quickstart', async () => {
            await createBlogClientSide(
              worldState,
              BLOG_OBJECT_KEY,
              'Blog',
              '',
              '',
              new Date(),
              abortSignal,
            )
          })
          return {
            indexPath: BLOG_OBJECT_KEY,
            pluginIds: [this.pluginId],
          }
        default:
          throw new Error('unhandled notes quickstart: ' + request.quickstartId)
      }
    } finally {
      engine.release()
    }
  }
}

// startNotesBackend starts the notes backend generation.
export function startNotesBackend(
  api: BackendAPI,
  signal: AbortSignal,
): BackendEntrypointLifecycle {
  const pluginId = api.startInfo.pluginId
  if (!pluginId) {
    return {
      startup: Promise.reject(
        new Error('missing plugin id in backend start info'),
      ),
      done: waitForAbort(signal),
    }
  }

  // Build root mux with ObjectTypeHandler and WorldOpHandler services.
  const otHandler = new NotesObjectTypeHandler()
  const opHandler = new NotesWorldOpHandler()
  const quickstartHandler = new NotesQuickstartHandler(pluginId)
  const rootMux = newResourceMux(
    createHandler(ObjectTypeHandlerServiceDefinition, otHandler),
    createHandler(WorldOpHandlerServiceDefinition, opHandler),
    createHandler(QuickstartHandlerServiceDefinition, quickstartHandler),
  )

  // Create ResourceServer with the root mux.
  const resourceServer = new ResourceServer(rootMux)
  const outerMux = createMux()
  resourceServer.register(outerMux)

  const resourceRpcServer = new Server(outerMux.lookupMethod)
  const plugin = new NotesPlugin(resourceRpcServer)
  const pluginMux = createMux()
  pluginMux.register(createHandler(PluginDefinition, plugin))
  const pluginServer = new Server(pluginMux.lookupMethod)
  api.handleStreamCtr.set((channel) => {
    pluginServer.handlePacketStream(channel)
    return Promise.resolve()
  })

  // Connect to spacewave-core via plugin open stream.
  const coreClient = new SRPCClient(api.buildPluginOpenStream('spacewave-core'))
  const resourcesService = new ResourceServiceClient(coreClient)
  const resourcesClient = new ResourcesClient(resourcesService, signal)
  const retained: unknown[] = [
    coreClient,
    resourcesClient,
    otHandler,
    opHandler,
    quickstartHandler,
    rootMux,
    resourceServer,
    outerMux,
    resourceRpcServer,
    plugin,
    pluginMux,
    pluginServer,
  ]
  // Startup only needs the RPC handler registered. Root-resource publication can
  // wait on host resources, so it stays on the long-lived backend lifecycle.
  const done = (async () => {
    const refs: ClientResourceRef[] = []
    const releaseRetained = retainUntilAbort(signal, refs, retained)
    try {
      const rootRef = await resourcesClient.accessRootResource()
      refs.push(rootRef)
      const retainRegistration = (
        resourceId: number | undefined,
        label: string,
      ) => {
        if (!resourceId) {
          throw new Error(label + ' registration did not return a resource id')
        }
        return rootRef.createRef(resourceId)
      }

      // Register ObjectTypes.
      const otSvc = new ObjectTypeRegistryResourceServiceClient(rootRef.client)
      const notebookType = await otSvc.RegisterObjectType(
        { typeId: 'notes/notebook', pluginId },
        signal,
      )
      refs.push(
        retainRegistration(notebookType.resourceId, 'notebook object type'),
      )
      const blogType = await otSvc.RegisterObjectType(
        { typeId: 'notes/blog', pluginId },
        signal,
      )
      refs.push(retainRegistration(blogType.resourceId, 'blog object type'))
      const docsType = await otSvc.RegisterObjectType(
        { typeId: 'notes/docs', pluginId },
        signal,
      )
      refs.push(retainRegistration(docsType.resourceId, 'docs object type'))

      // Register WorldOps.
      const woSvc = new WorldOpRegistryResourceServiceClient(rootRef.client)
      const initNotebookOp = await woSvc.RegisterWorldOp(
        { operationTypeId: INIT_NOTEBOOK_OP_ID, pluginId },
        signal,
      )
      refs.push(
        retainRegistration(initNotebookOp.resourceId, 'init notebook world op'),
      )
      const createBlogOp = await woSvc.RegisterWorldOp(
        { operationTypeId: CREATE_BLOG_OP_ID, pluginId },
        signal,
      )
      refs.push(
        retainRegistration(createBlogOp.resourceId, 'create blog world op'),
      )
      const createDocsOp = await woSvc.RegisterWorldOp(
        { operationTypeId: CREATE_DOCS_OP_ID, pluginId },
        signal,
      )
      refs.push(
        retainRegistration(createDocsOp.resourceId, 'create docs world op'),
      )

      // Register persistent ObjectWizards for in-space Notes creation.
      const wizardSvc = new ObjectWizardRegistryResourceServiceClient(
        rootRef.client,
      )
      const notebookWizard = await wizardSvc.RegisterWizard(
        {
          wizard: {
            typeId: 'notes/notebook',
            pluginId,
            displayName: 'Notebook',
            description: 'Notebook of markdown pages and notes',
            category: 'Content',
            iconName: 'LuNotebookPen',
            createOpId: INIT_NOTEBOOK_OP_ID,
            defaultNamePattern: 'Notebook',
            keyPrefix: 'notebook/',
            persistent: true,
            wizardTypeId: 'wizard/notes/notebook',
          },
        },
        signal,
      )
      refs.push(
        retainRegistration(notebookWizard.resourceId, 'notebook wizard'),
      )
      const docsWizard = await wizardSvc.RegisterWizard(
        {
          wizard: {
            typeId: 'notes/docs',
            pluginId,
            displayName: 'Documentation',
            description: 'Structured documentation site with sections',
            category: 'Content',
            iconName: 'LuBookOpen',
            createOpId: CREATE_DOCS_OP_ID,
            defaultNamePattern: 'Documentation',
            keyPrefix: 'docs/',
            persistent: true,
            wizardTypeId: 'wizard/notes/docs',
          },
        },
        signal,
      )
      refs.push(retainRegistration(docsWizard.resourceId, 'docs wizard'))
      const blogWizard = await wizardSvc.RegisterWizard(
        {
          wizard: {
            typeId: 'notes/blog',
            pluginId,
            displayName: 'Blog',
            description: 'Blog of dated posts with an index page',
            category: 'Content',
            iconName: 'LuPenLine',
            createOpId: CREATE_BLOG_OP_ID,
            defaultNamePattern: 'Blog',
            keyPrefix: 'blog/',
            persistent: true,
            wizardTypeId: 'wizard/notes/blog',
          },
        },
        signal,
      )
      refs.push(retainRegistration(blogWizard.resourceId, 'blog wizard'))

      // Resolve viewer script paths from the Vite manifest so the
      // frontend gets the hashed output paths (not the source paths).
      const [
        notebookViewerScript,
        blogViewerScript,
        docsViewerScript,
        notesWizardViewerScript,
      ] = await Promise.all([
        resolveAssetPath(api, signal, './plugin/notes/NotebookViewer.tsx'),
        resolveAssetPath(api, signal, './plugin/notes/BlogViewer.tsx'),
        resolveAssetPath(api, signal, './plugin/notes/DocsViewer.tsx'),
        resolveAssetPath(api, signal, './plugin/notes/NotesWizardViewer.tsx'),
      ])

      // Register Viewers.
      const vrSvc = new ViewerRegistryResourceServiceClient(rootRef.client)
      const notebookViewer = await vrSvc.RegisterViewer(
        {
          registration: {
            typeId: 'notes/notebook',
            viewerName: 'Notebook',
            componentId: 'notes.notebook.viewer',
            scriptPath: notebookViewerScript,
            surface: ViewerSurface.WEB,
          },
        },
        signal,
      )
      refs.push(
        retainRegistration(notebookViewer.resourceId, 'notebook viewer'),
      )
      const blogViewer = await vrSvc.RegisterViewer(
        {
          registration: {
            typeId: 'notes/blog',
            viewerName: 'Blog',
            componentId: 'notes.blog.viewer',
            scriptPath: blogViewerScript,
            surface: ViewerSurface.WEB,
          },
        },
        signal,
      )
      refs.push(retainRegistration(blogViewer.resourceId, 'blog viewer'))
      const docsViewer = await vrSvc.RegisterViewer(
        {
          registration: {
            typeId: 'notes/docs',
            viewerName: 'Documentation',
            componentId: 'notes.docs.viewer',
            scriptPath: docsViewerScript,
            surface: ViewerSurface.WEB,
          },
        },
        signal,
      )
      refs.push(retainRegistration(docsViewer.resourceId, 'docs viewer'))
      const notebookWizardViewer = await vrSvc.RegisterViewer(
        {
          registration: {
            typeId: 'wizard/notes/notebook',
            viewerName: 'Notebook Wizard',
            componentId: 'notes.notebook-wizard.viewer',
            scriptPath: notesWizardViewerScript,
            surface: ViewerSurface.WEB,
          },
        },
        signal,
      )
      refs.push(
        retainRegistration(
          notebookWizardViewer.resourceId,
          'notebook wizard viewer',
        ),
      )
      const docsWizardViewer = await vrSvc.RegisterViewer(
        {
          registration: {
            typeId: 'wizard/notes/docs',
            viewerName: 'Documentation Wizard',
            componentId: 'notes.docs-wizard.viewer',
            scriptPath: notesWizardViewerScript,
            surface: ViewerSurface.WEB,
          },
        },
        signal,
      )
      refs.push(
        retainRegistration(docsWizardViewer.resourceId, 'docs wizard viewer'),
      )
      const blogWizardViewer = await vrSvc.RegisterViewer(
        {
          registration: {
            typeId: 'wizard/notes/blog',
            viewerName: 'Blog Wizard',
            componentId: 'notes.blog-wizard.viewer',
            scriptPath: notesWizardViewerScript,
            surface: ViewerSurface.WEB,
          },
        },
        signal,
      )
      refs.push(
        retainRegistration(blogWizardViewer.resourceId, 'blog wizard viewer'),
      )

      // Register hidden Quickstarts last so app launchers only observe them once
      // the notes backend generation has completed startup registration.
      const qsSvc = new QuickstartRegistryResourceServiceClient(rootRef.client)
      const notebookQuickstart = await qsSvc.RegisterQuickstart(
        {
          registration: {
            quickstartId: 'notebook',
            pluginId,
            name: 'Create a Notebook',
            description: 'Markdown notes with folders, tags, and sync',
            category: 'storage',
            iconName: 'notebook',
            hidden: true,
            experimental: true,
            spaceName: 'My Notebook',
            requiredPluginIds: [pluginId],
          },
        },
        signal,
      )
      refs.push(
        retainRegistration(
          notebookQuickstart.resourceId,
          'notebook quickstart',
        ),
      )
      const docsQuickstart = await qsSvc.RegisterQuickstart(
        {
          registration: {
            quickstartId: 'docs',
            pluginId,
            name: 'Create Documentation',
            description: 'Markdown documentation site',
            category: 'content',
            iconName: 'notebook',
            hidden: true,
            experimental: true,
            spaceName: 'My Docs',
            requiredPluginIds: [pluginId],
          },
        },
        signal,
      )
      refs.push(
        retainRegistration(docsQuickstart.resourceId, 'docs quickstart'),
      )
      const blogQuickstart = await qsSvc.RegisterQuickstart(
        {
          registration: {
            quickstartId: 'blog',
            pluginId,
            name: 'Create a Blog',
            description: 'Date-based markdown blog',
            category: 'content',
            iconName: 'pen',
            hidden: true,
            experimental: true,
            spaceName: 'My Blog',
            requiredPluginIds: [pluginId],
          },
        },
        signal,
      )
      refs.push(
        retainRegistration(blogQuickstart.resourceId, 'blog quickstart'),
      )

      await waitForAbort(signal)
    } finally {
      releaseRetained()
    }
  })()

  return {
    startup: Promise.resolve(),
    done,
  }
}

// main is the notes backend entry point.
export default function main(
  api: BackendAPI,
  signal: AbortSignal,
): BackendEntrypointLifecycle {
  return startNotesBackend(api, signal)
}
