import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  detectV86RuntimeEnvironment,
  getV86RunnerContract,
  startBrowserV86Runtime,
  type V86BootAssets,
} from './browser-runner.js'

const h = vi.hoisted(() => ({
  broadcastChannels: [] as Array<{
    name: string
    messages: unknown[]
    close: ReturnType<typeof vi.fn>
    onmessage: ((event: { data: unknown }) => void) | null
    postMessage(message: unknown): void
  }>,
  constructors: [] as Array<Record<string, unknown>>,
  serialOutputListener: undefined as ((byte: number) => void) | undefined,
  instances: [] as Array<{
    add_listener: ReturnType<typeof vi.fn>
    serial0_send: ReturnType<typeof vi.fn>
    stop: ReturnType<typeof vi.fn>
    destroy: ReturnType<typeof vi.fn>
  }>,
}))

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

class FakeV86 {
  add_listener = vi.fn((event: string, listener: (byte: number) => void) => {
    if (event === 'serial0-output-byte') {
      h.serialOutputListener = listener
    }
  })
  serial0_send = vi.fn()
  stop = vi.fn()
  destroy = vi.fn()

  constructor(options: Record<string, unknown>) {
    h.constructors.push(options)
    h.instances.push(this)
  }
}

function bootAssets(): V86BootAssets {
  return {
    wasm: new Uint8Array([1, 2, 3, 4]),
    bios: new Uint8Array([5, 6, 7, 8]),
    vgaBios: new Uint8Array([9, 10, 11, 12]),
    kernel: new Uint8Array([13, 14, 15, 16]),
  }
}

describe('browser V86 runner contract', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  beforeEach(() => {
    h.broadcastChannels.length = 0
    h.constructors.length = 0
    h.serialOutputListener = undefined
    h.instances.length = 0
  })

  it('describes headless worker ownership without requiring document at import time', () => {
    const env = detectV86RuntimeEnvironment({
      BroadcastChannel: FakeBroadcastChannel,
      self: {},
    } as never)
    const contract = getV86RunnerContract(env)

    expect(env).toEqual(
      expect.objectContaining({
        executionOwner: 'worker',
        hasDocument: false,
        hasBroadcastChannel: true,
      }),
    )
    expect(contract).toEqual({
      backendExecution: 'worker',
      displayOwner: 'viewer',
      filesystemBridge: 'v86fs-adapter',
      screenBridge: 'headless',
      terminalBridge: 'BroadcastChannel',
    })
  })

  it('starts V86 headless and owns the serial BroadcastChannel bridge', async () => {
    vi.stubGlobal('BroadcastChannel', FakeBroadcastChannel)
    const adapter = { tag: 'v86fs' }
    const runtime = await startBrowserV86Runtime(
      {
        assets: bootAssets(),
        instanceKey: 'vm/v86/test',
        v86fsAdapter: adapter as never,
      },
      () => Promise.resolve({ V86: FakeV86 }),
    )

    expect(h.constructors).toHaveLength(1)
    expect(h.constructors[0]).toEqual(
      expect.objectContaining({
        virtio_v86fs: true,
        virtio_v86fs_adapter: adapter,
        autostart: true,
      }),
    )
    expect(runtime.serialChannelName).toBe('v86-serial-vm/v86/test')

    const channel = h.broadcastChannels[0]
    h.serialOutputListener?.(65)
    expect(channel?.messages).toEqual([{ dir: 'out', byte: 65 }])
    channel?.onmessage?.({ data: { dir: 'in', text: 'help\n' } })
    expect(h.instances[0]?.serial0_send).toHaveBeenCalledWith('help\n')

    runtime.stop()
    runtime.stop()
    expect(channel?.close).toHaveBeenCalledTimes(1)
    expect(h.instances[0]?.stop).toHaveBeenCalledTimes(1)
    expect(h.instances[0]?.destroy).toHaveBeenCalledTimes(1)
  })

  it('does not import V86 until the serial bridge environment is available', async () => {
    vi.stubGlobal('BroadcastChannel', undefined)
    const loadModule = vi.fn(() => Promise.resolve({ V86: FakeV86 }))

    await expect(
      startBrowserV86Runtime(
        {
          assets: bootAssets(),
          instanceKey: 'vm/v86/test',
          v86fsAdapter: {} as never,
        },
        loadModule,
      ),
    ).rejects.toThrow('requires BroadcastChannel')
    expect(loadModule).not.toHaveBeenCalled()
  })

  it('reports DOM-sensitive V86 import failures at the runner boundary', async () => {
    vi.stubGlobal('BroadcastChannel', FakeBroadcastChannel)

    await expect(
      startBrowserV86Runtime(
        {
          assets: bootAssets(),
          instanceKey: 'vm/v86/test',
          v86fsAdapter: {} as never,
        },
        () => Promise.reject(new ReferenceError('document is not defined')),
      ),
    ).rejects.toThrow('worker-safe V86 module or move DOM-dependent boot')
    expect(h.broadcastChannels).toHaveLength(0)
  })
})
