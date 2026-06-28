import { afterEach, describe, expect, test, vi } from 'vitest'

import {
  PluginWorker,
  PLUGIN_STARTUP_FAILURE_SHUTDOWN_DELAY_MS,
  parsePluginWorkerName,
  waitPluginStartupFailureShutdownDelay,
} from './plugin-worker.js'

describe('parsePluginWorkerName', () => {
  test('strips wrapper parameters from the worker identity', () => {
    expect(
      parsePluginWorkerName(
        'plugin/spacewave-app?s=/b/pd/app.mjs&t=quickjs&p=1',
      ),
    ).toBe('plugin/spacewave-app')
  })
})

describe('waitPluginStartupFailureShutdownDelay', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  test('waits before allowing failed plugin workers to shut down', async () => {
    vi.useFakeTimers()
    const complete = vi.fn()
    const wait = waitPluginStartupFailureShutdownDelay().then(complete)

    await vi.advanceTimersByTimeAsync(
      PLUGIN_STARTUP_FAILURE_SHUTDOWN_DELAY_MS - 1,
    )
    expect(complete).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(1)
    await wait
    expect(complete).toHaveBeenCalledTimes(1)
  })
})

describe('PluginWorker startup shutdown', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  test('does not report expected WebDocument exhaustion as a startup failure', async () => {
    vi.useFakeTimers()
    const global = new FakeDedicatedWorkerGlobal()
    const startPlugin = vi.fn()
    const consoleWarn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    vi.stubGlobal('navigator', {})

    const worker = new PluginWorker(
      global as unknown as DedicatedWorkerGlobalScope,
      startPlugin,
      null,
    )
    const postMessage = vi.spyOn(worker.webDocumentTracker, 'postMessage')
    global.dispatchMessage({
      initData: new TextEncoder().encode(btoa('{}')),
    })

    await vi.runAllTimersAsync()
    await Promise.resolve()

    expect(startPlugin).not.toHaveBeenCalled()
    expect(postMessage).toHaveBeenCalledWith({
      from: 'plugin/spacewave-core',
      startupMark: expect.objectContaining({
        label: 'worker.init-message-received',
      }),
    })
    expect(global.close).toHaveBeenCalledTimes(1)
    expect(consoleWarn).toHaveBeenCalledWith(
      'PluginWorker: plugin/spacewave-core: startup canceled because WebDocument closed',
    )
  })

  test('reports one runtime failure close before idempotent shutdown', async () => {
    vi.useFakeTimers()
    const global = new FakeDedicatedWorkerGlobal()
    vi.stubGlobal('navigator', {})
    const worker = new PluginWorker(
      global as unknown as DedicatedWorkerGlobalScope,
      vi.fn(),
      null,
    )
    const postMessage = vi.spyOn(worker.webDocumentTracker, 'postMessage')

    const firstFailure = worker.reportRuntimeFailure(
      new Error('fatal wasm exit'),
    )
    const secondFailure = worker.reportRuntimeFailure(
      new Error('second fatal wasm exit'),
    )

    expect(postMessage).toHaveBeenCalledWith({
      from: 'plugin/spacewave-core',
      close: true,
      failureReason: 'fatal wasm exit',
    })
    expect(
      postMessage.mock.calls.filter(([msg]) => msg.failureReason),
    ).toHaveLength(1)

    await vi.runAllTimersAsync()
    await Promise.all([firstFailure, secondFailure])

    expect(global.close).toHaveBeenCalledTimes(1)
    expect(
      postMessage.mock.calls.filter(([msg]) => msg.failureReason),
    ).toHaveLength(1)
  })

  test('installs OPFS bridge global when hosted in a SharedWorker under config C', async () => {
    vi.stubGlobal('SharedWorkerGlobalScope', FakeSharedWorkerGlobalScope)
    const global = new FakeSharedWorkerGlobal()
    vi.stubGlobal('navigator', {})
    const opfsChannel = new MessageChannel()
    const refreshedOpfsChannel = new MessageChannel()
    const installRemoteDriver = vi.fn()
    const globals = globalThis as typeof globalThis & {
      __spacewaveOpfsBridgePort?: {
        request: (op: string, args: unknown) => Promise<unknown>
        close: () => void
      }
      __spacewaveInstallOpfsRemoteDriver?: (port: {
        request: (op: string, args: unknown) => Promise<unknown>
        close: () => void
      }) => boolean
    }
    globals.__spacewaveInstallOpfsRemoteDriver = installRemoteDriver
    const startPlugin = vi.fn()
    const started = new Promise<void>((resolve) => {
      startPlugin.mockImplementation(async () => {
        resolve()
      })
    })
    const worker = new PluginWorker(
      global as unknown as SharedWorkerGlobalScope,
      startPlugin,
      null,
    )
    vi.spyOn(worker.webDocumentTracker, 'waitConn').mockResolvedValue(undefined)
    vi.spyOn(
      worker.webDocumentTracker,
      'requestWebRtcBridge',
    ).mockResolvedValue(null)
    vi.spyOn(worker.webDocumentTracker, 'requestOpfsWorker')
      .mockResolvedValueOnce(opfsChannel.port1)
      .mockResolvedValueOnce(refreshedOpfsChannel.port1)

    // A cross-origin-isolated config C page still hosts the Go runtime in a
    // SharedWorker, where direct OPFS is denied, so the bridge must activate.
    const initChannel = new MessageChannel()
    global.dispatchConnect(initChannel.port2)
    initChannel.port1.postMessage({
      initData: new TextEncoder().encode(btoa('{}')),
      workerCommsDetect: {
        config: 'C',
        caps: {
          crossOriginIsolated: true,
          sabAvailable: true,
          opfsAvailable: true,
          webLocksAvailable: true,
          broadcastChannelAvailable: true,
        },
      },
    })

    await started

    expect(worker.webDocumentTracker.requestOpfsWorker).toHaveBeenCalledOnce()
    const opfsBridge = globals.__spacewaveOpfsBridgePort
    expect(opfsBridge).toBeDefined()
    expect(opfsBridge).not.toBe(opfsChannel.port1)
    expect(installRemoteDriver.mock.calls[0]?.[0]).toBe(opfsBridge)

    const nextRequest = new Promise<MessageEvent<unknown>>((resolve) => {
      opfsChannel.port2.addEventListener(
        'message',
        (event) => resolve(event as MessageEvent<unknown>),
        { once: true },
      )
      opfsChannel.port2.start()
    })
    const getRoot = opfsBridge!.request('getRoot', { ready: true })
    const request = await nextRequest
    expect(request.data).toMatchObject({ op: 'getRoot', args: { ready: true } })
    opfsChannel.port2.postMessage({
      id: (request.data as { id: number }).id,
      ok: true,
      result: { id: 1 },
    })
    await expect(getRoot).resolves.toEqual({ id: 1 })

    ;(
      worker as unknown as {
        refreshOpfsBridge: () => void
      }
    ).refreshOpfsBridge()
    await Promise.resolve()
    await Promise.resolve()

    expect(worker.webDocumentTracker.requestOpfsWorker).toHaveBeenCalledTimes(2)
    const refreshedBridge = globals.__spacewaveOpfsBridgePort
    expect(refreshedBridge).toBeDefined()
    expect(refreshedBridge).not.toBe(refreshedOpfsChannel.port1)
    expect(refreshedBridge).not.toBe(opfsBridge)
    expect(installRemoteDriver.mock.calls[1]?.[0]).toBe(refreshedBridge)
    opfsBridge?.close()
    refreshedBridge?.close()
    opfsChannel.port1.close()
    opfsChannel.port2.close()
    refreshedOpfsChannel.port1.close()
    refreshedOpfsChannel.port2.close()
    delete globals.__spacewaveOpfsBridgePort
    delete globals.__spacewaveInstallOpfsRemoteDriver
  })

  test('does not install an OPFS bridge in a DedicatedWorker host even under config A', async () => {
    const global = new FakeDedicatedWorkerGlobal()
    vi.stubGlobal('navigator', {})
    const globals = globalThis as typeof globalThis & {
      __spacewaveOpfsBridgePort?: { close: () => void }
      __spacewaveInstallOpfsRemoteDriver?: (port: unknown) => boolean
    }
    const installRemoteDriver = vi.fn()
    globals.__spacewaveInstallOpfsRemoteDriver = installRemoteDriver
    const startPlugin = vi.fn()
    const started = new Promise<void>((resolve) => {
      startPlugin.mockImplementation(async () => {
        resolve()
      })
    })
    const worker = new PluginWorker(
      global as unknown as DedicatedWorkerGlobalScope,
      startPlugin,
      null,
    )
    vi.spyOn(worker.webDocumentTracker, 'waitConn').mockResolvedValue(undefined)
    vi.spyOn(
      worker.webDocumentTracker,
      'requestWebRtcBridge',
    ).mockResolvedValue(null)
    const requestOpfsWorker = vi.spyOn(
      worker.webDocumentTracker,
      'requestOpfsWorker',
    )

    global.dispatchMessage({
      initData: new TextEncoder().encode(btoa('{}')),
      workerCommsDetect: {
        config: 'A',
        caps: {
          crossOriginIsolated: false,
          sabAvailable: false,
          opfsAvailable: false,
          webLocksAvailable: true,
          broadcastChannelAvailable: true,
        },
      },
    })

    await started

    expect(requestOpfsWorker).not.toHaveBeenCalled()
    expect(globals.__spacewaveOpfsBridgePort).toBeUndefined()
    expect(installRemoteDriver).not.toHaveBeenCalled()
    delete globals.__spacewaveInstallOpfsRemoteDriver
  })
})

class FakeSharedWorkerGlobalScope {}

class FakeSharedWorkerGlobal extends FakeSharedWorkerGlobalScope {
  public readonly name = 'plugin/spacewave-core?s=/b/pd/core.mjs&t=wasm&p=1'
  public readonly close = vi.fn()
  private connectHandler?: (ev: MessageEvent) => void

  public addEventListener(type: string, handler: EventListener): void {
    if (type === 'connect') {
      this.connectHandler = handler as (ev: MessageEvent) => void
    }
  }

  public dispatchConnect(port: MessagePort): void {
    this.connectHandler?.({ ports: [port] } as unknown as MessageEvent)
  }
}

class FakeDedicatedWorkerGlobal {
  public readonly name = 'plugin/spacewave-core?s=/b/pd/core.mjs&t=wasm&p=1'
  public readonly close = vi.fn()
  private messageHandler?: (ev: MessageEvent) => void

  public addEventListener(type: string, handler: EventListener): void {
    if (type === 'message') {
      this.messageHandler = handler as (ev: MessageEvent) => void
    }
  }

  public dispatchMessage(data: unknown): void {
    this.messageHandler?.({ data } as MessageEvent)
  }
}
