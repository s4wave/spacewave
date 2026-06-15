import { HandleStreamFunc } from 'starpc'
import { Message } from '@aptre/protobuf-es-lite'

import {
  buildWebDocumentLockName,
  ClientToWebDocument,
  ConnectWebRuntimeAck,
  SabPairEndpointDescriptor,
  WebDocumentToClient,
  WebDocumentToWorker,
} from '../runtime/runtime.js'
import {
  WebRuntimeClientInit,
  WebRuntimeClientType,
} from '../runtime/runtime.pb.js'
import {
  WebRuntimeClient,
  type RuntimeClientStreamOpenGateResult,
} from './web-runtime-client.js'

interface WebDocumentWaiter {
  resume: () => void
  reject: (err: Error) => void
}

interface WebDocumentResumeReadyWaiter extends WebDocumentWaiter {
  webDocumentId: string
}

interface SabPairOpenWaiter {
  webDocumentId: string
  resolve: (endpoint: SabPairEndpointDescriptor) => void
  reject: (err: Error) => void
}

interface WebRtcBridgeOpenWaiter {
  webDocumentId: string
  resolve: (port: MessagePort) => void
  reject: (err: Error) => void
}

// WebDocumentTracker is a tracks a set of connected WebDocument and attempts to
// connect to the remote WebRuntime via these documents, retrying when the
// owning WebDocument lifecycle closes a stale route.
//
// onWebDocumentsExhausted is called if there are no available web documents to
// connect to and we want a connection with the WebRuntime. Depending on the
// environment, the callback should attempt to acquire new connections with at
// least one WebDocument.
export class WebDocumentTracker {
  // clientUuid is the client uuid to use for WebRuntime clients.
  public readonly clientUuid: string
  // clientType is the client type to use for WebRuntime clients.
  public readonly clientType: WebRuntimeClientType
  // webRuntimeClient is the client to the webRuntime which accesses via. the tracker.
  public readonly webRuntimeClient: WebRuntimeClient

  // webDocuments is the list of active WebDocument MessagePorts.
  private webDocuments: Record<string, MessagePort> = {}
  // closed records that the tracker is shutting down and should not accept new work.
  private closed = false
  // webDocumentWaiters are callbacks waiting for the next WebDocument.
  private webDocumentWaiters: WebDocumentWaiter[] = []
  // webDocumentResumeReadyIds are WebDocuments that reported resume readiness.
  private webDocumentResumeReadyIds = new Set<string>()
  // webDocumentResumeReadyWaiters are callbacks waiting on a specific WebDocument.
  private webDocumentResumeReadyWaiters: WebDocumentResumeReadyWaiter[] = []
  // lastWebDocumentIdx was the last index used from WebDocuments.
  private lastWebDocumentIdx = 0
  // lastWebDocumentId was the last web document id used from WebDocuments.
  private lastWebDocumentId?: string
  // activeRuntimeWebDocumentId is the WebDocument currently relaying the
  // WebRuntimeClient channel.
  private activeRuntimeWebDocumentId?: string
  private activeRuntimeDocumentAbort?: AbortController
  private preferredRuntimeWebDocumentId?: string
  private nextSabPairRequestNumber = 1
  private sabPairOpenWaiters = new Map<string, SabPairOpenWaiter>()
  private sabPairEndpoints = new Map<string, SabPairEndpointDescriptor>()
  private webRtcBridgeOpenWaiters: WebRtcBridgeOpenWaiter[] = []

  constructor(
    clientUuid: string,
    clientType: WebRuntimeClientType,
    private readonly onWebDocumentsExhausted: () => Promise<void>,
    handleIncomingStream: HandleStreamFunc | null,
    private readonly onAllWebDocumentsClosed?:
      | (() => Promise<void> | void)
      | null,
    logicalClientId?: string,
  ) {
    this.clientUuid = clientUuid
    this.clientType = clientType
    this.webRuntimeClient = new WebRuntimeClient(
      '',
      clientUuid,
      clientType,
      this.openWebRuntimeClient.bind(this),
      handleIncomingStream,
      null,
      undefined,
      logicalClientId,
      this.waitForActiveWebDocumentResumeReady.bind(this),
    )
  }

