import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { HandleStreamCtr, type PacketStream } from 'starpc'
import type { BackendAPI } from '@aptre/bldr-sdk'

import { PluginStartInfo } from '../../../plugin/plugin.pb.js'
import main from './plugin-goscript.js'

const originalMessageChannel = globalThis.MessageChannel
const originalConsoleWarn = console.warn
let reportedFailures: unknown[]

describe('plugin-goscript generation lifecycle', () => {
  beforeEach(() => {
    globalThis.MessageChannel = originalMessageChannel
    console.warn = vi.fn()
    reportedFailures = []
    delete globalThis.BLDR_PLUGIN_START_INFO
    globalThis.BLDR_BLAKE3 = buildInstalledBlake3Sidecar()
    globalThis.BLDR_PLUGIN_REPORT_RUNTIME_FAILURE = (err: unknown) => {
      reportedFailures.push(err)
    }
  })

  afterEach(() => {
    globalThis.MessageChannel = originalMessageChannel
    console.warn = originalConsoleWarn
    delete globalThis.BLDR_BLAKE3
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('publishes start info and reports plugin main failure', async () => {
    const err = new Error('fatal goscript exit')
    const api = buildBackendAPI()
    const failureReported = new Promise<void>((resolve) => {
      globalThis.BLDR_PLUGIN_REPORT_RUNTIME_FAILURE = (
        reportedErr: unknown,
      ) => {
        reportedFailures.push(reportedErr)
        resolve()
      }
    })

    await main(api, async () => async () => {
      throw err
    })
    await failureReported

    expect(globalThis.BLDR_PLUGIN_START_INFO).toBe(
      btoa(PluginStartInfo.toJsonString(api.startInfo)),
    )
    expect(reportedFailures).toEqual([err])
  })

  it('installs the BLAKE3 sidecar before loading GoScript plugin main', async () => {
    const api = buildBackendAPI()
    delete globalThis.BLDR_BLAKE3
    const fetchBlake3 = vi.fn(async () => {
      return new Response(new Uint8Array([0, 1, 2]), { status: 200 })
    })
    vi.stubGlobal('fetch', fetchBlake3)
    const instantiateBlake3 = vi
      .spyOn(WebAssembly, 'instantiate')
      .mockResolvedValue(
        buildBlake3Instantiation() as unknown as WebAssembly.Instance,
      )
    const lifecycle: string[] = []
    let resolvePluginStarted!: () => void
    const pluginStarted = new Promise<void>((resolve) => {
      resolvePluginStarted = resolve
    })

    await main(api, async () => {
      lifecycle.push(
        globalThis.BLDR_BLAKE3 ? 'loader:installed' : 'loader:missing',
      )
      return () => {
        lifecycle.push(
          globalThis.BLDR_BLAKE3 ? 'main:installed' : 'main:missing',
        )
        resolvePluginStarted()
        return new Promise<void>(() => {})
      }
    })
    await pluginStarted

    expect(lifecycle).toEqual(['loader:installed', 'main:installed'])
    expect(fetchBlake3).toHaveBeenCalledTimes(1)
    expect(instantiateBlake3).toHaveBeenCalledTimes(1)
  })

  it('turns accept-stream into a terminal error after the GoScript plugin exits', async () => {
    let rejectPluginMain!: (err: unknown) => void
    const pluginMainExited = new Promise<void>((_resolve, reject) => {
      rejectPluginMain = reject
    })
    const api = buildBackendAPI()

    await main(api, async () => () => pluginMainExited)
    await Promise.resolve()
    await Promise.resolve()

    const err = new Error('runtime exited')
    rejectPluginMain(err)
    await Promise.resolve()
    await Promise.resolve()

    await expect(
      api.handleStreamCtr.handleStreamFunc(buildPacketStream()),
    ).rejects.toThrow('runtime exited')
  })

  it('closes active accepted streams when the GoScript plugin exits', async () => {
    let rejectPluginMain!: (err: unknown) => void

    const pluginMainExited = new Promise<void>((_resolve, reject) => {
      rejectPluginMain = reject
    })
    const api = buildBackendAPI()
    const failureReported = new Promise<void>((resolve) => {
      globalThis.BLDR_PLUGIN_REPORT_RUNTIME_FAILURE = (
        reportedErr: unknown,
      ) => {
        reportedFailures.push(reportedErr)
        resolve()
      }
    })
    await main(api, async () => () => pluginMainExited)
    await Promise.resolve()
    await Promise.resolve()

    const acceptedChannel = buildMessageChannel()
    globalThis.MessageChannel = vi.fn(function () {
      return acceptedChannel
    })
    const setAcceptStream = globalThis.BLDR_PLUGIN_SET_ACCEPT_STREAM
    expect(setAcceptStream).toBeTypeOf('function')
    const acceptStream = vi.fn()
    if (!setAcceptStream) {
      throw new Error('missing accept stream setter')
    }
    setAcceptStream(acceptStream)

    void api.handleStreamCtr.handleStreamFunc(buildPendingPacketStream())
    await Promise.resolve()

    expect(acceptStream).toHaveBeenCalledWith(acceptedChannel.port1)
    expect(acceptedChannel.port2.close).not.toHaveBeenCalled()

    rejectPluginMain(new Error('fatal goscript exit'))
    await failureReported

    expect(acceptedChannel.port2.close).toHaveBeenCalledTimes(1)
    await expect(
      api.handleStreamCtr.handleStreamFunc(buildPacketStream()),
    ).rejects.toThrow('fatal goscript exit')
  })
})

function buildBackendAPI(): BackendAPI {
  return {
    startInfo: {
      instanceId: 'inst1',
      pluginId: 'goscript-runtime-proof',
      instanceKey: 'default',
    },
    openStream: vi.fn(),
    handleStreamCtr: new HandleStreamCtr(),
  } as unknown as BackendAPI
}

function buildPacketStream(): PacketStream {
  return {
    source: (async function* () {
      await new Promise<void>(() => {})
      yield new Uint8Array()
    })(),
    sink: vi.fn(async () => {}),
  }
}

function buildPendingPacketStream(): PacketStream {
  return {
    source: (async function* () {
      await new Promise<void>(() => {})
      yield new Uint8Array()
    })(),
    sink: vi.fn(async () => {
      await new Promise<void>(() => {})
    }),
  }
}

function buildMessagePort(): MessagePort {
  return {
    postMessage: vi.fn(),
    close: vi.fn(),
    start: vi.fn(),
  } as unknown as MessagePort
}

function buildMessageChannel(): MessageChannel {
  return {
    port1: buildMessagePort(),
    port2: buildMessagePort(),
  }
}

function buildInstalledBlake3Sidecar() {
  return {
    hash: () => new Uint8Array(),
    keyedHash: () => new Uint8Array(),
    deriveKey: () => new Uint8Array(),
  }
}

function buildBlake3Instantiation(): WebAssembly.WebAssemblyInstantiatedSource {
  return {
    instance: {
      exports: {
        memory: new WebAssembly.Memory({ initial: 1 }),
        blake3_workspace: vi.fn(() => 32),
        blake3_hash: vi.fn(),
        blake3_keyed_hash: vi.fn(),
        blake3_derive_key: vi.fn(),
      },
    } as unknown as WebAssembly.Instance,
    module: {} as WebAssembly.Module,
  }
}
