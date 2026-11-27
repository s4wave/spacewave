import { afterEach, describe, expect, it, vi } from 'vitest'

import { WebRuntimeClientType } from '../runtime/runtime.pb.js'

import {
  DedicatedWorkerHostOwner,
  buildDedicatedWorkerHostLockName,
} from './dedicated-worker-host-owner.js'
import { buildRuntimeLifeLockName } from './runtime-life-lock.js'
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
      held: [...this.held.values()].map(
        (entry) =>
          ({
            name: entry.name,
            mode: 'exclusive',
            clientId: 'test-holder',
          }) satisfies LockInfo,
      ),
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

  private readonly held = new Map<string, TestLockRequest>()
  private pending: TestLockRequest[] = []

  // hold takes the named lock the way a runtime Worker takes its life lock and
  // returns the Worker's death, which releases it.
  public hold(name: string): () => void {
    const { promise, resolve } = Promise.withResolvers<void>()
    void this.request(name, {}, () => promise)
    return resolve
  }

  private drain(): void {
    const entry = this.pending.find((item) => !this.held.has(item.name))
    if (!entry) {
      return
    }
    this.pending = this.pending.filter((item) => item !== entry)
    entry.releaseAbort?.()
    this.held.set(entry.name, entry)
    const release = () => {
      if (this.held.get(entry.name) === entry) {
        this.held.delete(entry.name)
      }
    }
    Promise.resolve(entry.callback({} as Lock)).then(
      () => {
        release()
        entry.resolve()
        this.drain()
      },
      (err: unknown) => {
        release()
        entry.reject(err)
        this.drain()
      },
    )
    this.drain()
  }
}