  // waitConn opens and waits for the connection to be ready.
  public async waitConn() {
    return this.webRuntimeClient.waitConn()
  }

  // handleWebDocumentMessage handles an incoming message from the WebDocument.
  public handleWebDocumentMessage(msg: WebDocumentToWorker) {
    if (this.closed) {
      return
    }
    if (typeof msg !== 'object' || !msg.from || !msg.initPort) {
      return
    }

    const { from: webDocumentId, initPort: port } = msg
    console.log(
      `WebDocumentTracker: ${this.clientUuid}: added WebDocument: ${webDocumentId}`,
    )

    this.webDocuments[webDocumentId] = port
    this.lastWebDocumentId = webDocumentId
    port.onmessage = (ev) => {
      const data: WebDocumentToClient = ev.data
      if (typeof data !== 'object' || data === null) {
        return
      }

      if (data.close) {
        const closeErr = new Error(
          `WebDocumentTracker: ${this.clientUuid}: WebDocument ${webDocumentId} closed`,
        )
        this.removeWebDocument(webDocumentId, closeErr).catch((err) => {
          console.error(
            `WebDocumentTracker: ${this.clientUuid}: error handling WebDocument close:`,
            err,
          )
        })
        return
      }

      if (data.resumeReady === true) {
        this.webDocumentResumeReadyIds.add(webDocumentId)
        this.resolveResumeReadyWaiters(webDocumentId)
        this.preferReadyServiceWorkerDocument(webDocumentId)
        const waiters = this.webDocumentWaiters.splice(0)
        for (const waiter of waiters) {
          waiter.resume()
        }
      }
      if (data.resumeReady === false) {
        this.webDocumentResumeReadyIds.delete(webDocumentId)
      }

      if (data.openSabPairAck) {
        const waiter = this.sabPairOpenWaiters.get(
          data.openSabPairAck.requestId,
        )
        if (!waiter) {
          return
        }
        this.sabPairOpenWaiters.delete(data.openSabPairAck.requestId)
        if (data.openSabPairAck.error) {
          waiter.reject(new Error(data.openSabPairAck.error))
          return
        }
        if (!data.openSabPairAck.endpoint) {
          waiter.reject(new Error('SAB pair open ack missing endpoint'))
          return
        }
        waiter.resolve(data.openSabPairAck.endpoint)
        return
      }

      if (data.bridgePort) {
        const waiterIdx = this.webRtcBridgeOpenWaiters.findIndex(
          (waiter) => waiter.webDocumentId === webDocumentId,
        )
        if (waiterIdx === -1) {
          return
        }
        const waiter = this.webRtcBridgeOpenWaiters.splice(waiterIdx, 1)[0]
        if (!waiter) {
          return
        }
        waiter.resolve(data.bridgePort)
        return
      }

      if (data.sabPairEndpoint) {
        this.sabPairEndpoints.set(
          data.sabPairEndpoint.pairId,
          data.sabPairEndpoint,
        )
      }

      if (data.sabPairClosed) {
        this.sabPairEndpoints.delete(data.sabPairClosed.pairId)
      }
    }

    const waiters = this.webDocumentWaiters.splice(0)
    for (const waiter of waiters) {
      waiter.resume()
    }

    port.start()
  }

