import { Client as SRPCClient } from 'starpc'
import type { BackendAPI } from '@aptre/bldr-sdk'
import {
  Client as ResourcesClient,
  ResourceServiceClient,
  type ClientResourceRef,
} from '@aptre/bldr-sdk/resource/index.js'
import { ViewerRegistryResourceServiceClient } from '@s4wave/sdk/viewer/registry/registry_srpc.pb.js'
import { WorldStateResource } from '@s4wave/sdk/world/world-state.js'
import { FSCursorServiceClient } from '@go/github.com/s4wave/spacewave/db/unixfs/rpc/rpc_srpc.pb.js'
import { buildFSHandle } from '@go/github.com/s4wave/spacewave/db/unixfs/rpc/client/fs-handle.js'
import { createV86fsSrpcAdapter, type V86fsAdapter } from './v86fs-bridge.js'
import { v86SerialChannelName, type SerialFrame } from './serial-channel.js'

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

// readV86fsMountRoot mounts a named v86fs mount and reads the entire
// root inode contents. Used to load kernel/BIOS images from UnixFS
// objects linked via graph edges on the VmV86 world object.
function readV86fsMountRoot(
  adapter: V86fsAdapter,
  mountName: string,
): Promise<Uint8Array> {
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
            reject(new Error('v86fs getattr failed: status ' + status))
            return
          }
          adapter.onOpen(rootInodeId, 0, (status: number, handleId: number) => {
            if (status !== 0) {
              reject(new Error('v86fs open failed: status ' + status))
              return
            }
            adapter.onRead(
              handleId,
              0,
              size,
              (status: number, data: Uint8Array) => {
                adapter.onClose(handleId, () => {})
                if (status !== 0) {
                  reject(new Error('v86fs read failed: status ' + status))
                  return
                }
                resolve(data)
              },
            )
          })
        },
      )
    })
  })
}

// main is the vm backend entry point. It runs inside the spacewave-app
// plugin worker alongside the notes backend and the frontend.
export default async function main(
  api: BackendAPI,
  signal: AbortSignal,
): Promise<void> {
  // Connect to spacewave-core via plugin open stream.
  const coreClient = new SRPCClient(api.buildPluginOpenStream('spacewave-core'))
  const resourcesService = new ResourceServiceClient(coreClient)
  const resourcesClient = new ResourcesClient(resourcesService, signal)
  const rootRef = await resourcesClient.accessRootResource()

  // Resolve the V86 viewer script path from the Vite manifest.
  const v86ViewerScript = await resolveAssetPath(
    api,
    signal,
    './plugin/vm/VmV86Viewer.tsx',
  )

  // Register Viewer for the V86 type.
  const vrSvc = new ViewerRegistryResourceServiceClient(rootRef.client)
  const viewer = await vrSvc.RegisterViewer(
    {
      registration: {
        typeId: 'spacewave/vm/v86',
        viewerName: 'V86',
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
    console.log('[spacewave-vm] no instance key, viewer-only mode')
    retainUntilAbort(
      signal,
      [rootRef, viewerRef],
      [coreClient, resourcesClient],
    )
    return
  }

  using _rootRef = rootRef
  using _viewerRef = viewerRef

  console.log('[spacewave-vm] booting v86 for instance:', instanceKey)

  // Access the VmV86 world object's typed resource to reach the v86fs service.
  // The Go-side vmV86Factory registered V86FsService on this resource's mux.
  const worldState = new WorldStateResource(rootRef)
  const typedAccess = await worldState.accessTypedObject(instanceKey, signal)
  if (!typedAccess.resourceId) {
    console.error('[spacewave-vm] failed to access typed object:', instanceKey)
    return
  }

  // Create a resource ref to the typed object's mux (has V86FsService).
  using vmRef = rootRef.createRef(typedAccess.resourceId)

  // Create v86fs SRPC adapter connected to the Go v86fs server via the typed resource.
  using v86fsBridge = createV86fsSrpcAdapter(vmRef.client)

  console.log('[spacewave-vm] loading v86 binaries from UnixFS...')

  // Load wasm/seabios/vgabios/kernel from UnixFS via v86fs mounts resolved
  // through the VmV86 -> V86Image graph edges. The rootfs mount is resolved
  // by the guest kernel itself at init time via MOUNT("") once v86 boots.
  const [wasmBuf, biosBuf, vgaBiosBuf, kernelBuf] = await Promise.all([
    readV86fsMountRoot(v86fsBridge.adapter, 'wasm'),
    readV86fsMountRoot(v86fsBridge.adapter, 'seabios'),
    readV86fsMountRoot(v86fsBridge.adapter, 'vgabios'),
    readV86fsMountRoot(v86fsBridge.adapter, 'kernel'),
  ])

  console.log(
    '[spacewave-vm] binaries loaded:',
    `wasm=${wasmBuf.byteLength}`,
    `bios=${biosBuf.byteLength}`,
    `vga=${vgaBiosBuf.byteLength}`,
    `kernel=${kernelBuf.byteLength}`,
  )

  // Import V86 constructor (no type declarations, use dynamic import).
  const { V86 } = await import('@aptre/v86')

  // Boot v86 headless with v86fs root mount.
  const emulator = new V86({
    wasm: { buffer: wasmBuf.buffer },
    memory_size: 256 * 1024 * 1024,
    vga_memory_size: 2 * 1024 * 1024,
    bios: { buffer: biosBuf.buffer },
    vga_bios: { buffer: vgaBiosBuf.buffer },
    bzimage: { buffer: kernelBuf.buffer },
    cmdline: 'rw init=/usr/bin/bash root=v86fs rootfstype=v86fs console=ttyS0',
    virtio_v86fs: true,
    virtio_v86fs_adapter: v86fsBridge.adapter,
    autostart: true,
  })

  // Serial I/O bridge (IC-5): relay serial bytes between the emulator and
  // the VmV86Viewer via a BroadcastChannel keyed by the VmV86 object key.
  // Output bytes are forwarded to every subscriber; input frames posted by a
  // viewer are fed back into the emulator via serial0_send one at a time.
  const serialChannelName = v86SerialChannelName(instanceKey)
  const serialChannel = new BroadcastChannel(serialChannelName)
  emulator.add_listener('serial0-output-byte', (byte: number) => {
    serialChannel.postMessage({ dir: 'out', byte })
  })
  serialChannel.onmessage = (ev: MessageEvent<SerialFrame>) => {
    const frame = ev.data
    if (!frame || frame.dir !== 'in') return
    if (typeof frame.text === 'string' && frame.text.length > 0) {
      emulator.serial0_send(frame.text)
    }
  }

  console.log(
    '[spacewave-vm] v86 emulator started, serial channel:',
    serialChannelName,
  )

  // Block until shutdown, then clean up.
  await new Promise<void>((resolve) => {
    signal.addEventListener('abort', () => resolve(), { once: true })
  })

  serialChannel.close()
  emulator.stop()
  emulator.destroy()
}
