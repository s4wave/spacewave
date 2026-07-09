import { afterEach, describe, expect, it, vi } from 'vitest'

import type { ClientToWebDocument } from '../runtime/runtime.js'
import {
  WebRuntimeClientInit,
  WebRuntimeClientType,
} from '../runtime/runtime.pb.js'
import { WebDocumentTracker } from './web-document-tracker.js'

function buildTracker(): WebDocumentTracker {
  return new WebDocumentTracker(
    'tracker-client',
    WebRuntimeClientType.WebRuntimeClientType_WEB_WORKER,
    vi.fn().mockResolvedValue(undefined),
    null,
  )
}

function waitForActiveWebDocumentRuntimeConnected(
  tracker: WebDocumentTracker,
): Promise<unknown> {
  const waitForRuntimeConnected = Reflect.get(
    tracker,
    'waitForActiveWebDocumentRuntimeConnected',
  ) as (this: WebDocumentTracker) => Promise<unknown>
  return waitForRuntimeConnected.call(tracker)
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
      waitForActiveWebDocumentRuntimeConnected(tracker),
    ).resolves.toMatchObject({
      state: 'unavailable',
      reason: 'no active WebDocument',
    })

    tracker.close()
  })

  it('closes the shared runtime client on explicit tracker close', () => {
    const tracker = buildTracker()
    const closeRuntime = vi.spyOn(tracker.webRuntimeClient, 'close')

    tracker.close()
    tracker.close()

    expect(closeRuntime).toHaveBeenCalledTimes(1)
  })

  it('resolves the active document runtime-connected gate from the WebDocument port', async () => {
    const tracker = buildTracker()
    const { port1, port2 } = new MessageChannel()
    tracker.handleWebDocumentMessage({
      from: 'document-1',
      initPort: port1,
    })
    Reflect.set(tracker, 'lastWebDocumentId', 'document-1')

    const readyPromise = waitForActiveWebDocumentRuntimeConnected(tracker)
    let resolved = false
    readyPromise.then(() => {
      resolved = true
    })
    await Promise.resolve()
    expect(resolved).toBe(false)

    port2.postMessage({
      from: 'document-1',
      runtimeConnected: true,
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

    const readyPromise = waitForActiveWebDocumentRuntimeConnected(tracker)

    port2.postMessage({
      from: 'document-1',
      close: true,
    })

    await expect(readyPromise).resolves.toMatchObject({
      state: 'closed',
      documentId: 'document-1',
      reason: expect.stringContaining(
        'WebDocument document-1 closed before runtime-connected',
      ),
    })

    tracker.close()
    port2.close()
  })

  it('waits again after the active WebDocument clears runtime-connected', async () => {
    const tracker = buildTracker()
    const { port1, port2 } = new MessageChannel()
    tracker.handleWebDocumentMessage({
      from: 'document-1',
      initPort: port1,
    })
    Reflect.set(tracker, 'lastWebDocumentId', 'document-1')

    port2.postMessage({
      from: 'document-1',
      runtimeConnected: true,
    })
    await expect(
      waitForActiveWebDocumentRuntimeConnected(tracker),
    ).resolves.toMatchObject({
      state: 'ready',
      documentId: 'document-1',
    })

    port2.postMessage({
      from: 'document-1',
      runtimeConnected: false,
    })
    await new Promise((resolve) => setTimeout(resolve, 0))

    const readyPromise = waitForActiveWebDocumentRuntimeConnected(tracker)
    let resolved = false
    readyPromise.then(() => {
      resolved = true
    })
    await Promise.resolve()
    expect(resolved).toBe(false)

    port2.postMessage({
      from: 'document-1',
      runtimeConnected: true,
    })

    await expect(readyPromise).resolves.toMatchObject({
      state: 'ready',
      documentId: 'document-1',
    })

    tracker.close()
    port2.close()
  })

  it('moves the active gate to the latest document without closing runtime when documents drain', async () => {
    const tracker = buildTracker()
    const closeRuntime = vi.spyOn(tracker.webRuntimeClient, 'close')
    const firstPort = attachWebDocument(tracker, 'document-1')

    firstPort.postMessage({
      from: 'document-1',
      runtimeConnected: true,
    })

    await expect(
      waitForActiveWebDocumentRuntimeConnected(tracker),
    ).resolves.toMatchObject({
      state: 'ready',
      documentId: 'document-1',
    })

    const secondPort = attachWebDocument(tracker, 'document-2')
    const secondReady = waitForActiveWebDocumentRuntimeConnected(tracker)
    const secondSettled = markSettled(secondReady)
    await Promise.resolve()
    expect(secondSettled()).toBe(false)

    secondPort.postMessage({
      from: 'document-2',
      runtimeConnected: true,
    })

    await expect(secondReady).resolves.toMatchObject({
      state: 'ready',
      documentId: 'document-2',
    })

    secondPort.postMessage({
      from: 'document-2',
      close: true,
    })
    await vi.waitFor(() => {
      expect(Reflect.get(tracker, 'webDocuments')).not.toHaveProperty(
        'document-2',
      )
    })

    expect(closeRuntime).not.toHaveBeenCalled()
    await expect(
      waitForActiveWebDocumentRuntimeConnected(tracker),
    ).resolves.toMatchObject({
      state: 'ready',
      documentId: 'document-1',
    })

    firstPort.postMessage({
      from: 'document-1',
      close: true,
    })
    await vi.waitFor(() => {
      expect(Reflect.get(tracker, 'webDocuments')).not.toHaveProperty(
        'document-1',
      )
    })

    expect(closeRuntime).not.toHaveBeenCalled()
    expect(Reflect.get(tracker, 'lastWebDocumentId')).toBeUndefined()
    tracker.close()
    firstPort.close()
    secondPort.close()
  })

  it('keeps stream-open readiness on the active runtime relay instead of the newest document', async () => {
    const tracker = buildTracker()
    const firstPort = attachWebDocument(tracker, 'document-1')

    firstPort.postMessage({
      from: 'document-1',
      runtimeConnected: true,
    })
    await expect(
      waitForActiveWebDocumentRuntimeConnected(tracker),
    ).resolves.toMatchObject({
      state: 'ready',
      documentId: 'document-1',
    })

    Reflect.set(tracker, 'activeRuntimeWebDocumentId', 'document-1')
    const secondPort = attachWebDocument(tracker, 'document-2')
    const readyPromise = waitForActiveWebDocumentRuntimeConnected(tracker)

    await expect(readyPromise).resolves.toMatchObject({
      state: 'ready',
      documentId: 'document-1',
    })

    tracker.close()
    firstPort.close()
    secondPort.close()
  })

  it('prefers a newly ready service worker document without interrupting the active runtime', async () => {
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
    await vi.waitFor(() => {
      expect(Reflect.get(tracker, 'preferredRuntimeWebDocumentId')).toBe(
        'document-2',
      )
    })

    expect(closeRuntime).not.toHaveBeenCalled()
    expect(Reflect.get(tracker, 'activeRuntimeWebDocumentId')).toBe(
      'document-1',
    )
    expect(Reflect.get(tracker, 'preferredRuntimeWebDocumentId')).toBe(
      'document-2',
    )

    tracker.close()
    firstPort.close()
    secondPort.close()
  })

  it('clears the preferred service worker document when resume-ready turns false', async () => {
    const tracker = new WebDocumentTracker(
      'service-worker',
      WebRuntimeClientType.WebRuntimeClientType_SERVICE_WORKER,
      vi.fn().mockResolvedValue(undefined),
      null,
    )
    const documentPort = attachWebDocument(tracker, 'document-1')

    documentPort.start()
    documentPort.postMessage({
      from: 'document-1',
      resumeReady: true,
    })
    await vi.waitFor(() => {
      expect(Reflect.get(tracker, 'preferredRuntimeWebDocumentId')).toBe(
        'document-1',
      )
    })

    documentPort.postMessage({
      from: 'document-1',
      resumeReady: false,
    })
    await vi.waitFor(() => {
      expect(
        Reflect.get(tracker, 'preferredRuntimeWebDocumentId'),
      ).toBeUndefined()
    })

    tracker.close()
    documentPort.close()
  })

  it('reroutes the runtime client when the active relay document closes while others remain', async () => {
    const tracker = buildTracker()
    const closeRuntime = vi.spyOn(tracker.webRuntimeClient, 'close')
    const rerouteRuntime = vi
      .spyOn(tracker.webRuntimeClient, 'rerouteChannel')
      .mockResolvedValue(undefined)
    const firstPort = attachWebDocument(tracker, 'document-1')
    const secondPort = attachWebDocument(tracker, 'document-2')

    Reflect.set(tracker, 'activeRuntimeWebDocumentId', 'document-1')

    firstPort.postMessage({
      from: 'document-1',
      close: true,
    })
    await vi.waitFor(() => {
      expect(Reflect.get(tracker, 'webDocuments')).not.toHaveProperty(
        'document-1',
      )
    })

    expect(closeRuntime).not.toHaveBeenCalled()
    expect(rerouteRuntime).toHaveBeenCalledTimes(1)
    expect(Reflect.get(tracker, 'activeRuntimeWebDocumentId')).toBeUndefined()
    expect(Reflect.get(tracker, 'webDocuments')).not.toHaveProperty(
      'document-1',
    )
    expect(Reflect.get(tracker, 'webDocuments')).toHaveProperty('document-2')
    expect(Reflect.get(tracker, 'lastWebDocumentId')).toBe('document-2')

    tracker.close()
    firstPort.close()
    secondPort.close()
  })

  it('reroutes instead of closing the shared runtime client when the last active WebDocument closes', async () => {
    const onWebDocumentsExhausted = vi.fn().mockResolvedValue(undefined)
    const onAllWebDocumentsClosed = vi.fn()
    const tracker = new WebDocumentTracker(
      'tracker-client',
      WebRuntimeClientType.WebRuntimeClientType_WEB_WORKER,
      onWebDocumentsExhausted,
      null,
      onAllWebDocumentsClosed,
    )
    const closeRuntime = vi.spyOn(tracker.webRuntimeClient, 'close')
    const rerouteRuntime = vi
      .spyOn(tracker.webRuntimeClient, 'rerouteChannel')
      .mockResolvedValue(undefined)
    const documentPort = attachWebDocument(tracker, 'document-1')

    Reflect.set(tracker, 'activeRuntimeWebDocumentId', 'document-1')

    documentPort.postMessage({
      from: 'document-1',
      close: true,
    })
    await vi.waitFor(() => {
      expect(onAllWebDocumentsClosed).toHaveBeenCalledTimes(1)
    })

    expect(rerouteRuntime).toHaveBeenCalledTimes(1)
    expect(closeRuntime).not.toHaveBeenCalled()
    expect(onWebDocumentsExhausted).not.toHaveBeenCalled()
    expect(Reflect.get(tracker, 'activeRuntimeWebDocumentId')).toBeUndefined()
    expect(Reflect.get(tracker, 'lastWebDocumentId')).toBeUndefined()
    expect(Reflect.get(tracker, 'lastWebDocumentIdx')).toBe(0)
    expect(Reflect.get(tracker, 'webDocuments')).not.toHaveProperty(
      'document-1',
    )

    tracker.close()
    documentPort.close()
  })

  it('closes the shared runtime client when the last active WebDocument closes terminally', async () => {
    const onWebDocumentsExhausted = vi.fn().mockResolvedValue(undefined)
    const onAllWebDocumentsClosed = vi.fn()
    const tracker = new WebDocumentTracker(
      'tracker-client',
      WebRuntimeClientType.WebRuntimeClientType_WEB_WORKER,
      onWebDocumentsExhausted,
      null,
      onAllWebDocumentsClosed,
    )
    const closeRuntime = vi.spyOn(tracker.webRuntimeClient, 'close')
    const rerouteRuntime = vi
      .spyOn(tracker.webRuntimeClient, 'rerouteChannel')
      .mockResolvedValue(undefined)
    const documentPort = attachWebDocument(tracker, 'document-1')

    Reflect.set(tracker, 'activeRuntimeWebDocumentId', 'document-1')

    documentPort.postMessage({
      from: 'document-1',
      close: true,
      terminal: true,
    })
    await vi.waitFor(() => {
      expect(onAllWebDocumentsClosed).toHaveBeenCalledTimes(1)
    })

    expect(closeRuntime).toHaveBeenCalledTimes(1)
    expect(rerouteRuntime).not.toHaveBeenCalled()
    expect(onWebDocumentsExhausted).not.toHaveBeenCalled()
    expect(Reflect.get(tracker, 'activeRuntimeWebDocumentId')).toBeUndefined()
    expect(Reflect.get(tracker, 'lastWebDocumentId')).toBeUndefined()
    expect(Reflect.get(tracker, 'webDocuments')).not.toHaveProperty(
      'document-1',
    )

    documentPort.close()
  })

  it('relays to a hidden connected active document without resume-ready', async () => {
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

      const readyPromise = waitForActiveWebDocumentRuntimeConnected(tracker)
      const isSettled = markSettled(readyPromise)

      await vi.advanceTimersByTimeAsync(5000)

      expect(isSettled()).toBe(false)
      expect(onWebDocumentsExhausted).not.toHaveBeenCalled()
      expect(onAllWebDocumentsClosed).not.toHaveBeenCalled()
      expect(Reflect.get(tracker, 'webDocuments')).toHaveProperty('document-1')

      documentPort.postMessage({
        from: 'document-1',
        resumeReady: false,
      })
      await Promise.resolve()
      expect(isSettled()).toBe(false)

      documentPort.postMessage({
        from: 'document-1',
        runtimeConnected: true,
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
      runtimeConnected: true,
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

  it('retries an already-ready replacement WebDocument after a stale pre-ack disconnect', async () => {
    const webLock = installControllableWebLock()
    const onWebDocumentsExhausted = vi.fn().mockResolvedValue(undefined)
    const tracker = new WebDocumentTracker(
      'service-worker',
      WebRuntimeClientType.WebRuntimeClientType_SERVICE_WORKER,
      onWebDocumentsExhausted,
      null,
    )
    const stalePort = attachWebDocument(tracker, 'document-1')
    const { promise: staleConnectMsg, resolve: resolveStaleConnectMsg } =
      Promise.withResolvers<ClientToWebDocument>()
    stalePort.onmessage = (ev) => {
      resolveStaleConnectMsg(ev.data)
    }
    stalePort.start()

    const waitConn = tracker.waitConn()
    const staleMsg = await staleConnectMsg
    const staleAckPort = staleMsg.connectWebRuntime?.port
    if (!staleAckPort) {
      throw new Error('stale connectWebRuntime ack port missing')
    }

    const replacementPort = attachWebDocument(tracker, 'document-2')
    const {
      promise: replacementConnectMsg,
      resolve: resolveReplacementConnectMsg,
    } = Promise.withResolvers<ClientToWebDocument>()
    replacementPort.onmessage = (ev) => {
      resolveReplacementConnectMsg(ev.data)
    }
    replacementPort.start()
    replacementPort.postMessage({
      from: 'document-2',
      runtimeConnected: true,
    })

    webLock.release()

    const replacementMsg = await replacementConnectMsg
    expect(Reflect.get(tracker, 'webDocuments')).not.toHaveProperty(
      'document-1',
    )
    expect(Reflect.get(tracker, 'webDocuments')).toHaveProperty('document-2')
    const replacementAckPort = replacementMsg.connectWebRuntime?.port
    if (!replacementAckPort) {
      throw new Error('replacement connectWebRuntime ack port missing')
    }
    const runtimeChannel = new MessageChannel()
    replacementAckPort.postMessage(
      {
        from: 'document-2',
        webRuntimePort: runtimeChannel.port2,
      },
      [runtimeChannel.port2],
    )
    runtimeChannel.port1.postMessage({ connected: true })

    await expect(waitConn).resolves.toBeUndefined()
    expect(onWebDocumentsExhausted).not.toHaveBeenCalled()

    tracker.close()
    stalePort.close()
    staleAckPort.close()
    replacementPort.close()
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

  it('returns OPFS worker bridge port from the WebDocument ack', async () => {
    const tracker = buildTracker()
    const documentPort = attachWebDocument(tracker)
    const openMsg = new Promise<ClientToWebDocument>((resolve) => {
      documentPort.onmessage = (ev) => {
        resolve(ev.data)
      }
      documentPort.start()
    })

    const bridgePort = tracker.requestOpfsWorker()
    const msg = await openMsg
    const requestId = msg.openOpfsWorker?.requestId
    expect(requestId).toBe('opfs-worker-open-1')
    if (!requestId) {
      throw new Error('expected OPFS worker request id')
    }
    const { port1, port2 } = new MessageChannel()
    documentPort.postMessage(
      {
        from: 'document-1',
        openOpfsWorkerAck: {
          from: 'document-1',
          requestId,
        },
      },
      [port2],
    )

    const resolvedPort = await bridgePort
    expect(resolvedPort).not.toBeNull()
    const portMessage = new Promise<unknown>((resolve) => {
      port1.onmessage = (ev) => {
        resolve(ev.data)
      }
      port1.start()
    })
    resolvedPort?.postMessage({ ok: true })
    await expect(portMessage).resolves.toEqual({ ok: true })

    tracker.close()
    documentPort.close()
    port1.close()
    resolvedPort?.close()
  })

  it('resolves overlapping OPFS worker acks by requestId', async () => {
    const tracker = buildTracker()
    const documentPort = attachWebDocument(tracker)
    const messages: ClientToWebDocument[] = []
    documentPort.onmessage = (ev) => {
      messages.push(ev.data)
    }
    documentPort.start()

    const first = tracker.requestOpfsWorker()
    const second = tracker.requestOpfsWorker()
    await vi.waitFor(() => {
      expect(messages).toHaveLength(2)
    })

    const firstChannel = new MessageChannel()
    const secondChannel = new MessageChannel()
    documentPort.postMessage(
      {
        from: 'document-1',
        openOpfsWorkerAck: {
          from: 'document-1',
          requestId: messages[1].openOpfsWorker?.requestId ?? '',
        },
      },
      [secondChannel.port2],
    )
    documentPort.postMessage(
      {
        from: 'document-1',
        openOpfsWorkerAck: {
          from: 'document-1',
          requestId: messages[0].openOpfsWorker?.requestId ?? '',
        },
      },
      [firstChannel.port2],
    )

    const firstPort = await first
    const secondPort = await second
    const firstDelivered = new Promise<unknown>((resolve) => {
      firstChannel.port1.onmessage = (ev) => resolve(ev.data)
      firstChannel.port1.start()
    })
    const secondDelivered = new Promise<unknown>((resolve) => {
      secondChannel.port1.onmessage = (ev) => resolve(ev.data)
      secondChannel.port1.start()
    })
    firstPort?.postMessage({ from: 'first' })
    secondPort?.postMessage({ from: 'second' })
    await expect(firstDelivered).resolves.toEqual({ from: 'first' })
    await expect(secondDelivered).resolves.toEqual({ from: 'second' })

    tracker.close()
    documentPort.close()
    firstChannel.port1.close()
    secondChannel.port1.close()
    firstPort?.close()
    secondPort?.close()
  })

  it('resolves overlapping WebRTC bridge acks by requestId', async () => {
    const tracker = buildTracker()
    const documentPort = attachWebDocument(tracker)
    const messages: ClientToWebDocument[] = []
    documentPort.onmessage = (ev) => {
      messages.push(ev.data)
    }
    documentPort.start()

    const first = tracker.requestWebRtcBridge()
    const second = tracker.requestWebRtcBridge()
    await vi.waitFor(() => {
      expect(messages).toHaveLength(2)
    })

    documentPort.postMessage({
      from: 'document-1',
      requestId: messages[1].connectWebRtcBridge?.requestId,
      bridgePort: { label: 'second' } as unknown as MessagePort,
    })
    documentPort.postMessage({
      from: 'document-1',
      requestId: messages[0].connectWebRtcBridge?.requestId,
      bridgePort: { label: 'first' } as unknown as MessagePort,
    })

    await expect(first).resolves.toMatchObject({ label: 'first' })
    await expect(second).resolves.toMatchObject({ label: 'second' })

    tracker.close()
    documentPort.close()
  })

  it('keeps OPFS worker open pending until the WebDocument closes', async () => {
    vi.useFakeTimers()
    const tracker = buildTracker()
    const documentPort = attachWebDocument(tracker)

    const bridgePort = tracker.requestOpfsWorker()
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

  it('opens an elected DedicatedWorker host relay through the host document, not the requester', async () => {
    const tracker = buildTracker()
    Reflect.set(tracker, 'preferredRuntimeWebDocumentId', 'requester-document')
    const requesterPort = attachWebDocument(tracker, 'requester-document')
    const hostPort = attachWebDocument(tracker, 'host-document')
    const requesterMessage = vi.fn()
    requesterPort.onmessage = requesterMessage
    requesterPort.start()

    const runtimeChannel = new MessageChannel()
    const hostMessage = new Promise<void>((resolve) => {
      hostPort.onmessage = (ev: MessageEvent<ClientToWebDocument>) => {
        const request = ev.data.connectWebRuntime
        expect(request).toBeDefined()
        const ackPort = request?.port ?? ev.ports?.[0]
        expect(ackPort).toBeDefined()
        ackPort!.postMessage(
          {
            from: 'host-document',
            webRuntimePort: runtimeChannel.port1,
          },
          [runtimeChannel.port1],
        )
        resolve()
      }
      hostPort.start()
    })
    const init = WebRuntimeClientInit.toBinary({
      webRuntimeId: 'runtime-1',
      clientUuid: 'requester-document',
      clientType: WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT,
    })

    const openedPort = await tracker.openWebRuntimePort(
      init,
      'requester-document',
    )
    await hostMessage

    expect(requesterMessage).not.toHaveBeenCalled()
    const delivered = new Promise<unknown>((resolve) => {
      runtimeChannel.port2.onmessage = (ev: MessageEvent) => resolve(ev.data)
      runtimeChannel.port2.start()
    })
    openedPort.postMessage({ ok: true })
    await expect(delivered).resolves.toEqual({ ok: true })

    tracker.close()
    requesterPort.close()
    hostPort.close()
    openedPort.close()
    runtimeChannel.port2.close()
  })

  it('fails DedicatedWorker host relay when only the requester remains', async () => {
    const onWebDocumentsExhausted = vi.fn().mockResolvedValue(undefined)
    const tracker = new WebDocumentTracker(
      'tracker-client',
      WebRuntimeClientType.WebRuntimeClientType_WEB_WORKER,
      onWebDocumentsExhausted,
      null,
    )
    const requesterPort = attachWebDocument(tracker, 'requester-document')
    const init = WebRuntimeClientInit.toBinary({
      webRuntimeId: 'runtime-1',
      clientUuid: 'requester-document',
      clientType: WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT,
    })

    await expect(
      tracker.openWebRuntimePort(init, 'requester-document'),
    ).rejects.toThrow('no elected DedicatedWorker runtime host available')
    expect(onWebDocumentsExhausted).not.toHaveBeenCalled()

    tracker.close()
    requesterPort.close()
  })

  it('notifies surviving documents when the elected host relay closes', async () => {
    const tracker = buildTracker()
    const requesterPort = attachWebDocument(tracker, 'requester-document')
    const hostPort = attachWebDocument(tracker, 'host-document')
    const lost = new Promise<ClientToWebDocument>((resolve) => {
      requesterPort.onmessage = (ev: MessageEvent<ClientToWebDocument>) => {
        if (ev.data.dedicatedRuntimeHostLost) {
          resolve(ev.data)
        }
      }
      requesterPort.start()
    })

    const runtimeChannel = new MessageChannel()
    const hostMessage = new Promise<void>((resolve) => {
      hostPort.onmessage = (ev: MessageEvent<ClientToWebDocument>) => {
        const request = ev.data.connectWebRuntime
        expect(request).toBeDefined()
        const ackPort = request?.port ?? ev.ports?.[0]
        expect(ackPort).toBeDefined()
        ackPort!.postMessage(
          {
            from: 'host-document',
            webRuntimePort: runtimeChannel.port1,
          },
          [runtimeChannel.port1],
        )
        resolve()
      }
      hostPort.start()
    })
    const init = WebRuntimeClientInit.toBinary({
      webRuntimeId: 'runtime-1',
      clientUuid: 'requester-document',
      clientType: WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT,
    })

    const openedPort = await tracker.openWebRuntimePort(
      init,
      'requester-document',
    )
    await hostMessage
    hostPort.postMessage({ from: 'host-document', close: true })

    await expect(lost).resolves.toMatchObject({
      dedicatedRuntimeHostLost: {
        webDocumentId: 'host-document',
      },
    })

    tracker.close()
    requesterPort.close()
    hostPort.close()
    openedPort.close()
    runtimeChannel.port2.close()
  })
})

describe('WebDocumentTracker runtime fetch relay wait', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  it('resolves immediately when a relay already exists', async () => {
    const tracker = buildTracker()
    const port = attachWebDocument(tracker)

    await expect(tracker.waitForRuntimeFetchRelay(1000)).resolves.toBe(true)

    tracker.close()
    port.close()
  })

  it('resolves true when a WebDocument attaches during the wait', async () => {
    const tracker = buildTracker()

    const waitPromise = tracker.waitForRuntimeFetchRelay(1000)
    const isSettled = markSettled(waitPromise)
    await Promise.resolve()
    expect(isSettled()).toBe(false)

    const port = attachWebDocument(tracker)

    await expect(waitPromise).resolves.toBe(true)

    tracker.close()
    port.close()
  })

  it('resolves false when the wait deadline elapses with no relay', async () => {
    vi.useFakeTimers()
    const tracker = buildTracker()

    const waitPromise = tracker.waitForRuntimeFetchRelay(5000)
    const isSettled = markSettled(waitPromise)

    await vi.advanceTimersByTimeAsync(4999)
    expect(isSettled()).toBe(false)

    await vi.advanceTimersByTimeAsync(1)
    await expect(waitPromise).resolves.toBe(false)

    tracker.close()
  })

  it('resolves false when the tracker closes during the wait', async () => {
    const tracker = buildTracker()

    const waitPromise = tracker.waitForRuntimeFetchRelay(5000)
    tracker.close()

    await expect(waitPromise).resolves.toBe(false)
  })
})

describe('WebDocumentTracker OPFS bridge host lifecycle', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  function buildOpfsBridgeTracker(
    onOpfsBridgeLost: () => void,
  ): WebDocumentTracker {
    return new WebDocumentTracker(
      'tracker-client',
      WebRuntimeClientType.WebRuntimeClientType_WEB_WORKER,
      vi.fn().mockResolvedValue(undefined),
      null,
      null,
      undefined,
      onOpfsBridgeLost,
    )
  }

  async function hostOpfsWorker(
    tracker: WebDocumentTracker,
    documentPort: MessagePort,
    webDocumentId: string,
  ): Promise<void> {
    const openMsg = new Promise<ClientToWebDocument>((resolve) => {
      documentPort.onmessage = (ev) => resolve(ev.data)
      documentPort.start()
    })
    const opfsReq = tracker.requestOpfsWorker()
    const { port1: bridgePort } = new MessageChannel()
    const request = await openMsg
    documentPort.postMessage(
      {
        from: webDocumentId,
        openOpfsWorkerAck: {
          from: webDocumentId,
          requestId: request.openOpfsWorker?.requestId ?? '',
        },
      },
      [bridgePort],
    )
    await opfsReq
  }

  const tick = () => new Promise((resolve) => setTimeout(resolve, 0))

  it('re-hosts the bridge only when the OPFS host document is removed', async () => {
    const onLost = vi.fn()
    const tracker = buildOpfsBridgeTracker(onLost)
    const hostPort = attachWebDocument(tracker, 'host')
    await hostOpfsWorker(tracker, hostPort, 'host')
    const survivorA = attachWebDocument(tracker, 'survivor-a')
    const survivorB = attachWebDocument(tracker, 'survivor-b')

    survivorA.postMessage({ from: 'survivor-a', close: true })
    await tick()
    await tick()
    expect(onLost).not.toHaveBeenCalled()

    hostPort.postMessage({ from: 'host', close: true })
    await tick()
    await tick()
    expect(onLost).toHaveBeenCalledTimes(1)

    tracker.close()
    hostPort.close()
    survivorA.close()
    survivorB.close()
  })

  it('re-hosts the bridge when the broker reports the OPFS worker died', async () => {
    const onLost = vi.fn()
    const tracker = buildOpfsBridgeTracker(onLost)
    const hostPort = attachWebDocument(tracker, 'host')
    await hostOpfsWorker(tracker, hostPort, 'host')

    hostPort.postMessage({ from: 'host', opfsWorkerClosed: true })
    await tick()
    await tick()
    expect(onLost).toHaveBeenCalledTimes(1)

    tracker.close()
    hostPort.close()
  })
})