  // close tells all connected web documents that this client is closing.
  public close() {
    if (this.closed) {
      return
    }
    this.closed = true
    const msg: ClientToWebDocument = {
      from: this.clientUuid,
      close: true,
    }
    for (const docID in this.webDocuments) {
      const doc = this.webDocuments[docID]
      doc.postMessage(msg)
      delete this.webDocuments[docID]
    }
    this.webDocumentResumeReadyIds.clear()
    delete this.lastWebDocumentId
    delete this.activeRuntimeWebDocumentId
    this.activeRuntimeDocumentAbort?.abort()
    this.activeRuntimeDocumentAbort = undefined
    const err = new Error(
      `WebDocumentTracker: ${this.clientUuid}: closed while waiting for WebDocument`,
    )
    this.rejectWaiters(err)
    this.rejectAllResumeReadyWaiters(err)
    this.rejectAllSabPairWaiters(err)
    this.rejectAllWebRtcBridgeWaiters(err)
  }

  // postMessage posts a message to all connected web documents.
  public postMessage(msg: ClientToWebDocument) {
    for (const docID in this.webDocuments) {
      this.webDocuments[docID]?.postMessage(msg)
    }
  }

  public hasRuntimeFetchRelay(): boolean {
    const webDocumentId =
      this.activeRuntimeWebDocumentId ??
      this.preferredRuntimeWebDocumentId ??
      this.lastWebDocumentId
    return !!(webDocumentId && this.webDocuments[webDocumentId])
  }

  // waitForRuntimeFetchRelay resolves true once a runtime fetch relay becomes
  // available, or false if timeoutMs elapses, the signal aborts, or the tracker
  // closes. After a page client closes there is a brief gap before the next
  // WebDocument establishes its relay; routing a static plugin asset fetch to
  // the dying client during that gap fails an in-flight navigation. Callers wait
  // for the next relay (driven by WebDocument attach and resume-ready events, no
  // polling) and retry instead of failing the fetch.
  public waitForRuntimeFetchRelay(
    timeoutMs: number,
    signal?: AbortSignal,
  ): Promise<boolean> {
    if (this.hasRuntimeFetchRelay()) {
      return Promise.resolve(true)
    }
    if (this.closed || signal?.aborted) {
      return Promise.resolve(false)
    }
    return new Promise<boolean>((resolve) => {
      let settled = false
      const onAbort = () => settle(false)
      const waiter: WebDocumentWaiter = {
        resume: () => {
          if (settled) {
            return
          }
          if (this.hasRuntimeFetchRelay()) {
            settle(true)
            return
          }
          // Relay still absent: re-arm for the next attach / resume-ready event.
          this.webDocumentWaiters.push(waiter)
        },
        reject: () => settle(false),
      }
      const timer = setTimeout(() => settle(false), timeoutMs)
      // settle is hoisted so timer can be const-initialized before its body runs.
      function settle(value: boolean) {
        if (settled) {
          return
        }
        settled = true
        clearTimeout(timer)
        signal?.removeEventListener('abort', onAbort)
        resolve(value)
      }
      signal?.addEventListener('abort', onAbort, { once: true })
      this.webDocumentWaiters.push(waiter)
    })
  }

  public async requestSabPair(
    targetWorkerId: string,
  ): Promise<SabPairEndpointDescriptor> {
    const webDocumentIds = Object.keys(this.webDocuments)
    if (!webDocumentIds.length) {
      throw new Error('no WebDocument available for SAB pair open')
    }

    const docId = this.lastWebDocumentId ?? webDocumentIds[0]
    const docPort = this.webDocuments[docId]
    if (!docPort) {
      throw new Error('selected WebDocument is closed')
    }

    const requestId = `sab-pair-open-${this.nextSabPairRequestNumber++}`
    return new Promise<SabPairEndpointDescriptor>((resolve, reject) => {
      this.sabPairOpenWaiters.set(requestId, {
        webDocumentId: docId,
        resolve,
        reject,
      })
      try {
        docPort.postMessage({
          from: this.clientUuid,
          openSabPair: {
            requestId,
            targetWorkerId,
          },
        } satisfies ClientToWebDocument)
      } catch (err) {
        this.sabPairOpenWaiters.delete(requestId)
        reject(err instanceof Error ? err : new Error(String(err)))
      }
    })
  }

