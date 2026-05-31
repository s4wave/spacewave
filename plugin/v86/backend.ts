import { Client as SRPCClient } from 'starpc'
import type { BackendAPI } from '@aptre/bldr-sdk'
import {
  Client as ResourcesClient,
  ResourceServiceClient,
  type ClientResourceRef,
} from '@aptre/bldr-sdk/resource/index.js'
import { ViewerRegistryResourceServiceClient } from '@s4wave/sdk/viewer/registry/registry_srpc.pb.js'
import { FSCursorServiceClient } from '@go/github.com/s4wave/spacewave/db/unixfs/rpc/rpc_srpc.pb.js'
import { buildFSHandle } from '@go/github.com/s4wave/spacewave/db/unixfs/rpc/client/fs-handle.js'
import { V86fsServiceServiceName } from '@go/github.com/s4wave/spacewave/db/unixfs/v86fs/v86fs_srpc.pb.js'
import { startBrowserV86Runtime } from './browser-runner.js'
import { createV86fsSrpcAdapter, type V86fsAdapter } from './v86fs-bridge.js'

const V86_RUNTIME_V86FS_SERVICE_PREFIX = 'vm/v86-runtime/v86fs/'

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

// main is the V86 backend entry point.
export default async function main(
  api: BackendAPI,
  signal: AbortSignal,
): Promise<void> {
  if (!api.startInfo.pluginId) {
    throw new Error('missing plugin id in backend start info')
  }

  // Connect to spacewave-core via plugin open stream.
  const coreClient = new SRPCClient(api.buildPluginOpenStream('spacewave-core'))
  const resourcesService = new ResourceServiceClient(coreClient)
  const resourcesClient = new ResourcesClient(resourcesService, signal)

  // Resolve the V86 viewer script path from the Vite manifest.
  const [rootRef, v86ViewerScript] = await Promise.all([
    resourcesClient.accessRootResource(),
    resolveAssetPath(api, signal, './plugin/v86/VmV86Viewer.tsx'),
  ])

  // Register Viewer for the V86 type.
  const vrSvc = new ViewerRegistryResourceServiceClient(rootRef.client)
  const viewer = await vrSvc.RegisterViewer(
    {
      registration: {
        typeId: 'vm/v86',
        viewerName: 'V86',
        componentId: 'spacewave.v86.viewer',
        scriptPath: v86ViewerScript,
      },
    },
    signal,
  )
  if (!viewer.resourceId) {
    throw new Error('v86 viewer registration did not return a resource id')
  }
  const viewerRef = rootRef.createRef(viewer.resourceId)

  // Get the instance key (VmV86 world object key) from plugin start info.
  // When the plugin starts without an instance, return after registration so
  // bldr can finish frontend setup.
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

  console.log('[spacewave-v86] booting v86 for instance:', instanceKey)

  let runtime: Awaited<ReturnType<typeof startBrowserV86Runtime>> | undefined
  try {
    using v86fsBridge = createV86fsSrpcAdapter(coreClient, {
      service: v86RuntimeV86fsServiceID(instanceKey),
    })

    console.log('[spacewave-v86] loading v86 binaries from UnixFS...')

    // Load wasm/seabios/vgabios/kernel from UnixFS via v86fs mounts resolved
    // through the VmV86 -> V86Image graph edges. The rootfs mount is resolved
    // by the guest kernel itself at init time via MOUNT("") once v86 boots.
    const [wasmBuf, biosBuf, vgaBiosBuf, kernelBuf] = await Promise.all([
      readV86fsMountFile(v86fsBridge.adapter, 'wasm', 'v86.wasm'),
      readV86fsMountFile(v86fsBridge.adapter, 'seabios', 'seabios.bin'),
      readV86fsMountFile(v86fsBridge.adapter, 'vgabios', 'vgabios.bin'),
      readV86fsMountFile(v86fsBridge.adapter, 'kernel', 'bzImage'),
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
      v86fsAdapter: v86fsBridge.adapter,
    })
  } catch (err) {
    const errorMessage = formatErrorMessage(err)
    console.error('[spacewave-v86] runtime failed:', errorMessage)
    throw err
  }

  console.log(
    '[spacewave-v86] v86 emulator started, serial channel:',
    runtime.serialChannelName,
  )

  // Block until shutdown, then clean up.
  if (!signal.aborted) {
    await new Promise<void>((resolve) => {
      signal.addEventListener('abort', () => resolve(), { once: true })
    })
  }

  runtime?.stop()
}