function buildCallbacks() {
  return {
    startHost: vi.fn(),
    startAttached: vi.fn(),
    startUnavailable: vi.fn(),
    promoteToHost: vi.fn(),
    hostWorkerLost: vi.fn(),
    attachedHostGone: vi.fn(),
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

  it('a host fails its term only when its own Worker dies', async () => {
    const locks = new TestLockManager()
    vi.stubGlobal('navigator', { locks })
    const owner = new DedicatedWorkerHostOwner('runtime-1', 'document-1')
    const callbacks = buildCallbacks()
    owner.start(callbacks)
    await flushPromises()
    const lifeLockName = owner.lifeLockName
    expect(lifeLockName).toBe(
      buildRuntimeLifeLockName('runtime-1', owner.generation ?? ''),
    )

    const workerDies = locks.hold(lifeLockName ?? '')
    await flushPromises()
    owner.workerReportsLifeLock(lifeLockName ?? '', true)
    await owner.waitWorkerHoldsLifeLock()
    await flushPromises()
    expect(callbacks.hostWorkerLost).not.toHaveBeenCalled()

    workerDies()
    await flushPromises()
    await flushPromises()
    expect(callbacks.hostWorkerLost).toHaveBeenCalledExactlyOnceWith(
      'runtime worker exited',
    )

    // A term that ended first ignores its Worker's end: the host terminates
    // the Worker itself after a reported failure.
    const failed = new DedicatedWorkerHostOwner('runtime-2', 'document-2')
    const failedCallbacks = buildCallbacks()
    failed.start(failedCallbacks)
    await flushPromises()
    const failedLockName = failed.lifeLockName ?? ''
    const failedWorkerDies = locks.hold(failedLockName)
    await flushPromises()
    failed.workerReportsLifeLock(failedLockName, true)
    failed.fail('panic: boom')
    failedWorkerDies()
    await flushPromises()
    await flushPromises()
    expect(failedCallbacks.hostWorkerLost).not.toHaveBeenCalled()

    failed.close()
    owner.close()
  })

  it('host whose Worker holds no life lock fails its term', async () => {
    const locks = new TestLockManager()
    vi.stubGlobal('navigator', { locks })
    const owner = new DedicatedWorkerHostOwner('runtime-1', 'document-1')
    const callbacks = buildCallbacks()
    owner.start(callbacks)
    await flushPromises()
    const lifeLockName = owner.lifeLockName ?? ''
    const acked = vi.fn()
    void owner.waitWorkerHoldsLifeLock().then(acked, () => {})

    owner.workerReportsLifeLock(lifeLockName, false)
    await flushPromises()
    // No attach is acked and no watch is requested on a lock nobody holds.
    expect(acked).not.toHaveBeenCalled()
    expect(locks.request.mock.calls.map(([name]) => name)).not.toContain(
      lifeLockName,
    )
    expect(callbacks.hostWorkerLost).toHaveBeenCalledExactlyOnceWith(
      'runtime worker holds no life lock',
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

  it('a failed runtime ends every term so the next document hosts fresh', async () => {
    const locks = new TestLockManager()
    vi.stubGlobal('navigator', { locks })
    const host = new DedicatedWorkerHostOwner('runtime-1', 'document-1')
    host.start(buildCallbacks())
    await flushPromises()
    const attached = new DedicatedWorkerHostOwner('runtime-1', 'document-2')
    const attachedCallbacks = buildCallbacks()
    attached.start(attachedCallbacks)
    await flushPromises()

    // The attached document learns of the failure first and withdraws, so the
    // host's release cannot promote a document that shows the failure.
    attached.fail('panic: boom')
    host.fail('panic: boom')
    await flushPromises()

    expect(host.role).toBe('failed')
    expect(attached.role).toBe('failed')
    expect(attachedCallbacks.promoteToHost).not.toHaveBeenCalled()

    const reloaded = new DedicatedWorkerHostOwner('runtime-1', 'document-3')
    const reloadedCallbacks = buildCallbacks()
    reloaded.start(reloadedCallbacks)
    await flushPromises()

    expect(reloaded.role).toBe('host')
    expect(reloadedCallbacks.startHost).toHaveBeenCalledOnce()

    reloaded.close()
    attached.close()
    host.close()
  })

  it('cancels a pending attached relay when the document becomes host', async () => {
    const locks = new TestLockManager()
    const host = new DedicatedWorkerHostOwner('runtime-1', 'document-1')
    vi.stubGlobal('navigator', { locks })
    host.start(buildCallbacks())
    await flushPromises()
    const cancel = Promise.withResolvers<unknown>()
    const postMessage = vi.fn(
      (message: unknown, _transfer?: Transferable[]) => {
        const requestMessage = message as {
          connectDedicatedRuntimeHost?: { port: MessagePort }
        }
        const port = requestMessage.connectDedicatedRuntimeHost?.port
        if (!port) {
          throw new Error('dedicated runtime host request port missing')
        }
        port.onmessage = (ev: MessageEvent) => cancel.resolve(ev.data)
        port.start()
      },
    )
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
    const postMessage = vi.fn(
      (message: unknown, _transfer?: Transferable[]) => {
        const request = (message as Record<string, unknown>)
          .connectDedicatedRuntimeHost as { port: MessagePort }
        request.port.postMessage(
          {
            from: 'service-worker-test',
            hostDocumentId: 'document-1',
            hostGeneration: 'document-1-generation-1',
            webRuntimePort: runtimeChannel.port1,
          },
          [runtimeChannel.port1],
        )
      },
    )
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
    // The host acks only once its Worker holds the term's life lock.
    const hostWorkerDies = locks.hold(
      buildRuntimeLifeLockName('runtime-1', 'document-1-generation-1'),
    )
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
    expect(callbacks.attachedHostGone).not.toHaveBeenCalled()

    // The host's Worker dies with the host page alive: the attached document
    // learns it from the life lock and reroutes once, whichever signal
    // reports the loss first.
    hostWorkerDies()
    await flushPromises()
    await flushPromises()
    expect(callbacks.attachedHostGone).toHaveBeenCalledExactlyOnceWith(
      'document-1',
      'document-1-generation-1',
    )
    expect(owner.hostLost('document-1', 'document-1-generation-1')).toBe(true)
    expect(owner.hostLost('document-1', 'document-1-generation-1')).toBe(false)

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
    const postMessage = vi.fn(
      (message: unknown, _transfer?: Transferable[]) => {
        const request = (message as Record<string, unknown>)
          .connectDedicatedRuntimeHost as { port: MessagePort }
        request.port.postMessage({
          from: 'service-worker-test',
          error: 'no elected DedicatedWorker runtime host available',
        })
      },
    )
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