  public closeSabPair(pairId: string): void {
    this.sabPairEndpoints.delete(pairId)
    const msg: ClientToWebDocument = {
      from: this.clientUuid,
      closeSabPair: { pairId },
    }
    for (const docID in this.webDocuments) {
      this.webDocuments[docID]?.postMessage(msg)
    }
  }

  // requestWebRtcBridge requests a WebRTC bridge port from the first available
  // WebDocument. Returns null only when no WebDocument is available.
  public async requestWebRtcBridge(): Promise<MessagePort | null> {
    const webDocumentIds = Object.keys(this.webDocuments)
    if (!webDocumentIds.length) return null

    // Use the last connected WebDocument (most likely to be alive).
    const docId = this.lastWebDocumentId ?? webDocumentIds[0]
    const docPort = this.webDocuments[docId]
    if (!docPort) return null

    return new Promise<MessagePort | null>((resolve, reject) => {
      const waiter: WebRtcBridgeOpenWaiter = {
        webDocumentId: docId,
        resolve,
        reject,
      }
      this.webRtcBridgeOpenWaiters.push(waiter)
      const msg: ClientToWebDocument = {
        from: this.clientUuid,
        connectWebRtcBridge: true,
      }
      try {
        docPort.postMessage(msg)
      } catch (err) {
        this.removeWebRtcBridgeWaiter(waiter)
        reject(err instanceof Error ? err : new Error(String(err)))
      }
    })
  }

