import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { HandleStreamCtr, type PacketStream } from 'starpc'
import type { BackendAPI } from '@aptre/bldr-sdk'

import { PluginStartInfo } from '../../../plugin/plugin.pb.js'
import main from './plugin-goscript.js'

const originalMessageChannel = globalThis.MessageChannel
const originalConsoleWarn = console.warn

describe('plugin-goscript generation lifecycle', () => {
  beforeEach(() => {
    globalThis.MessageChannel = originalMessageChannel
    console.warn = vi.fn()
    delete globalThis.BLDR_PLUGIN_START_INFO
  })

  afterEach(() => {
    globalThis.MessageChannel = originalMessageChannel
    console.warn = originalConsoleWarn
    delete globalThis.BLDR_PLUGIN_SET_ACCEPT_STREAM
  })

  it('publishes start info and rejects lifecycle on plugin main failure', async () => {
    const err = new Error('fatal goscript exit')
    const api = buildBackendAPI()

    const lifecycle = main(api, async () => async () => {
      throw err
    })

    expect(globalThis.BLDR_PLUGIN_START_INFO).toBe(
      btoa(PluginStartInfo.toJsonString(api.startInfo)),
    )
    await expect(Promise.resolve(lifecycle.startup)).rejects.toBe(err)
    await expect(Promise.resolve(lifecycle.done)).rejects.toBe(err)
  })

  it('publishes startup only after the GoScript runtime accepts streams', async () => {
    const api = buildBackendAPI()
    const lifecycle = main(api, async () => async () => {
      await new Promise<void>(() => {})
    })
    let started = false
    void Promise.resolve(lifecycle.startup).then(() => {
      started = true
    })

    await Promise.resolve()
    await Promise.resolve()
    expect(started).toBe(false)

    const setAcceptStream = globalThis.BLDR_PLUGIN_SET_ACCEPT_STREAM
    expect(setAcceptStream).toBeTypeOf('function')
    if (!setAcceptStream) {
      throw new Error('missing accept stream setter')
    }
    setAcceptStream(vi.fn())
    await lifecycle.startup

    expect(started).toBe(true)
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
    await Promise.resolve()
    await Promise.resolve()
    await Promise.resolve()
    await Promise.resolve()

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
