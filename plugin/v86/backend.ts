import { Client as SRPCClient } from 'starpc'
import type { BackendAPI } from '@aptre/bldr-sdk'
import {
  Client as ResourcesClient,
  ResourceServiceClient,
  type ClientResourceRef,
} from '@aptre/bldr-sdk/resource/index.js'
import { ViewerRegistryResourceServiceClient } from '@s4wave/sdk/viewer/registry/registry_srpc.pb.js'
import { ViewerSurface } from '@s4wave/sdk/viewer/registry/registry.pb.js'
import { V86RuntimeStatus } from '@s4wave/sdk/vm/v86.pb.js'
import {
  V86RuntimeStatusServiceClient,
  V86RuntimeStatusServiceServiceName,
} from '@s4wave/sdk/vm/v86_srpc.pb.js'
import { FSCursorServiceClient } from '@go/github.com/s4wave/spacewave/db/unixfs/rpc/rpc_srpc.pb.js'
import { buildFSHandle } from '@go/github.com/s4wave/spacewave/db/unixfs/rpc/client/fs-handle.js'
import { V86fsServiceServiceName } from '@go/github.com/s4wave/spacewave/db/unixfs/v86fs/v86fs_srpc.pb.js'
import {
  startBrowserV86Runtime,
  type BrowserV86Runtime,
} from './browser-runner.js'
import { createV86fsSrpcAdapter, type V86fsAdapter } from './v86fs-bridge.js'

const V86_RUNTIME_V86FS_SERVICE_PREFIX = 'vm/v86-runtime/v86fs/'
const V86_RUNTIME_STATUS_SERVICE_PREFIX = 'vm/v86-runtime/status/'

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
    if (!pluginId) {
      throw new Error('missing plugin id in backend start info')
    }
    return api.utils.pluginAssetHttpPath(pluginId, 'v/b/fe/' + entry.file)
  }
  return srcPath
}

const S_IFMT = 0o170000
const S_IFDIR = 0o040000

function isDirectoryMode(mode: number): boolean {
  return (mode & S_IFMT) === S_IFDIR
}

// readV86fsMountFile mounts a named v86fs asset and reads the binary file.
// Imported V86Image single-file assets are FS_NODE directories containing the
// original filename; direct file roots remain supported for per-VM overrides.
function readV86fsMountFile(
  adapter: V86fsAdapter,
  mountName: string,
  fileName: string,
): Promise<Uint8Array> {
  const readInode = (inodeId: number, size: number) =>
    new Promise<Uint8Array>((resolve, reject) => {
      adapter.onOpen(inodeId, 0, (status: number, handleId: number) => {
        if (status !== 0) {
          reject(
            new Error(
              'v86fs open "' + mountName + '" failed: status ' + status,
            ),
          )
          return
        }
        adapter.onRead(
          handleId,
          0,
          size,
          (status: number, data: Uint8Array) => {
            adapter.onClose(handleId, () => {})
            if (status !== 0) {
              reject(
                new Error(
                  'v86fs read "' + mountName + '" failed: status ' + status,
                ),
              )
              return
            }
            resolve(data)
          },
        )
      })
    })

  return new Promise((resolve, reject) => {
    adapter.onMount(mountName, (status: number, rootInodeId: number) => {
      if (status !== 0) {
        reject(
          new Error('v86fs mount "' + mountName + '" failed: status ' + status),
        )
        return
      }
      adapter.onGetattr(
        rootInodeId,
        (status: number, _mode: number, size: number) => {
          if (status !== 0) {
            reject(
              new Error(
                'v86fs getattr "' + mountName + '" failed: status ' + status,
              ),
            )
            return
          }

          if (!isDirectoryMode(_mode)) {
            readInode(rootInodeId, size).then(resolve, reject)
            return
          }

          adapter.onLookup(
            rootInodeId,
            fileName,
            (
              status: number,
              inodeId: number,
              _mode: number,
              fileSize: number,
            ) => {
              if (status !== 0) {
                reject(
                  new Error(
                    'v86fs lookup "' +
                      mountName +
                      '/' +
                      fileName +
                      '" failed: status ' +
                      status,
                  ),
                )
                return
              }
              readInode(inodeId, fileSize).then(resolve, reject)
            },
          )
        },
      )
    })
  })
}

function formatErrorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

function v86RuntimeV86fsServiceID(objectKey: string): string {
  return (
    V86_RUNTIME_V86FS_SERVICE_PREFIX + objectKey + '/' + V86fsServiceServiceName
  )
}
function v86RuntimeStatusServiceID(objectKey: string): string {
  return (
    V86_RUNTIME_STATUS_SERVICE_PREFIX +
    objectKey +
    '/' +
    V86RuntimeStatusServiceServiceName
  )
}

function waitForAbort(signal: AbortSignal): Promise<void> {
  if (signal.aborted) return Promise.resolve()
  const gate = Promise.withResolvers<void>()
  const onAbort = () => {
    signal.removeEventListener('abort', onAbort)
    gate.resolve()
  }
  signal.addEventListener('abort', onAbort, { once: true })
  return gate.promise
}

async function reportRuntimeStatus(
  client: V86RuntimeStatusServiceClient,
  objectKey: string,
  generation: bigint,
  status: V86RuntimeStatus,
  errorMessage: string,
  signal?: AbortSignal,
): Promise<bigint> {
  const response = await client.ReportStatus(
    {
      objectKey,
      runGeneration: generation,
      status,
      errorMessage,
    },
    signal,
  )
  if (!response.accepted) {
    throw new Error(
      'v86 runtime status report rejected: ' +
        (response.rejection || 'unknown reason'),
    )
  }
  return response.runGeneration ?? generation
}

async function tryReportRuntimeStatus(
  client: V86RuntimeStatusServiceClient,
  objectKey: string,
  generation: bigint,
  status: V86RuntimeStatus,
  errorMessage: string,
): Promise<void> {
  try {
    await reportRuntimeStatus(
      client,
      objectKey,
      generation,
      status,
      errorMessage,
    )
  } catch (err) {
    console.error(
      '[spacewave-v86] runtime status report failed:',
      formatErrorMessage(err),
    )
  }
}
function waitForRuntimeReady(
  runtime: BrowserV86Runtime,
  signal: AbortSignal,
): Promise<void> {
  if (signal.aborted) {
    runtime.stop()
    return Promise.reject(new Error('v86 runtime startup aborted'))
  }
  const gate = Promise.withResolvers<void>()
  let settled = false
  const finish = (err?: unknown) => {
    if (settled) return
    settled = true
    signal.removeEventListener('abort', onAbort)
    if (err) {
      gate.reject(err instanceof Error ? err : new Error(String(err)))
    } else {
      gate.resolve()
    }
  }
  const onAbort = () => {
    finish(new Error('v86 runtime startup aborted'))
    runtime.stop()
  }
  signal.addEventListener('abort', onAbort, { once: true })
  runtime.ready.then(
    () => finish(),
    (err: unknown) => finish(err),
  )
  return gate.promise
}