  // openWebRuntimeClient attempts to open a client via one of the WebDocuments.
  private async openWebRuntimeClient(
    initMsg: Message<WebRuntimeClientInit>,
  ): Promise<MessagePort> {
    if (this.closed) {
      throw new Error(
        `WebDocumentTracker: ${this.clientUuid}: closed while waiting for WebDocument`,
      )
    }
    const init = WebRuntimeClientInit.toBinary(initMsg)
    const usePreferredOrder = !!(
      this.preferredRuntimeWebDocumentId &&
      this.webDocuments[this.preferredRuntimeWebDocumentId]
    )
    const webDocumentIds = this.orderRuntimeOpenWebDocuments(
      Object.keys(this.webDocuments),
    )
    for (const i of webDocumentIds.keys()) {
      const x = usePreferredOrder
        ? i
        : (i + this.lastWebDocumentIdx + 1) % webDocumentIds.length
      const webDocumentId = webDocumentIds[x]
      const webDocumentPort = this.webDocuments[webDocumentId]
      if (!webDocumentPort) {
        delete this.webDocuments[webDocumentId]
        continue
      }

      const ackChannel = new MessageChannel()
      const ackPromise = new Promise<ConnectWebRuntimeAck>((resolve) => {
        const ackPort = ackChannel.port1
        ackPort.onmessage = (ev) => {
          const data: ConnectWebRuntimeAck = ev.data
          if (!data || !data.from) {
            return
          }
          resolve(data)
        }
        ackPort.start()
      })
      const lockAbortController = new AbortController()
      const disconnectedPromise = this.waitForWebDocumentDisconnect(
        webDocumentId,
        lockAbortController.signal,
      )
      let removeWebDocumentOnFailure = false

      try {
        console.log(
          `WebDocumentTracker: ${this.clientUuid}: connecting via WebDocument: ${webDocumentId}`,
        )

        // request that we open the connection to the web runtime.
        // NOTE: this does not necessarily throw an error if the remote WebDocument is closed.
        const connectMsg: ClientToWebDocument = {
          from: this.clientUuid,
          connectWebRuntime: {
            init,
            port: ackChannel.port2,
          },
        }
        try {
          webDocumentPort.postMessage(connectMsg, [ackChannel.port2])
        } catch (err) {
          removeWebDocumentOnFailure = true
          throw err
        }

        const result = await Promise.race([ackPromise, disconnectedPromise])
        if (!result) {
          removeWebDocumentOnFailure = true
          throw new Error(
            `WebDocumentTracker: ${this.clientUuid}: WebDocument ${webDocumentId} closed before ack`,
          )
        }
        if (result instanceof Error) {
          removeWebDocumentOnFailure = true
          throw result
        }
        if (result.error) {
          throw new Error(result.error)
        }
        if (!result.webRuntimePort) {
          throw new Error(
            `WebDocumentTracker: ${this.clientUuid}: WebDocument ${webDocumentId} ack missing runtime port`,
          )
        }
        console.log(
          `WebDocumentTracker: ${this.clientUuid}: opened port with WebRuntime via WebDocument: ${webDocumentId}`,
        )
        this.lastWebDocumentIdx = x
        this.lastWebDocumentId = webDocumentId
        if (this.preferredRuntimeWebDocumentId === webDocumentId) {
          delete this.preferredRuntimeWebDocumentId
        }
        this.trackActiveRuntimeWebDocument(webDocumentId)
        return result.webRuntimePort
      } catch (err) {
        // message port must be closed.
        const expectedClose = isExpectedWebDocumentCloseError(err)
        if (expectedClose) {
          console.warn(
            `ServiceWorker: connecting via WebDocument closed: ${webDocumentId}`,
            err,
          )
        }
        if (!expectedClose) {
          console.error(
            `ServiceWorker: connecting via WebDocument failed: ${webDocumentId}`,
            err,
          )
        }
        // A connect ack error can mean the document is alive but its runtime
        // port is not ready yet. Only forget the document when liveness or
        // postMessage proves the relay is actually gone.
        if (expectedClose || removeWebDocumentOnFailure) {
          await this.removeWebDocument(
            webDocumentId,
            err instanceof Error ? err : new Error(String(err)),
          )
        }
        continue
      } finally {
        ackChannel.port1.close()
        lockAbortController.abort()
      }
    }

    if (Object.keys(this.webDocuments).length) {
      console.log(
        'ServiceWorker: waiting for existing WebDocument to become ready',
      )
      return new Promise<MessagePort>((resolve, reject) => {
        this.webDocumentWaiters.push({
          resume: () => {
            resolve(this.openWebRuntimeClient(initMsg))
          },
          reject,
        })
      })
    }

    // construct a promise to catch any new incoming WebDocument client
    const waitPromise = new Promise<MessagePort>((resolve, reject) => {
      // try again once a new WebDocument is added.
      this.webDocumentWaiters.push({
        resume: () => {
          resolve(this.openWebRuntimeClient(initMsg))
        },
        reject,
      })
    })

    void waitPromise.catch(() => {})

    // notify all WebDocument that we are looking for a connection to them.
    await this.onWebDocumentsExhausted()

    console.log('ServiceWorker: waiting for next WebDocument to proxy conn')
    return waitPromise
  }

  // waitForWebDocumentDisconnect resolves when the web document liveness lock becomes available.
  private waitForWebDocumentDisconnect(
    webDocumentId: string,
    signal: AbortSignal,
  ): Promise<Error | undefined> {
    if (typeof navigator === 'undefined' || !navigator.locks) {
      // No timer fallback here: a hidden WebDocument without Web Locks can be
      // suspended, so elapsed time is not proof that the document disappeared.
      return new Promise(() => {})
    }

    return navigator.locks
      .request(buildWebDocumentLockName(webDocumentId), { signal }, () => {
        return new Error(
          `WebDocumentTracker: ${this.clientUuid}: WebDocument ${webDocumentId} disconnected before ack`,
        )
      })
      .catch((err) => {
        if (isAbortError(err)) {
          return undefined
        }
        throw err
      })
  }

