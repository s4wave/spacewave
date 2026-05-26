import { afterEach, describe, expect, it, vi } from 'vitest'

import type { ClientToWebDocument } from '../runtime/runtime.js'
import { WebRuntimeClientType } from '../runtime/runtime.pb.js'
import { WebDocumentTracker } from './web-document-tracker.js'

function buildTracker(): WebDocumentTracker {
  return new WebDocumentTracker(
    'tracker-client',
    WebRuntimeClientType.WebRuntimeClientType_WEB_WORKER,
    vi.fn().mockResolvedValue(undefined),
    null,
  )
}

function waitForActiveWebDocumentResumeReady(
  tracker: WebDocumentTracker,
): Promise<void> {
  const waitForResumeReady = Reflect.get(
    tracker,
    'waitForActiveWebDocumentResumeReady',
  ) as (this: WebDocumentTracker) => Promise<void>
  return waitForResumeReady.call(tracker)
}

function attachWebDocument(
  tracker: WebDocumentTracker,
  webDocumentId = 'document-1',
): MessagePort {
  const { port1, port2 } = new MessageChannel()
  tracker.handleWebDocumentMessage({
    from: webDocumentId,
    initPort: port1,
  })
  return port2
}

function markSettled(promise: Promise<unknown>): () => boolean {
  let settled = false
  promise.then(
    () => {
      settled = true
    },
    () => {
      settled = true
    },
  )
  return () => settled
}

function installControllableWebLock(): { release(): void } {
  let releaseLock: (() => void) | undefined
  vi.stubGlobal('navigator', {
    locks: {
      request: vi.fn(
        (
          _name: string,
          opts: { signal?: AbortSignal },
          cb: () => Error | undefined,
        ) =>
          new Promise<Error | undefined>((resolve, reject) => {
            const abort = () => {
              reject(new DOMException('aborted', 'AbortError'))
            }
            opts.signal?.addEventListener('abort', abort, { once: true })
            releaseLock = () => {
              opts.signal?.removeEventListener('abort', abort)
              resolve(cb())
            }
          }),
      ),
    },
  })
  return {
    release() {
      releaseLock?.()
    },
  }
}

