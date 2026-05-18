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

    new PluginWorker(
      global as unknown as DedicatedWorkerGlobalScope,
      startPlugin,
      null,
    )
    global.dispatchMessage({
      initData: new TextEncoder().encode(btoa('{}')),
    })

    await vi.runAllTimersAsync()
    await Promise.resolve()

    expect(startPlugin).not.toHaveBeenCalled()
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
})

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