  private trackActiveRuntimeWebDocument(webDocumentId: string): void {
    this.activeRuntimeDocumentAbort?.abort()
    this.activeRuntimeWebDocumentId = webDocumentId

    if (typeof navigator === 'undefined' || !navigator.locks) {
      this.activeRuntimeDocumentAbort = undefined
      return
    }

    const abortController = new AbortController()
    this.activeRuntimeDocumentAbort = abortController
    this.waitForWebDocumentDisconnect(webDocumentId, abortController.signal)
      .then((err) => {
        if (
          !err ||
          this.closed ||
          this.activeRuntimeDocumentAbort !== abortController ||
          this.activeRuntimeWebDocumentId !== webDocumentId
        ) {
          return
        }
        return this.removeWebDocument(webDocumentId, err)
      })
      .catch((err: unknown) => {
        if (abortController.signal.aborted) {
          return
        }
        console.error(
          `WebDocumentTracker: ${this.clientUuid}: active WebDocument disconnect watch failed:`,
          err,
        )
      })
  }

  private preferReadyServiceWorkerDocument(webDocumentId: string): void {
    if (
      this.clientType !==
        WebRuntimeClientType.WebRuntimeClientType_SERVICE_WORKER ||
      this.activeRuntimeWebDocumentId === webDocumentId ||
      !this.webDocuments[webDocumentId]
    ) {
      return
    }

    this.preferredRuntimeWebDocumentId = webDocumentId
  }

  private orderRuntimeOpenWebDocuments(webDocumentIds: string[]): string[] {
    const preferred = this.preferredRuntimeWebDocumentId
    if (!preferred || !this.webDocuments[preferred]) {
      return webDocumentIds
    }
    return [
      preferred,
      ...webDocumentIds.filter((webDocumentId) => webDocumentId !== preferred),
    ]
  }

  private async removeWebDocument(
    webDocumentId: string,
    closeErr: Error,
  ): Promise<void> {
    const closePort = this.webDocuments[webDocumentId]
    if (!closePort) {
      return
    }

    closePort.close()
    console.log(
      `WebDocumentTracker: ${this.clientUuid}: removed WebDocument: ${webDocumentId}`,
    )
    delete this.webDocuments[webDocumentId]
    this.webDocumentResumeReadyIds.delete(webDocumentId)
    this.rejectSabPairWaitersForWebDocument(webDocumentId, closeErr)
    this.rejectWebRtcBridgeWaitersForWebDocument(webDocumentId, closeErr)
    this.rejectResumeReadyWaiters(
      webDocumentId,
      new Error(
        `WebDocumentTracker: ${this.clientUuid}: WebDocument ${webDocumentId} closed before resume-ready`,
      ),
    )

    const wasActiveRuntimeDocument =
      this.activeRuntimeWebDocumentId === webDocumentId
    if (wasActiveRuntimeDocument) {
      delete this.activeRuntimeWebDocumentId
      this.activeRuntimeDocumentAbort?.abort()
      this.activeRuntimeDocumentAbort = undefined
    }

    const remainingWebDocumentIds = Object.keys(this.webDocuments)
    if (
      this.lastWebDocumentId === webDocumentId ||
      !this.lastWebDocumentId ||
      !this.webDocuments[this.lastWebDocumentId]
    ) {
      const nextWebDocumentId =
        remainingWebDocumentIds[remainingWebDocumentIds.length - 1]
      this.lastWebDocumentId = nextWebDocumentId
      this.lastWebDocumentIdx = nextWebDocumentId
        ? remainingWebDocumentIds.indexOf(nextWebDocumentId)
        : 0
    }

    const shouldCloseRuntimeClient =
      !remainingWebDocumentIds.length || wasActiveRuntimeDocument
    if (!remainingWebDocumentIds.length) {
      this.lastWebDocumentId = undefined
      this.lastWebDocumentIdx = 0
    }
    if (shouldCloseRuntimeClient) {
      this.webRuntimeClient.close()
    }

    if (!remainingWebDocumentIds.length && this.onAllWebDocumentsClosed) {
      await this.onAllWebDocumentsClosed()
    }
  }