describe('WebDocumentTracker resume-ready gate', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  it('reports resume unavailable when there is no active WebDocument', async () => {
    const tracker = buildTracker()

    await expect(
      waitForActiveWebDocumentResumeReady(tracker),
    ).resolves.toMatchObject({
      state: 'unavailable',
      reason: 'no active WebDocument',
    })

    tracker.close()
  })

  it('resolves the active document resume-ready gate from the WebDocument port', async () => {
    const tracker = buildTracker()
    const { port1, port2 } = new MessageChannel()
    tracker.handleWebDocumentMessage({
      from: 'document-1',
      initPort: port1,
    })
    Reflect.set(tracker, 'lastWebDocumentId', 'document-1')

    const readyPromise = waitForActiveWebDocumentResumeReady(tracker)
    let resolved = false
    readyPromise.then(() => {
      resolved = true
    })
    await Promise.resolve()
    expect(resolved).toBe(false)

    port2.postMessage({
      from: 'document-1',
      resumeReady: true,
    })

    await expect(readyPromise).resolves.toMatchObject({
      state: 'ready',
      documentId: 'document-1',
    })
    expect(resolved).toBe(true)

    tracker.close()
    port2.close()
  })

  it('reports the resume-ready gate closed when the active WebDocument closes', async () => {
    const tracker = buildTracker()
    const { port1, port2 } = new MessageChannel()
    tracker.handleWebDocumentMessage({
      from: 'document-1',
      initPort: port1,
    })
    Reflect.set(tracker, 'lastWebDocumentId', 'document-1')

    const readyPromise = waitForActiveWebDocumentResumeReady(tracker)

    port2.postMessage({
      from: 'document-1',
      close: true,
    })

    await expect(readyPromise).resolves.toMatchObject({
      state: 'closed',
      documentId: 'document-1',
      reason: expect.stringContaining(
        'WebDocument document-1 closed before resume-ready',
      ),
    })

    tracker.close()
    port2.close()
  })

  it('waits again after the active WebDocument clears resume-ready', async () => {
    const tracker = buildTracker()
    const { port1, port2 } = new MessageChannel()
    tracker.handleWebDocumentMessage({
      from: 'document-1',
      initPort: port1,
    })
    Reflect.set(tracker, 'lastWebDocumentId', 'document-1')

    port2.postMessage({
      from: 'document-1',
      resumeReady: true,
    })
    await expect(
      waitForActiveWebDocumentResumeReady(tracker),
    ).resolves.toMatchObject({
      state: 'ready',
      documentId: 'document-1',
    })

    port2.postMessage({
      from: 'document-1',
      resumeReady: false,
    })
    await new Promise((resolve) => setTimeout(resolve, 0))

    const readyPromise = waitForActiveWebDocumentResumeReady(tracker)
    let resolved = false
    readyPromise.then(() => {
      resolved = true
    })
    await Promise.resolve()
    expect(resolved).toBe(false)

    port2.postMessage({
      from: 'document-1',
      resumeReady: true,
    })

    await expect(readyPromise).resolves.toMatchObject({
      state: 'ready',
      documentId: 'document-1',
    })

    tracker.close()
    port2.close()
  })

  it('moves the active gate to the latest document and only closes runtime after all documents close', async () => {
    const tracker = buildTracker()
    const closeRuntime = vi.spyOn(tracker.webRuntimeClient, 'close')
    const firstPort = attachWebDocument(tracker, 'document-1')

    firstPort.postMessage({
      from: 'document-1',
      resumeReady: true,
    })

    await expect(
      waitForActiveWebDocumentResumeReady(tracker),
    ).resolves.toMatchObject({
      state: 'ready',
      documentId: 'document-1',
    })

    const secondPort = attachWebDocument(tracker, 'document-2')
    const secondReady = waitForActiveWebDocumentResumeReady(tracker)
    const secondSettled = markSettled(secondReady)
    await Promise.resolve()
    expect(secondSettled()).toBe(false)

    secondPort.postMessage({
      from: 'document-2',
      resumeReady: true,
    })

    await expect(secondReady).resolves.toMatchObject({
      state: 'ready',
      documentId: 'document-2',
    })

    secondPort.postMessage({
      from: 'document-2',
      close: true,
    })
    await new Promise((resolve) => setTimeout(resolve, 0))

    expect(closeRuntime).not.toHaveBeenCalled()
    await expect(
      waitForActiveWebDocumentResumeReady(tracker),
    ).resolves.toMatchObject({
      state: 'ready',
      documentId: 'document-1',
    })

    firstPort.postMessage({
      from: 'document-1',
      close: true,
    })
    await new Promise((resolve) => setTimeout(resolve, 0))

    expect(closeRuntime).toHaveBeenCalledTimes(1)
    tracker.close()
    firstPort.close()
    secondPort.close()
  })

  it('keeps stream-open readiness on the active runtime relay instead of the newest document', async () => {
    const tracker = buildTracker()
    const firstPort = attachWebDocument(tracker, 'document-1')

    firstPort.postMessage({
      from: 'document-1',
      resumeReady: true,
    })
    await expect(
      waitForActiveWebDocumentResumeReady(tracker),
    ).resolves.toMatchObject({
      state: 'ready',
      documentId: 'document-1',
    })

    Reflect.set(tracker, 'activeRuntimeWebDocumentId', 'document-1')
    const secondPort = attachWebDocument(tracker, 'document-2')
    const readyPromise = waitForActiveWebDocumentResumeReady(tracker)

    await expect(readyPromise).resolves.toMatchObject({
      state: 'ready',
      documentId: 'document-1',
    })

    tracker.close()
    firstPort.close()
    secondPort.close()
  })

  it('reconnects the service worker runtime through a newly ready document', async () => {
    const tracker = new WebDocumentTracker(
      'service-worker',
      WebRuntimeClientType.WebRuntimeClientType_SERVICE_WORKER,
      vi.fn().mockResolvedValue(undefined),
      null,
    )
    const closeRuntime = vi.spyOn(tracker.webRuntimeClient, 'close')
    const firstPort = attachWebDocument(tracker, 'document-1')
    const secondPort = attachWebDocument(tracker, 'document-2')
    Reflect.set(tracker, 'activeRuntimeWebDocumentId', 'document-1')

    firstPort.start()
    secondPort.start()
    secondPort.postMessage({
      from: 'document-2',
      resumeReady: true,
    })
    await new Promise((resolve) => setTimeout(resolve, 0))

    expect(closeRuntime).toHaveBeenCalledTimes(1)
    expect(Reflect.get(tracker, 'activeRuntimeWebDocumentId')).toBeUndefined()
    expect(Reflect.get(tracker, 'preferredRuntimeWebDocumentId')).toBe(
      'document-2',
    )

    tracker.close()
    firstPort.close()
    secondPort.close()
  })

  it('closes the runtime client when the active relay document closes', async () => {
    const tracker = buildTracker()
    const closeRuntime = vi.spyOn(tracker.webRuntimeClient, 'close')
    const firstPort = attachWebDocument(tracker, 'document-1')
    const secondPort = attachWebDocument(tracker, 'document-2')

    Reflect.set(tracker, 'activeRuntimeWebDocumentId', 'document-1')

    firstPort.postMessage({
      from: 'document-1',
      close: true,
    })
    await new Promise((resolve) => setTimeout(resolve, 0))

    expect(closeRuntime).toHaveBeenCalledTimes(1)
    expect(Reflect.get(tracker, 'webDocuments')).not.toHaveProperty(
      'document-1',
    )
    expect(Reflect.get(tracker, 'webDocuments')).toHaveProperty('document-2')
    expect(Reflect.get(tracker, 'lastWebDocumentId')).toBe('document-2')

    tracker.close()
    firstPort.close()
    secondPort.close()
  })

  it('keeps plugin worker resume gate parked while the active WebDocument is hidden', async () => {
    vi.useFakeTimers()
    const onWebDocumentsExhausted = vi.fn().mockResolvedValue(undefined)
    const onAllWebDocumentsClosed = vi.fn()
    const tracker = new WebDocumentTracker(
      'plugin/spacewave-app',
      WebRuntimeClientType.WebRuntimeClientType_WEB_WORKER,
      onWebDocumentsExhausted,
      null,
      onAllWebDocumentsClosed,
    )
    const documentPort = attachWebDocument(tracker)

    try {
      Reflect.set(tracker, 'lastWebDocumentId', 'document-1')

      const readyPromise = waitForActiveWebDocumentResumeReady(tracker)
      const isSettled = markSettled(readyPromise)

      await vi.advanceTimersByTimeAsync(5000)

      expect(isSettled()).toBe(false)
      expect(onWebDocumentsExhausted).not.toHaveBeenCalled()
      expect(onAllWebDocumentsClosed).not.toHaveBeenCalled()
      expect(Reflect.get(tracker, 'webDocuments')).toHaveProperty('document-1')

      documentPort.postMessage({
        from: 'document-1',
        resumeReady: true,
      })

      await expect(readyPromise).resolves.toMatchObject({
        state: 'ready',
        documentId: 'document-1',
      })

      expect(isSettled()).toBe(true)
    } finally {
      tracker.close()
      documentPort.close()
    }
  })

  it('keeps WebDocument proxy ack pending while the document lock is held', async () => {
    vi.useFakeTimers()
    const webLock = installControllableWebLock()
    let resolveExhausted: () => void = () => {}
    const exhausted = new Promise<void>((resolve) => {
      resolveExhausted = resolve
    })
    const onWebDocumentsExhausted = vi.fn(async () => {
      resolveExhausted()
    })
    const tracker = new WebDocumentTracker(
      'service-worker',
      WebRuntimeClientType.WebRuntimeClientType_SERVICE_WORKER,
      onWebDocumentsExhausted,
      null,
    )
    const documentPort = attachWebDocument(tracker)
    const waitConn = tracker.waitConn()
    const isSettled = markSettled(waitConn)

    await vi.advanceTimersByTimeAsync(5000)

    expect(isSettled()).toBe(false)
    expect(onWebDocumentsExhausted).not.toHaveBeenCalled()
    expect(Reflect.get(tracker, 'webDocuments')).toHaveProperty('document-1')

    webLock.release()
    await exhausted

    expect(isSettled()).toBe(false)
    expect(onWebDocumentsExhausted).toHaveBeenCalledTimes(1)
    expect(Reflect.get(tracker, 'webDocuments')).not.toHaveProperty(
      'document-1',
    )

    tracker.close()
    await expect(waitConn).rejects.toThrow(
      'closed while waiting for WebDocument',
    )
    documentPort.close()
  })

  it('keeps a live WebDocument after connect error acks and retries when it becomes ready', async () => {
    const onWebDocumentsExhausted = vi.fn().mockResolvedValue(undefined)
    const tracker = new WebDocumentTracker(
      'service-worker',
      WebRuntimeClientType.WebRuntimeClientType_SERVICE_WORKER,
      onWebDocumentsExhausted,
      null,
    )
    const documentPort = attachWebDocument(tracker)
    const connectResolvers: Array<(msg: ClientToWebDocument) => void> = []
    const nextConnectMsg = () =>
      new Promise<ClientToWebDocument>((resolve) => {
        connectResolvers.push(resolve)
      })
    documentPort.onmessage = (ev) => {
      connectResolvers.shift()?.(ev.data)
    }
    documentPort.start()

    const firstConnectMsg = nextConnectMsg()
    const waitConn = tracker.waitConn()
    const isSettled = markSettled(waitConn)
    const msg = await firstConnectMsg
    const ackPort = msg.connectWebRuntime?.port
    if (!ackPort) {
      throw new Error('connectWebRuntime ack port missing')
    }
    ackPort.postMessage({
      from: 'document-1',
      error: 'webRuntimePort not initialized',
    })

    await Promise.resolve()
    await Promise.resolve()

    expect(isSettled()).toBe(false)
    expect(onWebDocumentsExhausted).not.toHaveBeenCalled()
    expect(Reflect.get(tracker, 'webDocuments')).toHaveProperty('document-1')

    const secondConnectMsg = nextConnectMsg()
    documentPort.postMessage({
      from: 'document-1',
      resumeReady: true,
    })

    const retryMsg = await secondConnectMsg
    const retryAckPort = retryMsg.connectWebRuntime?.port
    if (!retryAckPort) {
      throw new Error('retry connectWebRuntime ack port missing')
    }
    const runtimeChannel = new MessageChannel()
    retryAckPort.postMessage(
      {
        from: 'document-1',
        webRuntimePort: runtimeChannel.port2,
      },
      [runtimeChannel.port2],
    )
    runtimeChannel.port1.postMessage({ connected: true })

    await expect(waitConn).resolves.toBeUndefined()
    expect(onWebDocumentsExhausted).not.toHaveBeenCalled()

    tracker.close()
    documentPort.close()
    runtimeChannel.port1.close()
  })

  it('removes a WebDocument when it disconnects before the connect ack', async () => {
    const webLock = installControllableWebLock()
    let resolveExhausted: () => void = () => {}
    const exhausted = new Promise<void>((resolve) => {
      resolveExhausted = resolve
    })
    const onWebDocumentsExhausted = vi.fn(async () => {
      resolveExhausted()
    })
    const tracker = new WebDocumentTracker(
      'service-worker',
      WebRuntimeClientType.WebRuntimeClientType_SERVICE_WORKER,
      onWebDocumentsExhausted,
      null,
    )
    const documentPort = attachWebDocument(tracker)
    const connectMsg = new Promise<ClientToWebDocument>((resolve) => {
      documentPort.onmessage = (ev) => {
        resolve(ev.data)
      }
      documentPort.start()
    })

    const waitConn = tracker.waitConn()
    const msg = await connectMsg
    const ackPort = msg.connectWebRuntime?.port
    if (!ackPort) {
      throw new Error('connectWebRuntime ack port missing')
    }
    webLock.release()

    await exhausted
    tracker.close()
    await expect(waitConn).rejects.toThrow(
      'closed while waiting for WebDocument',
    )
    expect(onWebDocumentsExhausted).toHaveBeenCalledTimes(1)
    documentPort.close()
  })

  it('keeps SAB pair open pending until the WebDocument closes', async () => {
    vi.useFakeTimers()
    const tracker = buildTracker()
    const documentPort = attachWebDocument(tracker)

    const sabPair = tracker.requestSabPair('worker-1')
    const isSettled = markSettled(sabPair)

    await vi.advanceTimersByTimeAsync(5000)

    expect(isSettled()).toBe(false)

    documentPort.postMessage({
      from: 'document-1',
      close: true,
    })
    await vi.advanceTimersByTimeAsync(0)

    await expect(sabPair).rejects.toThrow('WebDocument document-1 closed')
    tracker.close()
    documentPort.close()
  })

  it('keeps WebRTC bridge open pending until the WebDocument closes', async () => {
    vi.useFakeTimers()
    const tracker = buildTracker()
    const documentPort = attachWebDocument(tracker)

    const bridgePort = tracker.requestWebRtcBridge()
    const isSettled = markSettled(bridgePort)

    await vi.advanceTimersByTimeAsync(5000)

    expect(isSettled()).toBe(false)

    documentPort.postMessage({
      from: 'document-1',
      close: true,
    })
    await vi.advanceTimersByTimeAsync(0)

    await expect(bridgePort).rejects.toThrow('WebDocument document-1 closed')
    tracker.close()
    documentPort.close()
  })

  it('keeps exhausted shutdown rejections attached to the reconnect promise', async () => {
    const onWebDocumentsExhausted = vi.fn(async () => {
      tracker.close()
    })
    const tracker = new WebDocumentTracker(
      'plugin/spacewave-core',
      WebRuntimeClientType.WebRuntimeClientType_WEB_WORKER,
      onWebDocumentsExhausted,
      null,
    )

    await expect(tracker.waitConn()).rejects.toThrow(
      'closed while waiting for WebDocument',
    )
    expect(onWebDocumentsExhausted).toHaveBeenCalledTimes(1)
  })
})
