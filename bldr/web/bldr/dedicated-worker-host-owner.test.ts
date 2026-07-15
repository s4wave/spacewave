import { afterEach, describe, expect, it, vi } from 'vitest'

import { WebRuntimeClientType } from '../runtime/runtime.pb.js'

import {
  DedicatedWorkerHostOwner,
  buildDedicatedWorkerHostLockName,
} from './dedicated-worker-host-owner.js'
import { resetStartupMarksForTest } from './startup-marks.js'

type TestLockOptions = {
  signal?: AbortSignal
  ifAvailable?: boolean
}

type TestLockRequest = {
  name: string
  opts: TestLockOptions
  callback: (lock: Lock) => Promise<void> | void
  resolve: () => void
  reject: (err: unknown) => void
  releaseAbort?: () => void
}

class TestLockManager {
  public readonly request = vi.fn(
    (
      name: string,
      opts: TestLockOptions,
      callback: (lock: Lock) => Promise<void> | void,
    ) =>
      new Promise<void>((resolve, reject) => {
        const entry: TestLockRequest = {
          name,
          opts,
          callback,
          resolve,
          reject,
        }
        if (opts.signal?.aborted) {
          reject(new DOMException('Lock request aborted', 'AbortError'))
          return
        }
        const onAbort = () => {
          this.pending = this.pending.filter((item) => item !== entry)
          reject(new DOMException('Lock request aborted', 'AbortError'))
        }
        opts.signal?.addEventListener('abort', onAbort, { once: true })
        entry.releaseAbort = () => {
          opts.signal?.removeEventListener('abort', onAbort)
        }
        this.pending.push(entry)
        this.drain()
      }),
  )

  public readonly query = vi.fn(() =>
    Promise.resolve({
      held: this.held
        ? [
            {
              name: this.held.name,
              mode: 'exclusive',
              clientId: 'test-holder',
            } satisfies LockInfo,
          ]
        : [],
      pending: this.pending.map(
        (entry) =>
          ({
            name: entry.name,
            mode: 'exclusive',
            clientId: 'test-pending',
          }) satisfies LockInfo,
      ),
    } satisfies LockManagerSnapshot),
  )

  private held?: TestLockRequest
  private pending: TestLockRequest[] = []

  private drain(): void {
    if (this.held || this.pending.length === 0) {
      return
    }
    const entry = this.pending.shift()
    if (!entry) {
      return
    }
    entry.releaseAbort?.()
    this.held = entry
    Promise.resolve(entry.callback({} as Lock)).then(
      () => {
        if (this.held === entry) {
          this.held = undefined
        }
        entry.resolve()
        this.drain()
      },
      (err: unknown) => {
        if (this.held === entry) {
          this.held = undefined
        }
        entry.reject(err)
        this.drain()
      },
    )
  }
}

function buildCallbacks() {
  return {
    startHost: vi.fn(),
    startAttached: vi.fn(),
    startUnavailable: vi.fn(),
    promoteToHost: vi.fn(),
  }
}

async function flushPromises(): Promise<void> {
  await Promise.resolve()
  await Promise.resolve()
}