  private async waitForActiveWebDocumentResumeReady(): Promise<RuntimeClientStreamOpenGateResult> {
    // Incoming runtime streams must wait on the WebDocument relaying the
    // runtime channel. A newer tab may be connected but hidden or still
    // resuming, and it must not gate streams for an older active relay.
    const webDocumentId =
      this.activeRuntimeWebDocumentId ?? this.lastWebDocumentId
    if (!webDocumentId || !this.webDocuments[webDocumentId]) {
      return {
        state: 'unavailable',
        reason: 'no active WebDocument',
      }
    }
    if (this.webDocumentResumeReadyIds.has(webDocumentId)) {
      return {
        state: 'ready',
        documentId: webDocumentId,
      }
    }

    return new Promise<RuntimeClientStreamOpenGateResult>((resolve) => {
      this.webDocumentResumeReadyWaiters.push({
        webDocumentId,
        resume: () => resolve({ state: 'ready', documentId: webDocumentId }),
        reject: (err) =>
          resolve({
            state: 'closed',
            documentId: webDocumentId,
            reason: err.message,
          }),
      })
    })
  }

  // rejectWaiters rejects all pending WebDocument waiters.
  private rejectWaiters(err: Error) {
    const waiters = this.webDocumentWaiters.splice(0)
    for (const waiter of waiters) {
      waiter.reject(err)
    }
  }

  private resolveResumeReadyWaiters(webDocumentId: string) {
    const waiters = this.webDocumentResumeReadyWaiters
    this.webDocumentResumeReadyWaiters = waiters.filter((waiter) => {
      if (waiter.webDocumentId !== webDocumentId) {
        return true
      }
      waiter.resume()
      return false
    })
  }

  private rejectResumeReadyWaiters(webDocumentId: string, err: Error) {
    const waiters = this.webDocumentResumeReadyWaiters
    this.webDocumentResumeReadyWaiters = waiters.filter((waiter) => {
      if (waiter.webDocumentId !== webDocumentId) {
        return true
      }
      waiter.reject(err)
      return false
    })
  }

  private rejectAllResumeReadyWaiters(err: Error) {
    const waiters = this.webDocumentResumeReadyWaiters.splice(0)
    for (const waiter of waiters) {
      waiter.reject(err)
    }
  }

  private rejectSabPairWaitersForWebDocument(
    webDocumentId: string,
    err: Error,
  ) {
    for (const [requestId, waiter] of this.sabPairOpenWaiters) {
      if (waiter.webDocumentId !== webDocumentId) {
        continue
      }
      this.sabPairOpenWaiters.delete(requestId)
      waiter.reject(err)
    }
  }

  private rejectAllSabPairWaiters(err: Error) {
    const waiters = Array.from(this.sabPairOpenWaiters.values())
    this.sabPairOpenWaiters.clear()
    for (const waiter of waiters) {
      waiter.reject(err)
    }
  }

  private removeWebRtcBridgeWaiter(waiter: WebRtcBridgeOpenWaiter) {
    const idx = this.webRtcBridgeOpenWaiters.indexOf(waiter)
    if (idx !== -1) {
      this.webRtcBridgeOpenWaiters.splice(idx, 1)
    }
  }

  private rejectWebRtcBridgeWaitersForWebDocument(
    webDocumentId: string,
    err: Error,
  ) {
    const waiters = this.webRtcBridgeOpenWaiters
    this.webRtcBridgeOpenWaiters = waiters.filter((waiter) => {
      if (waiter.webDocumentId !== webDocumentId) {
        return true
      }
      waiter.reject(err)
      return false
    })
  }

  private rejectAllWebRtcBridgeWaiters(err: Error) {
    const waiters = this.webRtcBridgeOpenWaiters.splice(0)
    for (const waiter of waiters) {
      waiter.reject(err)
    }
  }
}

function isAbortError(err: unknown): boolean {
  return (
    typeof err === 'object' &&
    err !== null &&
    'name' in err &&
    (err as { name?: string }).name === 'AbortError'
  )
}

function isExpectedWebDocumentCloseError(err: unknown): boolean {
  const msg = err instanceof Error ? err.message : String(err)
  return (
    msg.includes('closed while waiting for WebDocument') ||
    msg.includes('disconnected before ack')
  )
}
