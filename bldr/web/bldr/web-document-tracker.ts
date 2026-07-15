import { HandleStreamFunc } from 'starpc'
import { Message } from '@aptre/protobuf-es-lite'

import {
  buildWebDocumentLockName,
  ClientToWebDocument,
  ConnectWebRuntimeAck,
  OpenOpfsWorkerAck,
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

interface WebDocumentRuntimeConnectedWaiter extends WebDocumentWaiter {
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

interface OpfsWorkerOpenWaiter {
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
  // webDocumentResumeReadyIds are WebDocuments that reported foreground readiness.
  private webDocumentResumeReadyIds = new Set<string>()
  // webDocumentRuntimeConnectedIds are WebDocuments with a live runtime channel.
  private webDocumentRuntimeConnectedIds = new Set<string>()
  // webDocumentRuntimeConnectedWaiters wait on a specific WebDocument relay.
  private webDocumentRuntimeConnectedWaiters: WebDocumentRuntimeConnectedWaiter[] =
    []
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
  private nextWebRtcBridgeRequestNumber = 1
  private nextOpfsWorkerRequestNumber = 1
  private sabPairEndpoints = new Map<string, SabPairEndpointDescriptor>()
  private webRtcBridgeOpenWaiters = new Map<string, WebRtcBridgeOpenWaiter>()
  private opfsWorkerOpenWaiters = new Map<string, OpfsWorkerOpenWaiter>()
  // opfsWorkerHostId is the WebDocument currently hosting the OPFS bridge worker.
  private opfsWorkerHostId?: string

  constructor(
    clientUuid: string,
    clientType: WebRuntimeClientType,
    private readonly onWebDocumentsExhausted: () => Promise<void>,
    handleIncomingStream: HandleStreamFunc | null,
    private readonly onAllWebDocumentsClosed?:
      | (() => Promise<void> | void)
      | null,
    logicalClientId?: string,
    // onOpfsBridgeLost fires when the WebDocument hosting the OPFS bridge worker
    // is removed, or the broker reports that worker died. The owner re-hosts the
    // bridge. Removing a non-host WebDocument does not fire it, so an unrelated
    // tab close never invalidates a healthy mounted volume's handles.
    private readonly onOpfsBridgeLost?: (() => Promise<void> | void) | null,
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
      this.waitForActiveWebDocumentRuntimeConnected.bind(this),
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
        this.removeWebDocument(
          webDocumentId,
          closeErr,
          data.terminal === true,
        ).catch((err) => {
          console.error(
            `WebDocumentTracker: ${this.clientUuid}: error handling WebDocument close:`,
            err,
          )
        })
        return
      }

      if (data.resumeReady === true) {
        this.webDocumentResumeReadyIds.add(webDocumentId)
        this.preferReadyServiceWorkerDocument(webDocumentId)
      }
      if (data.resumeReady === false) {
        this.webDocumentResumeReadyIds.delete(webDocumentId)
        if (this.preferredRuntimeWebDocumentId === webDocumentId) {
          delete this.preferredRuntimeWebDocumentId
        }
      }

      if (data.runtimeConnected === true) {
        this.webDocumentRuntimeConnectedIds.add(webDocumentId)
        this.resolveRuntimeConnectedWaiters(webDocumentId)
        const waiters = this.webDocumentWaiters.splice(0)
        for (const waiter of waiters) {
          waiter.resume()
        }
      }
      if (data.runtimeConnected === false) {
        this.webDocumentRuntimeConnectedIds.delete(webDocumentId)
        if (this.preferredRuntimeWebDocumentId === webDocumentId) {
          delete this.preferredRuntimeWebDocumentId
        }
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
        const requestId = data.requestId
        const waiter = requestId
          ? this.webRtcBridgeOpenWaiters.get(requestId)
          : undefined
        if (!requestId || !waiter) {
          data.bridgePort.close()
          console.warn(
            `WebDocumentTracker: ${this.clientUuid}: unknown WebRTC bridge ack from ${webDocumentId}`,
          )
          return
        }
        this.webRtcBridgeOpenWaiters.delete(requestId)
        waiter.resolve(data.bridgePort)
        return
      }

      if (data.openOpfsWorkerAck) {
        this.handleOpenOpfsWorkerAck(
          webDocumentId,
          data.openOpfsWorkerAck,
          ev.ports?.[0],
        )
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

      if (data.opfsWorkerClosed) {
        this.opfsWorkerHostId = undefined
        void this.onOpfsBridgeLost?.()
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
    this.webRuntimeClient.close()
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
    this.webDocumentRuntimeConnectedIds.clear()
    delete this.lastWebDocumentId
    delete this.activeRuntimeWebDocumentId
    this.activeRuntimeDocumentAbort?.abort()
    this.activeRuntimeDocumentAbort = undefined
    const err = new Error(
      `WebDocumentTracker: ${this.clientUuid}: closed while waiting for WebDocument`,
    )
    this.rejectWaiters(err)
    this.rejectAllRuntimeConnectedWaiters(err)
    this.rejectAllSabPairWaiters(err)
    this.rejectAllWebRtcBridgeWaiters(err)
    this.rejectAllOpfsWorkerWaiters(err)
  }

  // postMessage posts a message to all connected web documents.
  public postMessage(msg: ClientToWebDocument) {
    for (const docID in this.webDocuments) {
      this.webDocuments[docID]?.postMessage(msg)
    }
  }

  public openWebRuntimePort(
    init: Uint8Array,
    excludedWebDocumentId?: string,
    signal?: AbortSignal,
  ): Promise<MessagePort> {
    return this.openWebRuntimeClient(
      WebRuntimeClientInit.fromBinary(init),
      excludedWebDocumentId,
      signal,
    )
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
  // for the next relay (driven by WebDocument attach and runtime-connected
  // events, no polling) and retry instead of failing the fetch.
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
          // Relay still absent: re-arm for the next attach / runtime-connected event.
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

  // waitForRuntimeClientReady resolves true once the shared runtime client
  // channel reconnects, or false if timeoutMs elapses or the tracker closes.
  // After the relaying WebDocument closes, removeWebDocument reroutes the client
  // through a surviving document; a plugin-asset fetch that failed on the stale
  // route waits for that reconnect to land before retrying instead of failing
  // the navigation. waitConn drives and shares the reconnect, so a lost relay
  // timer never beats a connect that is already in flight.
  public async waitForRuntimeClientReady(timeoutMs: number): Promise<boolean> {
    if (this.closed) {
      return false
    }
    let timer: ReturnType<typeof setTimeout> | undefined
    const timeout = new Promise<boolean>((resolve) => {
      timer = setTimeout(() => resolve(false), timeoutMs)
    })
    const ready = this.webRuntimeClient
      .waitConn()
      .then(() => true)
      .catch(() => false)
    try {
      return await Promise.race([ready, timeout])
    } finally {
      if (timer) {
        clearTimeout(timer)
      }
    }
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

    const requestId = `webrtc-bridge-open-${this.nextWebRtcBridgeRequestNumber++}`
    return new Promise<MessagePort | null>((resolve, reject) => {
      this.webRtcBridgeOpenWaiters.set(requestId, {
        webDocumentId: docId,
        resolve,
        reject,
      })
      const msg: ClientToWebDocument = {
        from: this.clientUuid,
        connectWebRtcBridge: { requestId },
      }
      try {
        docPort.postMessage(msg)
      } catch (err) {
        this.webRtcBridgeOpenWaiters.delete(requestId)
        reject(err instanceof Error ? err : new Error(String(err)))
      }
    })
  }

  // requestOpfsWorker requests a DedicatedWorker OPFS bridge from the first
  // available WebDocument. The returned MessagePort speaks the raw OPFS protocol.
  public async requestOpfsWorker(): Promise<MessagePort | null> {
    const webDocumentIds = Object.keys(this.webDocuments)
    if (!webDocumentIds.length) return null

    const docId = this.lastWebDocumentId ?? webDocumentIds[0]
    const docPort = this.webDocuments[docId]
    if (!docPort) return null

    const requestId = `opfs-worker-open-${this.nextOpfsWorkerRequestNumber++}`
    return new Promise<MessagePort | null>((resolve, reject) => {
      this.opfsWorkerOpenWaiters.set(requestId, {
        webDocumentId: docId,
        resolve,
        reject,
      })
      const msg: ClientToWebDocument = {
        from: this.clientUuid,
        openOpfsWorker: { requestId },
      }
      try {
        docPort.postMessage(msg)
      } catch (err) {
        this.opfsWorkerOpenWaiters.delete(requestId)
        reject(err instanceof Error ? err : new Error(String(err)))
      }
    })
  }

  // openWebRuntimeClient attempts to open a client via one of the WebDocuments.
  private async openWebRuntimeClient(
    initMsg: Message<WebRuntimeClientInit>,
    excludedWebDocumentId?: string,
    signal?: AbortSignal,
  ): Promise<MessagePort> {
    if (signal?.aborted) {
      throw signal.reason instanceof Error
        ? signal.reason
        : new DOMException('runtime host relay aborted', 'AbortError')
    }
    if (this.closed) {
      throw new Error(
        `WebDocumentTracker: ${this.clientUuid}: closed while waiting for WebDocument`,
      )
    }
    const init = WebRuntimeClientInit.toBinary(initMsg)
    const usePreferredOrder = !!(
      this.preferredRuntimeWebDocumentId &&
      this.preferredRuntimeWebDocumentId !== excludedWebDocumentId &&
      this.webDocuments[this.preferredRuntimeWebDocumentId]
    )
    const webDocumentIds = this.orderRuntimeOpenWebDocuments(
      Object.keys(this.webDocuments),
    ).filter((webDocumentId) => webDocumentId !== excludedWebDocumentId)
    const attemptedWebDocumentIds = new Set<string>()
    for (const i of webDocumentIds.keys()) {
      const x = usePreferredOrder
        ? i
        : (i + this.lastWebDocumentIdx + 1) % webDocumentIds.length
      const webDocumentId = webDocumentIds[x]
      attemptedWebDocumentIds.add(webDocumentId)
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
        if (!expectedClose) {
          this.webDocumentRuntimeConnectedIds.delete(webDocumentId)
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

    const shouldRetryExistingWebDocument = Object.keys(this.webDocuments).some(
      (webDocumentId) =>
        webDocumentId !== excludedWebDocumentId &&
        (!attemptedWebDocumentIds.has(webDocumentId) ||
          this.webDocumentRuntimeConnectedIds.has(webDocumentId)),
    )
    if (shouldRetryExistingWebDocument) {
      return this.openWebRuntimeClient(
        initMsg,
        excludedWebDocumentId,
        signal,
      )
    }

    const hasAvailableWebDocument = Object.keys(this.webDocuments).some(
      (webDocumentId) => webDocumentId !== excludedWebDocumentId,
    )
    if (hasAvailableWebDocument) {
      console.log(
        'ServiceWorker: waiting for existing WebDocument to become ready',
      )
      return this.waitForWebDocument(
        () =>
          this.openWebRuntimeClient(
            initMsg,
            excludedWebDocumentId,
            signal,
          ),
        signal,
      )
    }

    const waitPromise = this.waitForWebDocument(
      () =>
        this.openWebRuntimeClient(initMsg, excludedWebDocumentId, signal),
      signal,
    )

    void waitPromise.catch(() => {})

    // Rebuild the replacement ServiceWorker's in-memory tracker from live
    // documents. The elected DedicatedWorker lease can outlive this worker.
    await this.onWebDocumentsExhausted()

    console.log('ServiceWorker: waiting for next WebDocument to proxy conn')
    return waitPromise
  }

  private waitForWebDocument<T>(
    resume: () => Promise<T>,
    signal?: AbortSignal,
  ): Promise<T> {
    return new Promise<T>((resolve, reject) => {
      let settled = false
      const onAbort = () => {
        if (!settle()) {
          return
        }
        const reason = signal?.reason
        reject(
          reason instanceof Error
            ? reason
            : new DOMException('runtime host relay aborted', 'AbortError'),
        )
      }
      const waiter: WebDocumentWaiter = {
        resume: () => {
          if (!settle()) {
            return
          }
          resolve(resume())
        },
        reject: (err) => {
          if (!settle()) {
            return
          }
          reject(err)
        },
      }
      const settle = () => {
        if (settled) {
          return false
        }
        settled = true
        signal?.removeEventListener('abort', onAbort)
        const idx = this.webDocumentWaiters.indexOf(waiter)
        if (idx !== -1) {
          this.webDocumentWaiters.splice(idx, 1)
        }
        return true
      }
      if (signal?.aborted) {
        onAbort()
        return
      }
      signal?.addEventListener('abort', onAbort, { once: true })
      this.webDocumentWaiters.push(waiter)
    })
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
    terminal = false,
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
    this.webDocumentRuntimeConnectedIds.delete(webDocumentId)
    this.rejectSabPairWaitersForWebDocument(webDocumentId, closeErr)
    this.rejectWebRtcBridgeWaitersForWebDocument(webDocumentId, closeErr)
    this.rejectOpfsWorkerWaitersForWebDocument(webDocumentId, closeErr)
    this.rejectRuntimeConnectedWaiters(
      webDocumentId,
      new Error(
        `WebDocumentTracker: ${this.clientUuid}: WebDocument ${webDocumentId} closed before runtime-connected`,
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

    // Re-host the OPFS bridge only when the document that hosted it is the one
    // removed. Removing any other document leaves the bridge worker and its
    // handle id space intact, so a mounted volume keeps its cached handles
    // instead of breaking with "remote OPFS handle is stale" on the next op.
    if (
      remainingWebDocumentIds.length &&
      this.opfsWorkerHostId === webDocumentId &&
      this.onOpfsBridgeLost
    ) {
      this.opfsWorkerHostId = undefined
      Promise.resolve(this.onOpfsBridgeLost()).catch((err: unknown) => {
        console.error(
          `WebDocumentTracker: ${this.clientUuid}: error re-hosting OPFS bridge after host removal:`,
          err,
        )
      })
    }

    // In the DedicatedWorker host fallback only the elected host document
    // accepts a connectWebRuntime relay; every other document rejects it. So a
    // successful relay open always tracks the host as activeRuntimeWebDocumentId,
    // which makes wasActiveRuntimeDocument the host-lost signal. Keep that
    // coupling: the tracker cannot otherwise identify the elected host, since
    // election happens behind a Web Lock inside the documents.
    if (wasActiveRuntimeDocument) {
      this.notifyDedicatedRuntimeHostLost(webDocumentId, closeErr)
    }

    if (!remainingWebDocumentIds.length) {
      this.lastWebDocumentId = undefined
      this.lastWebDocumentIdx = 0
      // The last relay closed. A terminal close is a runtime-directed discard of
      // this worker (CreateWebWorker replacement or RemoveWebWorker removal): no
      // WebDocument will ever relay to this orphaned worker again, so tear the
      // tracker down now. Closing the client fails its active streams, and
      // closing the tracker also rejects every caller parked waiting for the
      // next WebDocument and refuses future opens, so orphaned work fails fast
      // instead of hanging until worker reclamation. A non-terminal close is a
      // reload transition: keep the logical client alive so in-flight work can
      // reconnect through the next WebDocument instead of failing with a
      // terminal close. The close message carries the discard intent because
      // these two cases are otherwise the same signal; without it either a
      // fast-fail breaks reload recovery or a keep-alive hangs orphaned workers
      // for the teardown-latency window.
      if (wasActiveRuntimeDocument) {
        if (terminal) {
          this.close()
        } else {
          this.webRuntimeClient.rerouteChannel().catch((err: unknown) => {
            console.error(
              `WebDocumentTracker: ${this.clientUuid}: error rerouting runtime client:`,
              err,
            )
          })
        }
      }
    } else if (wasActiveRuntimeDocument) {
      // The relaying WebDocument closed but other documents remain. Drop the
      // stale route and reconnect the shared runtime client through a surviving
      // document instead of tearing it down. Closing the client here would
      // surface RuntimeClientClosedError on every in-flight and subsequent
      // plugin-asset fetch the surviving documents relay, failing their
      // navigation. This honors the tracker contract: retry when the owning
      // WebDocument lifecycle closes a stale route.
      this.webRuntimeClient.rerouteChannel().catch((err: unknown) => {
        console.error(
          `WebDocumentTracker: ${this.clientUuid}: error rerouting runtime client:`,
          err,
        )
      })
    }

    if (!remainingWebDocumentIds.length && this.onAllWebDocumentsClosed) {
      await this.onAllWebDocumentsClosed()
    }
  }

  private notifyDedicatedRuntimeHostLost(
    webDocumentId: string,
    err?: Error,
  ): void {
    const msg: ClientToWebDocument = {
      from: this.clientUuid,
      dedicatedRuntimeHostLost: {
        webDocumentId,
        reason: err?.message,
      },
    }
    for (const doc of Object.values(this.webDocuments)) {
      doc.postMessage(msg)
    }
  }

  private async waitForActiveWebDocumentRuntimeConnected(): Promise<RuntimeClientStreamOpenGateResult> {
    // Stream opens gate on the active relay's runtime connection. Foreground
    // resume-ready is telemetry and ServiceWorker preference only.
    const webDocumentId =
      this.activeRuntimeWebDocumentId ?? this.lastWebDocumentId
    if (!webDocumentId || !this.webDocuments[webDocumentId]) {
      return {
        state: 'unavailable',
        reason: 'no active WebDocument',
      }
    }
    if (this.webDocumentRuntimeConnectedIds.has(webDocumentId)) {
      return {
        state: 'ready',
        documentId: webDocumentId,
      }
    }

    return new Promise<RuntimeClientStreamOpenGateResult>((resolve) => {
      this.webDocumentRuntimeConnectedWaiters.push({
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

  private resolveRuntimeConnectedWaiters(webDocumentId: string) {
    const waiters = this.webDocumentRuntimeConnectedWaiters
    this.webDocumentRuntimeConnectedWaiters = waiters.filter((waiter) => {
      if (waiter.webDocumentId !== webDocumentId) {
        return true
      }
      waiter.resume()
      return false
    })
  }

  private rejectRuntimeConnectedWaiters(webDocumentId: string, err: Error) {
    const waiters = this.webDocumentRuntimeConnectedWaiters
    this.webDocumentRuntimeConnectedWaiters = waiters.filter((waiter) => {
      if (waiter.webDocumentId !== webDocumentId) {
        return true
      }
      waiter.reject(err)
      return false
    })
  }

  private rejectAllRuntimeConnectedWaiters(err: Error) {
    const waiters = this.webDocumentRuntimeConnectedWaiters.splice(0)
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

  private handleOpenOpfsWorkerAck(
    webDocumentId: string,
    ack: OpenOpfsWorkerAck,
    port?: MessagePort,
  ): void {
    const waiter = this.opfsWorkerOpenWaiters.get(ack.requestId)
    if (!waiter) {
      port?.close()
      console.warn(
        `WebDocumentTracker: ${this.clientUuid}: unknown OPFS worker ack from ${webDocumentId}`,
      )
      return
    }
    this.opfsWorkerOpenWaiters.delete(ack.requestId)
    if (ack.error) {
      port?.close()
      waiter.reject(new Error(ack.error))
      return
    }
    if (!port) {
      waiter.reject(new Error('OPFS worker open ack missing port'))
      return
    }
    this.opfsWorkerHostId = webDocumentId
    waiter.resolve(port)
  }

  private rejectOpfsWorkerWaitersForWebDocument(
    webDocumentId: string,
    err: Error,
  ) {
    for (const [requestId, waiter] of this.opfsWorkerOpenWaiters) {
      if (waiter.webDocumentId !== webDocumentId) {
        continue
      }
      this.opfsWorkerOpenWaiters.delete(requestId)
      waiter.reject(err)
    }
  }

  private rejectAllOpfsWorkerWaiters(err: Error) {
    const waiters = Array.from(this.opfsWorkerOpenWaiters.values())
    this.opfsWorkerOpenWaiters.clear()
    for (const waiter of waiters) {
      waiter.reject(err)
    }
  }

  private rejectWebRtcBridgeWaitersForWebDocument(
    webDocumentId: string,
    err: Error,
  ) {
    for (const [requestId, waiter] of this.webRtcBridgeOpenWaiters) {
      if (waiter.webDocumentId !== webDocumentId) {
        continue
      }
      this.webRtcBridgeOpenWaiters.delete(requestId)
      waiter.reject(err)
    }
  }

  private rejectAllWebRtcBridgeWaiters(err: Error) {
    const waiters = Array.from(this.webRtcBridgeOpenWaiters.values())
    this.webRtcBridgeOpenWaiters.clear()
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
