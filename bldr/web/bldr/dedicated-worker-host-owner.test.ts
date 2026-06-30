import { afterEach, describe, expect, it, vi } from 'vitest'

import { WebRuntimeClientType } from '../runtime/runtime.pb.js'

import {
  DedicatedWorkerHostOwner,
  buildDedicatedWorkerHostLockName,
} from './dedicated-worker-host-owner.js'
import { resetStartupMarksForTest } from './startup-marks.js'

describe('DedicatedWorkerHostOwner', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    resetStartupMarksForTest()
  })

  it('acquires and holds the runtime host lease as one generation', () => {
    const lockRequest = vi.fn(
      (
        _name: string,
        _opts: { ifAvailable?: boolean; signal?: AbortSignal },
        callback: (lock: Lock | null) => Promise<void> | void,
      ) => callback({} as Lock),
    )
    vi.stubGlobal('navigator', {
      locks: {
        request: lockRequest,
      },
    })
    const owner = new DedicatedWorkerHostOwner('runtime-1', 'document-1')
    const startHost = vi.fn()
    const startAttached = vi.fn()
    const startUnavailable = vi.fn()

    owner.start({ startHost, startAttached, startUnavailable })

    expect(lockRequest).toHaveBeenCalledWith(
      buildDedicatedWorkerHostLockName('runtime-1'),
      expect.objectContaining({ ifAvailable: true }),
      expect.any(Function),
    )
    expect(owner.role).toBe('host')
    expect(owner.generation).toMatch(/^document-1-/)
    expect(startHost).toHaveBeenCalledWith(owner.generation)
    expect(startAttached).not.toHaveBeenCalled()
    expect(startUnavailable).not.toHaveBeenCalled()
    expect(globalThis.__swStartupMarks?.map((mark) => mark.label)).toContain(
      'dedicated-host.lease-acquired',
    )

    owner.close()
    expect(owner.role).toBe('closed')
  })

  it('attaches to the elected host through the ServiceWorker relay', async () => {
    const lockRequest = vi.fn(
      (
        _name: string,
        _opts: { ifAvailable?: boolean; signal?: AbortSignal },
        callback: (lock: Lock | null) => Promise<void> | void,
      ) => callback(null),
    )
    const runtimeChannel = new MessageChannel()
    const postMessage = vi.fn((message: unknown, _transfer?: Transferable[]) => {
      const request = (message as Record<string, unknown>)
        .connectDedicatedRuntimeHost as { port: MessagePort }
      request.port.postMessage(
        {
          from: 'service-worker-test',
          webRuntimePort: runtimeChannel.port1,
        },
        [runtimeChannel.port1],
      )
    })
    vi.stubGlobal('navigator', {
      locks: {
        request: lockRequest,
      },
      serviceWorker: {
        controller: {
          postMessage,
        },
      },
    })
    const owner = new DedicatedWorkerHostOwner('runtime-1', 'document-2')
    owner.start({
      startHost: vi.fn(),
      startAttached: vi.fn(),
      startUnavailable: vi.fn(),
    })

    const openedPort = await owner.openClientChannel({
      webRuntimeId: 'runtime-1',
      clientUuid: 'document-2',
      clientType: WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT,
    })

    expect(owner.role).toBe('attached')
    expect(postMessage).toHaveBeenCalledOnce()
    expect(postMessage.mock.calls[0][1]).toHaveLength(1)
    const delivered = new Promise<unknown>((resolve) => {
      runtimeChannel.port2.onmessage = (ev: MessageEvent) => resolve(ev.data)
      runtimeChannel.port2.start()
    })
    openedPort.postMessage({ attached: true })
    await expect(delivered).resolves.toEqual({ attached: true })
    expect(globalThis.__swStartupMarks?.map((mark) => mark.label)).toEqual(
      expect.arrayContaining([
        'dedicated-host.attach-selected',
        'dedicated-host.attach-open-ready',
      ]),
    )

    openedPort.close()
    runtimeChannel.port2.close()
    owner.close()
  })

  it('restarts election when an attached relay has no live host', async () => {
    let lockAttempt = 0
    const lockRequest = vi.fn(
      (
        _name: string,
        _opts: { ifAvailable?: boolean; signal?: AbortSignal },
        callback: (lock: Lock | null) => Promise<void> | void,
      ) => {
        lockAttempt++
        return callback(lockAttempt === 1 ? null : ({} as Lock))
      },
    )
    const postMessage = vi.fn((message: unknown, _transfer?: Transferable[]) => {
      const request = (message as Record<string, unknown>)
        .connectDedicatedRuntimeHost as { port: MessagePort }
      request.port.postMessage({
        from: 'service-worker-test',
        error: 'no elected DedicatedWorker runtime host available',
      })
    })
    vi.stubGlobal('navigator', {
      locks: {
        request: lockRequest,
      },
      serviceWorker: {
        controller: {
          postMessage,
        },
      },
    })
    const owner = new DedicatedWorkerHostOwner('runtime-1', 'document-2')
    const startHost = vi.fn()
    owner.start({
      startHost,
      startAttached: vi.fn(),
      startUnavailable: vi.fn(),
    })

    await expect(
      owner.openClientChannel({
        webRuntimeId: 'runtime-1',
        clientUuid: 'document-2',
        clientType: WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT,
      }),
    ).rejects.toThrow('no elected DedicatedWorker runtime host available')

    expect(owner.role).toBe('host')
    expect(startHost).toHaveBeenCalledWith(owner.generation)
    expect(lockRequest).toHaveBeenCalledTimes(2)
    expect(globalThis.__swStartupMarks?.map((mark) => mark.label)).toEqual(
      expect.arrayContaining([
        'dedicated-host.attach-lost',
        'dedicated-host.lease-acquired',
      ]),
    )

    owner.close()
  })
})
