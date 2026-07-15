import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { V86RuntimeStatus } from '@s4wave/sdk/vm/v86.pb.js'

const h = vi.hoisted(() => ({
  buildPluginOpenStream: vi.fn(),
  pluginAssetHttpPath: vi.fn(),
  viewerRegistrations: [] as Array<Record<string, unknown>>,
  mountNames: [] as string[],
  v86Constructors: [] as Array<Record<string, unknown>>,
  v86Instances: [] as Array<{
    add_listener: ReturnType<typeof vi.fn>
    serial0_send: ReturnType<typeof vi.fn>
    stop: ReturnType<typeof vi.fn>
    destroy: ReturnType<typeof vi.fn>
  }>,
  v86BootWaiters: [] as Array<() => void>,
  serialOutputListener: undefined as ((byte: number) => void) | undefined,
  runtimeStatusReports: [] as Array<{
    service: string
    request: Record<string, unknown>
  }>,
  broadcastChannels: [] as Array<{
    name: string
    messages: unknown[]
    close: ReturnType<typeof vi.fn>
    onmessage: ((event: { data: unknown }) => void) | null
    postMessage(message: unknown): void
  }>,
  v86fsBridges: [] as Array<{
    adapter: Record<string, unknown>
    rpc: unknown
    service?: string
    close: ReturnType<typeof vi.fn>
    [Symbol.dispose](): void
  }>,
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
  Client: class {
    public stream: unknown

    constructor(stream: unknown) {
      this.stream = stream
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

vi.mock('@s4wave/sdk/viewer/registry/registry_srpc.pb.js', () => ({
  ViewerRegistryResourceServiceClient: class {
    constructor(_client: unknown) {}

    RegisterViewer(req: { registration: Record<string, unknown> }) {
      h.viewerRegistrations.push(req.registration)
      return Promise.resolve({ resourceId: h.nextResourceId++ })
    }
  },
}))

vi.mock('./v86fs-bridge.js', () => ({
  createV86fsSrpcAdapter: vi.fn((rpc: unknown, opts?: { service?: string }) => {
    const adapter = {
      onMount(
        name: string,
        reply: (status: number, rootInodeId: number) => void,
      ) {
        h.mountNames.push(name)
        reply(0, 100)
      },
      onGetattr(
        _inodeId: number,
        reply: (status: number, mode: number, size: number) => void,
      ) {
        reply(0, 0, 4)
      },
      onOpen(
        _inodeId: number,
        _flags: number,
        reply: (status: number, handleId: number) => void,
      ) {
        reply(0, 7)
      },
      onRead(
        _handleId: number,
        _offset: number,
        _size: number,
        reply: (status: number, data: Uint8Array) => void,
      ) {
        reply(0, new Uint8Array([1, 2, 3, 4]))
      },
      onClose(_handleId: number, reply: () => void) {
        reply()
      },
    }
    const bridge = {
      adapter,
      rpc,
      service: opts?.service,
      close: vi.fn(),
      [Symbol.dispose]() {
        bridge.close()
      },
    }
    h.v86fsBridges.push(bridge)
    return bridge
  }),
}))
vi.mock('@s4wave/sdk/vm/v86_srpc.pb.js', () => ({
  V86RuntimeStatusServiceClient: class {
    private readonly service: string

    constructor(_client: unknown, options?: { service?: string }) {
      this.service = options?.service ?? ''
    }

    ReportStatus(request: Record<string, unknown>) {
      h.runtimeStatusReports.push({ service: this.service, request })
      return Promise.resolve({
        accepted: true,
        runGeneration:
          request.runGeneration === 0n ? 17n : request.runGeneration,
      })
    }
  },
  V86RuntimeStatusServiceServiceName: 's4wave.vm.V86RuntimeStatusService',
}))

vi.mock('@aptre/v86', () => ({
  V86: class {
    add_listener = vi.fn((event: string, listener: (byte: number) => void) => {
      if (event === 'serial0-output-byte') {
        h.serialOutputListener = listener
      }
    })
    serial0_send = vi.fn()
    stop = vi.fn()
    destroy = vi.fn()

    constructor(opts: Record<string, unknown>) {
      h.v86Constructors.push(opts)
      h.v86Instances.push(this)
      for (const resolve of h.v86BootWaiters.splice(0)) {
        resolve()
      }
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
        'plugin/v86/VmV86Viewer.tsx': { file: 'assets/v86-viewer.mjs' },
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

class FakeBroadcastChannel {
  name: string
  messages: unknown[] = []
  close = vi.fn()
  onmessage: ((event: { data: unknown }) => void) | null = null

  constructor(name: string) {
    this.name = name
    h.broadcastChannels.push(this)
  }

  postMessage(message: unknown) {
    this.messages.push(message)
  }
}

function buildApi(pluginId: string, instanceKey = '') {
  const client = {}
  return {
    startInfo: { pluginId, instanceKey },
    client,
    buildPluginOpenStream: vi.fn((target: string) => target),
    utils: {
      pluginAssetHttpPath: h.pluginAssetHttpPath,
    },
  }
}

function waitForV86Boot(): Promise<void> {
  if (h.v86Constructors.length > 0) {
    return Promise.resolve()
  }
  return new Promise((resolve) => {
    h.v86BootWaiters.push(resolve)
  })
}

describe('v86 backend registration', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  beforeEach(() => {
    vi.clearAllMocks()
    h.viewerRegistrations.length = 0
    h.mountNames.length = 0
    h.v86Constructors.length = 0
    h.v86Instances.length = 0
    h.v86BootWaiters.length = 0
    h.serialOutputListener = undefined
    h.runtimeStatusReports.length = 0
    h.broadcastChannels.length = 0
    h.v86fsBridges.length = 0
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

  it('boots an instanced runtime through the plugin-owned V86 bridge and serial channel', async () => {
    vi.stubGlobal('BroadcastChannel', FakeBroadcastChannel)
    const abort = new AbortController()
    const api = buildApi('spacewave-v86', 'vm/v86/test')

    const running = main(api as never, abort.signal)
    await waitForV86Boot()

    expect(h.v86fsBridges[0]?.service).toBe(
      'vm/v86-runtime/v86fs/vm/v86/test/unixfs.v86fs.V86fsService',
    )
    expect(h.v86fsBridges[0]?.rpc).toEqual(
      expect.objectContaining({ stream: 'spacewave-core' }),
    )
    expect(h.retainedRefs.map((entry) => entry.resourceId)).toEqual([1])
    expect(h.mountNames).toEqual(['wasm', 'seabios', 'vgabios', 'kernel'])
    expect(h.v86Constructors).toHaveLength(1)
    expect(h.v86Constructors[0]).toEqual(
      expect.objectContaining({
        wasm_fn: expect.any(Function),
        virtio_v86fs: true,
        virtio_v86fs_adapter: h.v86fsBridges[0]?.adapter,
        disable_keyboard: true,
        disable_mouse: true,
        disable_speaker: true,
        autostart: true,
      }),
    )
    for (const byte of 'login: '.split('').map((char) => char.charCodeAt(0))) {
      h.serialOutputListener?.(byte)
    }
    await vi.waitFor(() => {
      expect(h.runtimeStatusReports).toHaveLength(2)
    })
    expect(h.runtimeStatusReports.map(({ request }) => request.status)).toEqual(
      [
        V86RuntimeStatus.V86RuntimeStatus_BOOTING,
        V86RuntimeStatus.V86RuntimeStatus_READY,
      ],
    )
    expect(h.v86fsBridges[0]?.close).not.toHaveBeenCalled()
    h.broadcastChannels[0]?.messages.splice(0)

    const channel = h.broadcastChannels[0]
    expect(channel?.name).toBe('v86-serial-vm/v86/test')
    h.serialOutputListener?.(65)
    expect(channel?.messages).toEqual([{ dir: 'out', byte: 65 }])
    channel?.onmessage?.({ data: { dir: 'in', text: 'help\n' } })
    expect(h.v86Instances[0]?.serial0_send).toHaveBeenCalledWith('help\n')

    abort.abort()
    await running
    expect(h.runtimeStatusReports.at(-1)?.request.status).toBe(
      V86RuntimeStatus.V86RuntimeStatus_STOPPED,
    )

    expect(channel?.close).toHaveBeenCalledTimes(1)
    expect(h.v86Instances[0]?.stop).toHaveBeenCalledTimes(1)
    expect(h.v86Instances[0]?.destroy).toHaveBeenCalledTimes(1)
    expect(h.v86fsBridges[0]?.close).toHaveBeenCalledTimes(1)
    expect(h.retainedRefs[0]?.ref[Symbol.dispose]).toHaveBeenCalledTimes(1)
    expect(h.rootRef[Symbol.dispose]).toHaveBeenCalledTimes(1)
  })
  it('stops startup when the runtime is aborted before guest readiness', async () => {
    vi.stubGlobal('BroadcastChannel', FakeBroadcastChannel)
    const abort = new AbortController()
    const running = main(
      buildApi('spacewave-v86', 'vm/v86/test') as never,
      abort.signal,
    )
    await waitForV86Boot()
    abort.abort()
    await running

    expect(h.v86Instances[0]?.stop).toHaveBeenCalledTimes(1)
    expect(h.v86Instances[0]?.destroy).toHaveBeenCalledTimes(1)
    expect(h.runtimeStatusReports.map(({ request }) => request.status)).toEqual(
      [
        V86RuntimeStatus.V86RuntimeStatus_BOOTING,
        V86RuntimeStatus.V86RuntimeStatus_STOPPED,
      ],
    )
  })

  it('propagates runtime boot failures after cleaning up the V86 bridge', async () => {
    vi.stubGlobal('BroadcastChannel', undefined)
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {})

    await expect(
      main(
        buildApi('spacewave-v86', 'vm/v86/test') as never,
        new AbortController().signal,
      ),
    ).rejects.toThrow('requires BroadcastChannel')

    expect(h.v86Constructors).toHaveLength(0)
    expect(h.v86fsBridges[0]?.service).toBe(
      'vm/v86-runtime/v86fs/vm/v86/test/unixfs.v86fs.V86fsService',
    )
    expect(h.v86fsBridges[0]?.close).toHaveBeenCalledTimes(1)
    expect(errorSpy).toHaveBeenCalledWith(
      '[spacewave-v86] runtime failed:',
      'spacewave-v86 browser runner requires BroadcastChannel for the viewer serial bridge',
    )
    expect(h.runtimeStatusReports.map(({ request }) => request.status)).toEqual(
      [
        V86RuntimeStatus.V86RuntimeStatus_BOOTING,
        V86RuntimeStatus.V86RuntimeStatus_ERROR,
      ],
    )
  })

  it('registers the clean v86 viewer with the startInfo plugin id and retained lifetime', async () => {
    const abort = new AbortController()

    await main(buildApi('spacewave-v86') as never, abort.signal)

    expect(h.viewerRegistrations).toEqual([
      {
        typeId: 'vm/v86',
        viewerName: 'V86',
        componentId: 'spacewave.v86.viewer',
        scriptPath: '/asset/spacewave-v86/v/b/fe/assets/v86-viewer.mjs',
      },
    ])
    expect(h.rootRef.createRef).toHaveBeenCalledTimes(1)
    expect(h.retainedRefs.map((entry) => entry.resourceId)).toEqual([1])
    expect(h.retainedRefs[0]?.ref[Symbol.dispose]).not.toHaveBeenCalled()
    expect(h.rootRef[Symbol.dispose]).not.toHaveBeenCalled()

    abort.abort()

    expect(h.retainedRefs[0]?.ref[Symbol.dispose]).toHaveBeenCalledTimes(1)
    expect(h.rootRef[Symbol.dispose]).toHaveBeenCalledTimes(1)
  })

  it('requires a backend startInfo plugin id', async () => {
    await expect(
      main(buildApi('') as never, new AbortController().signal),
    ).rejects.toThrow('missing plugin id in backend start info')
    expect(h.viewerRegistrations).toHaveLength(0)
  })
})