describe('DedicatedWorkerHostOwner', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    resetStartupMarksForTest()
  })

  it('first standing lock grant becomes host', async () => {
    const locks = new TestLockManager()
    vi.stubGlobal('navigator', { locks })
    const owner = new DedicatedWorkerHostOwner('runtime-1', 'document-1')
    const callbacks = buildCallbacks()

    owner.start(callbacks)
    await flushPromises()

    expect(locks.request).toHaveBeenCalledWith(
      buildDedicatedWorkerHostLockName('runtime-1'),
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
      expect.any(Function),
    )
    expect(locks.request.mock.calls[0][1]).not.toHaveProperty('ifAvailable')
    expect(owner.role).toBe('host')
    expect(owner.generation).toMatch(/^document-1-/)
    expect(callbacks.startHost).toHaveBeenCalledOnce()
    expect(callbacks.startHost).toHaveBeenCalledWith(owner.generation)
    expect(callbacks.startAttached).not.toHaveBeenCalled()
    expect(callbacks.startUnavailable).not.toHaveBeenCalled()
    expect(callbacks.promoteToHost).not.toHaveBeenCalled()
    expect(globalThis.__swStartupMarks?.map((mark) => mark.label)).toContain(
      'dedicated-host.lease-acquired',
    )

    owner.close()
  })

  it('second tab attaches while host lock is held', async () => {
    const locks = new TestLockManager()
    vi.stubGlobal('navigator', { locks })
    const host = new DedicatedWorkerHostOwner('runtime-1', 'document-1')
    host.start(buildCallbacks())
    await flushPromises()
    const attached = new DedicatedWorkerHostOwner('runtime-1', 'document-2')
    const attachedCallbacks = buildCallbacks()

    attached.start(attachedCallbacks)
    await flushPromises()

    expect(locks.query).toHaveBeenCalled()
    expect(attached.role).toBe('attached')
    expect(attachedCallbacks.startAttached).toHaveBeenCalledOnce()
    expect(attachedCallbacks.startHost).not.toHaveBeenCalled()
    expect(attachedCallbacks.promoteToHost).not.toHaveBeenCalled()

    attached.close()
    host.close()
  })

  it('standing request promotes attached tab on host release', async () => {
    const locks = new TestLockManager()
    vi.stubGlobal('navigator', { locks })
    const host = new DedicatedWorkerHostOwner('runtime-1', 'document-1')
    host.start(buildCallbacks())
    await flushPromises()
    const attached = new DedicatedWorkerHostOwner('runtime-1', 'document-2')
    const attachedCallbacks = buildCallbacks()
    attached.start(attachedCallbacks)
    await flushPromises()

    host.close()
    await flushPromises()

    expect(attached.role).toBe('host')
    expect(attached.generation).toMatch(/^document-2-/)
    expect(attachedCallbacks.promoteToHost).toHaveBeenCalledOnce()
    expect(attachedCallbacks.startHost).not.toHaveBeenCalled()
    expect(globalThis.__swStartupMarks?.map((mark) => mark.label)).toContain(
      'dedicated-host.promoted',
    )

    attached.close()
  })

  it('cancels a pending attached relay when the document becomes host', async () => {
    const locks = new TestLockManager()
    const host = new DedicatedWorkerHostOwner('runtime-1', 'document-1')
    vi.stubGlobal('navigator', { locks })
    host.start(buildCallbacks())
    await flushPromises()
    const cancel = Promise.withResolvers<unknown>()
    const postMessage = vi.fn((message: unknown, _transfer?: Transferable[]) => {
      const requestMessage = message as {
        connectDedicatedRuntimeHost?: { port: MessagePort }
      }
      const port = requestMessage.connectDedicatedRuntimeHost?.port
      if (!port) {
        throw new Error('dedicated runtime host request port missing')
      }
      port.onmessage = (ev: MessageEvent) => cancel.resolve(ev.data)
      port.start()
    })
    vi.stubGlobal('navigator', {
      locks,
      serviceWorker: {
        controller: {
          postMessage,
        },
      },
    })
    const owner = new DedicatedWorkerHostOwner('runtime-1', 'document-2')
    const callbacks = buildCallbacks()
    owner.start(callbacks)
    await flushPromises()

    const opening = owner.openClientChannel({
      webRuntimeId: 'runtime-1',
      clientUuid: 'document-2',
      clientType: WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT,
    })
    const openingFailure = expect(opening).rejects.toThrow(
      'attached runtime host relay promoted to host',
    )
    await vi.waitFor(() => {
      expect(postMessage).toHaveBeenCalledOnce()
    })
    host.close()
    await flushPromises()

    await expect(cancel.promise).resolves.toEqual({
      cancelDedicatedRuntimeHostConnect: true,
    })
    await openingFailure
    expect(owner.role).toBe('host')
    expect(callbacks.promoteToHost).toHaveBeenCalledOnce()

    owner.close()
  })

  it('close while pending aborts standing request', async () => {
    const locks = new TestLockManager()
    vi.stubGlobal('navigator', { locks })
    const host = new DedicatedWorkerHostOwner('runtime-1', 'document-1')
    host.start(buildCallbacks())
    await flushPromises()
    const pending = new DedicatedWorkerHostOwner('runtime-1', 'document-2')
    const pendingCallbacks = buildCallbacks()

    pending.start(pendingCallbacks)
    pending.close()
    host.close()
    await flushPromises()

    expect(pending.role).toBe('closed')
    expect(pendingCallbacks.startAttached).not.toHaveBeenCalled()
    expect(pendingCallbacks.startHost).not.toHaveBeenCalled()
    expect(pendingCallbacks.promoteToHost).not.toHaveBeenCalled()
  })

  it('keeps no-Web-Locks fallback functional while marking the hazard', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    vi.stubGlobal('navigator', {})
    const owner = new DedicatedWorkerHostOwner('runtime-1', 'document-1')
    const callbacks = buildCallbacks()

    owner.start(callbacks)

    expect(owner.role).toBe('unavailable')
    expect(callbacks.startUnavailable).toHaveBeenCalledOnce()
    expect(warn).toHaveBeenCalledWith(
      expect.stringContaining('Web Locks unavailable'),
    )
    const unavailable = globalThis.__swStartupMarks?.find(
      (mark) => mark.label === 'dedicated-host.election-unavailable',
    )
    expect(unavailable?.detail).toMatchObject({
      hazard: expect.stringContaining('multiple OPFS writers'),
    })
  })

  it('attaches to the elected host through the ServiceWorker relay', async () => {
    const locks = new TestLockManager()
    const host = new DedicatedWorkerHostOwner('runtime-1', 'document-1')
    vi.stubGlobal('navigator', { locks })
    host.start(buildCallbacks())
    await flushPromises()
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
      locks,
      serviceWorker: {
        controller: {
          postMessage,
        },
      },
    })
    const owner = new DedicatedWorkerHostOwner('runtime-1', 'document-2')
    owner.start(buildCallbacks())
    await flushPromises()

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
    host.close()
  })

  it('leaves election state alone when an attached relay has no live host', async () => {
    const locks = new TestLockManager()
    const host = new DedicatedWorkerHostOwner('runtime-1', 'document-1')
    vi.stubGlobal('navigator', { locks })
    host.start(buildCallbacks())
    await flushPromises()
    const postMessage = vi.fn((message: unknown, _transfer?: Transferable[]) => {
      const request = (message as Record<string, unknown>)
        .connectDedicatedRuntimeHost as { port: MessagePort }
      request.port.postMessage({
        from: 'service-worker-test',
        error: 'no elected DedicatedWorker runtime host available',
      })
    })
    vi.stubGlobal('navigator', {
      locks,
      serviceWorker: {
        controller: {
          postMessage,
        },
      },
    })
    const owner = new DedicatedWorkerHostOwner('runtime-1', 'document-2')
    const callbacks = buildCallbacks()
    owner.start(callbacks)
    await flushPromises()

    await expect(
      owner.openClientChannel({
        webRuntimeId: 'runtime-1',
        clientUuid: 'document-2',
        clientType: WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT,
      }),
    ).rejects.toThrow('no elected DedicatedWorker runtime host available')

    expect(owner.role).toBe('attached')
    expect(callbacks.startHost).not.toHaveBeenCalled()
    expect(locks.request).toHaveBeenCalledTimes(2)
    expect(globalThis.__swStartupMarks?.map((mark) => mark.label)).toContain(
      'dedicated-host.attach-open-failed',
    )

    owner.close()
    host.close()
  })
})