// main is the V86 backend entry point.
export default async function main(
  api: BackendAPI,
  signal: AbortSignal,
): Promise<void> {
  if (!api.startInfo.pluginId) {
    throw new Error('missing plugin id in backend start info')
  }

  const coreClient = new SRPCClient(api.buildPluginOpenStream('spacewave-core'))
  const resourcesService = new ResourceServiceClient(coreClient)
  const resourcesClient = new ResourcesClient(resourcesService, signal)

  const [rootRef, v86ViewerScript] = await Promise.all([
    resourcesClient.accessRootResource(),
    resolveAssetPath(api, signal, './plugin/v86/VmV86Viewer.tsx'),
  ])

  const vrSvc = new ViewerRegistryResourceServiceClient(rootRef.client)
  const viewer = await vrSvc.RegisterViewer(
    {
      registration: {
        typeId: 'vm/v86',
        viewerName: 'V86',
        componentId: 'spacewave.v86.viewer',
        scriptPath: v86ViewerScript,
        surface: ViewerSurface.WEB,
      },
    },
    signal,
  )
  if (!viewer.resourceId) {
    throw new Error('v86 viewer registration did not return a resource id')
  }
  const viewerRef = rootRef.createRef(viewer.resourceId)

  const instanceKey = api.startInfo.instanceKey
  if (!instanceKey) {
    console.log('[spacewave-v86] no instance key, viewer-only mode')
    retainUntilAbort(
      signal,
      [rootRef, viewerRef],
      [coreClient, resourcesClient],
    )
    return
  }

  using _rootRef = rootRef
  using _viewerRef = viewerRef

  const statusClient = new V86RuntimeStatusServiceClient(coreClient, {
    service: v86RuntimeStatusServiceID(instanceKey),
  })
  let generation = 0n
  let runtime: BrowserV86Runtime | undefined
  let v86fsBridge: ReturnType<typeof createV86fsSrpcAdapter> | undefined
  const releaseV86fsBridge = () => {
    if (!v86fsBridge) return
    v86fsBridge[Symbol.dispose]()
    v86fsBridge = undefined
  }

  try {
    generation = await reportRuntimeStatus(
      statusClient,
      instanceKey,
      generation,
      V86RuntimeStatus.V86RuntimeStatus_BOOTING,
      '',
      signal,
    )
    if (generation === 0n) {
      throw new Error('v86 runtime status route did not return a generation')
    }

    const bridge = createV86fsSrpcAdapter(coreClient, {
      service: v86RuntimeV86fsServiceID(instanceKey),
    })
    v86fsBridge = bridge

    console.log('[spacewave-v86] loading v86 binaries from UnixFS...')
    const [wasmBuf, biosBuf, vgaBiosBuf, kernelBuf] = await Promise.all([
      readV86fsMountFile(bridge.adapter, 'wasm', 'v86.wasm'),
      readV86fsMountFile(bridge.adapter, 'seabios', 'seabios.bin'),
      readV86fsMountFile(bridge.adapter, 'vgabios', 'vgabios.bin'),
      readV86fsMountFile(bridge.adapter, 'kernel', 'bzImage'),
    ])

    console.log(
      '[spacewave-v86] binaries loaded:',
      `wasm=${wasmBuf.byteLength}`,
      `bios=${biosBuf.byteLength}`,
      `vga=${vgaBiosBuf.byteLength}`,
      `kernel=${kernelBuf.byteLength}`,
    )

    runtime = await startBrowserV86Runtime({
      assets: {
        wasm: wasmBuf,
        bios: biosBuf,
        vgaBios: vgaBiosBuf,
        kernel: kernelBuf,
      },
      instanceKey,
      v86fsAdapter: bridge.adapter,
    })
    await waitForRuntimeReady(runtime, signal)
    generation = await reportRuntimeStatus(
      statusClient,
      instanceKey,
      generation,
      V86RuntimeStatus.V86RuntimeStatus_READY,
      '',
      signal,
    )
  } catch (err) {
    const errorMessage = formatErrorMessage(err)
    console.error('[spacewave-v86] runtime failed:', errorMessage)
    runtime?.stop()
    releaseV86fsBridge()
    await tryReportRuntimeStatus(
      statusClient,
      instanceKey,
      generation,
      signal.aborted
        ? V86RuntimeStatus.V86RuntimeStatus_STOPPED
        : V86RuntimeStatus.V86RuntimeStatus_ERROR,
      signal.aborted ? '' : errorMessage,
    )
    if (signal.aborted) return
    throw err
  }

  console.log(
    '[spacewave-v86] v86 emulator ready, serial channel:',
    runtime.serialChannelName,
  )
  try {
    await waitForAbort(signal)
    runtime.stop()
    await tryReportRuntimeStatus(
      statusClient,
      instanceKey,
      generation,
      V86RuntimeStatus.V86RuntimeStatus_STOPPED,
      '',
    )
  } finally {
    releaseV86fsBridge()
  }
}
