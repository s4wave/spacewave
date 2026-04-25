import { createMux, createHandler, Server, Client as SRPCClient } from 'starpc'
import type { BackendAPI } from '@aptre/bldr-sdk'
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
import type {
  ApplyWorldOpRequest,
  ApplyWorldOpResponse,
  ApplyWorldObjectOpRequest,
  ApplyWorldObjectOpResponse,
  ValidateOpRequest,
  ValidateOpResponse,
} from '@s4wave/sdk/worldop/registry/registry.pb.js'
import { ViewerRegistryResourceServiceClient } from '@s4wave/sdk/viewer/registry/registry_srpc.pb.js'
import { WorldStateResource } from '@s4wave/sdk/world/world-state.js'
import { setObjectType } from '@s4wave/sdk/world/types/types.js'
import { FSCursorServiceClient } from '@go/github.com/s4wave/spacewave/db/unixfs/rpc/rpc_srpc.pb.js'
import { buildFSHandle } from '@go/github.com/s4wave/spacewave/db/unixfs/rpc/client/fs-handle.js'
import {
  FsInitOp,
  FSType,
} from '@go/github.com/s4wave/spacewave/db/unixfs/world/unixfs.pb.js'
import { NotebookResourceServiceDefinition } from './sdk/notebook_srpc.pb.js'
import { BlogResourceServiceDefinition } from './sdk/blog_srpc.pb.js'
import { DocsResourceServiceDefinition } from './sdk/docs_srpc.pb.js'
import { NotebookResource } from './notebook-resource.js'
import { BlogResource } from './blog-resource.js'
import { DocsResource } from './docs-resource.js'
import { createBlogClientSide } from './blog-seed.js'
import { createObjectWithBlockData } from './object-block.js'
import { INIT_NOTEBOOK_OP_ID } from './proto/init-notebook.js'
import { CREATE_BLOG_OP_ID } from './proto/create-blog.js'
import { CREATE_DOCS_OP_ID } from './proto/create-docs.js'
import { InitNotebookOp, Notebook } from './proto/notebook.pb.js'
import { CreateBlogOp } from './proto/blog.pb.js'
import { CreateDocumentationOp, Documentation } from './proto/docs.pb.js'
import { uploadSeedTree } from './unixfs-seed.js'

type ViteManifestEntry = {
  file?: string
}

function retainUntilAbort(
  signal: AbortSignal,
  refs: ClientResourceRef[],
  retained: unknown[],
): void {
  let released = false
  const release = () => {
    if (released) return
    released = true
    for (let i = refs.length - 1; i >= 0; --i) {
      refs[i][Symbol.dispose]()
    }
    retained.length = 0
  }
  if (signal.aborted) {
    release()
    return
  }
  signal.addEventListener('abort', release, { once: true })
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
    const engineId = request.engineResourceId ?? 0
    const objectKey = request.objectKey ?? ''
    const { resourceId } = constructChildResource(() => {
      // If engineId is provided, get an attached ref to the world engine.
      // The engine ref wraps the yamux-backed srpc.Client for RPCs
      // back to the Go bridge's EngineResource mux.
      const engineRef =
        engineId > 0 ?
          getCurrentResourceClient().getAttachedRef(engineId)
        : undefined

      switch (typeId) {
        case 'spacewave-notes/blog': {
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
        case 'spacewave-notes/docs': {
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
        default: {
          // Default to notebook for backward compatibility.
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
    const engineId = request.engineResourceId ?? 0
    if (!engineId) {
      throw new Error('engineResourceId is required')
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
      const fsInitOp: FsInitOp = {
        objectKey: unixfsKey,
        fsType: FSType.FSType_FS_NODE,
        timestamp: op.timestamp,
      }
      await ws.applyWorldOp(
        'hydra/unixfs/init',
        FsInitOp.toBinary(fsInitOp),
        '',
      )

      // 2. Create sample note files via batch tree upload.
      await uploadSeedTree(
        ws,
        unixfsKey,
        [
          { path: 'welcome.md', content: '' },
          { path: 'getting-started.md', content: '' },
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
      await setObjectType(ws, notebookKey, 'spacewave-notes/notebook')

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
    const engineId = request.engineResourceId ?? 0
    if (!engineId) {
      throw new Error('engineResourceId is required')
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
    const engineId = request.engineResourceId ?? 0
    if (!engineId) {
      throw new Error('engineResourceId is required')
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
      const fsInitOp: FsInitOp = {
        objectKey: unixfsKey,
        fsType: FSType.FSType_FS_NODE,
        timestamp: op.timestamp,
      }
      await ws.applyWorldOp(
        'hydra/unixfs/init',
        FsInitOp.toBinary(fsInitOp),
        '',
      )

      // 2. Create the initial docs tree via batch upload.
      await uploadSeedTree(
        ws,
        unixfsKey,
        [{ path: 'index.md', content: '' }],
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
      await setObjectType(ws, docsKey, 'spacewave-notes/docs')

      return {}
    } finally {
      ws.release()
      wsRef.release()
    }
  }
}

// main is the notes backend entry point. It runs inside the spacewave-app
// plugin worker alongside the vm backend and the frontend.
export default async function main(
  api: BackendAPI,
  signal: AbortSignal,
): Promise<void> {
  // Build root mux with ObjectTypeHandler and WorldOpHandler services.
  const otHandler = new NotesObjectTypeHandler()
  const opHandler = new NotesWorldOpHandler()
  const rootMux = newResourceMux(
    createHandler(ObjectTypeHandlerServiceDefinition, otHandler),
    createHandler(WorldOpHandlerServiceDefinition, opHandler),
  )

  // Create ResourceServer with the root mux.
  const resourceServer = new ResourceServer(rootMux)
  const outerMux = createMux()
  resourceServer.register(outerMux)

  // Wire incoming streams to the server.
  const server = new Server(outerMux.lookupMethod)
  api.handleStreamCtr.set((channel) => {
    server.handlePacketStream(channel)
    return Promise.resolve()
  })

  // Connect to spacewave-core via plugin open stream.
  const coreClient = new SRPCClient(api.buildPluginOpenStream('spacewave-core'))
  const resourcesService = new ResourceServiceClient(coreClient)
  const resourcesClient = new ResourcesClient(resourcesService, signal)
  const rootRef = await resourcesClient.accessRootResource()
  const refs: ClientResourceRef[] = [rootRef]
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
    { typeId: 'spacewave-notes/notebook', pluginId: 'spacewave-app' },
    signal,
  )
  refs.push(retainRegistration(notebookType.resourceId, 'notebook object type'))
  const blogType = await otSvc.RegisterObjectType(
    { typeId: 'spacewave-notes/blog', pluginId: 'spacewave-app' },
    signal,
  )
  refs.push(retainRegistration(blogType.resourceId, 'blog object type'))
  const docsType = await otSvc.RegisterObjectType(
    { typeId: 'spacewave-notes/docs', pluginId: 'spacewave-app' },
    signal,
  )
  refs.push(retainRegistration(docsType.resourceId, 'docs object type'))

  // Register WorldOps.
  const woSvc = new WorldOpRegistryResourceServiceClient(rootRef.client)
  const initNotebookOp = await woSvc.RegisterWorldOp(
    { operationTypeId: INIT_NOTEBOOK_OP_ID, pluginId: 'spacewave-app' },
    signal,
  )
  refs.push(
    retainRegistration(initNotebookOp.resourceId, 'init notebook world op'),
  )
  const createBlogOp = await woSvc.RegisterWorldOp(
    { operationTypeId: CREATE_BLOG_OP_ID, pluginId: 'spacewave-app' },
    signal,
  )
  refs.push(retainRegistration(createBlogOp.resourceId, 'create blog world op'))
  const createDocsOp = await woSvc.RegisterWorldOp(
    { operationTypeId: CREATE_DOCS_OP_ID, pluginId: 'spacewave-app' },
    signal,
  )
  refs.push(retainRegistration(createDocsOp.resourceId, 'create docs world op'))

  // Resolve viewer script paths from the Vite manifest so the
  // frontend gets the hashed output paths (not the source paths).
  const notebookViewerScript = await resolveAssetPath(
    api,
    signal,
    './plugin/notes/NotebookViewer.tsx',
  )
  const blogViewerScript = await resolveAssetPath(
    api,
    signal,
    './plugin/notes/BlogViewer.tsx',
  )
  const docsViewerScript = await resolveAssetPath(
    api,
    signal,
    './plugin/notes/DocsViewer.tsx',
  )

  // Register Viewers.
  const vrSvc = new ViewerRegistryResourceServiceClient(rootRef.client)
  const notebookViewer = await vrSvc.RegisterViewer(
    {
      registration: {
        typeId: 'spacewave-notes/notebook',
        viewerName: 'Notebook',
        scriptPath: notebookViewerScript,
      },
    },
    signal,
  )
  refs.push(retainRegistration(notebookViewer.resourceId, 'notebook viewer'))
  const blogViewer = await vrSvc.RegisterViewer(
    {
      registration: {
        typeId: 'spacewave-notes/blog',
        viewerName: 'Blog',
        scriptPath: blogViewerScript,
      },
    },
    signal,
  )
  refs.push(retainRegistration(blogViewer.resourceId, 'blog viewer'))
  const docsViewer = await vrSvc.RegisterViewer(
    {
      registration: {
        typeId: 'spacewave-notes/docs',
        viewerName: 'Documentation',
        scriptPath: docsViewerScript,
      },
    },
    signal,
  )
  refs.push(retainRegistration(docsViewer.resourceId, 'docs viewer'))

  retainUntilAbort(signal, refs, [
    coreClient,
    resourcesClient,
    otHandler,
    opHandler,
    rootMux,
    resourceServer,
    outerMux,
    server,
  ])
}
