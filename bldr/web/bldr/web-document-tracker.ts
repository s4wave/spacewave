import { HandleStreamFunc } from 'starpc'
import { Message } from '@aptre/protobuf-es-lite'

import {
  buildWebDocumentLockName,
  ClientToWebDocument,
  ConnectWebRtcBridgeAck,
  ConnectWebRuntimeAck,
  WebDocumentToClient,
  WebDocumentToWorker,
} from '../runtime/runtime.js'
import {
  WebRuntimeClientInit,
  WebRuntimeClientType,
} from '../runtime/runtime.pb.js'
import { timeoutPromise } from './timeout.js'
import { WebRuntimeClient } from './web-runtime-client.js'

const openViaWebDocumentTimeoutMs = 1000
const waitForNextWebDocumentTimeoutMs = 3000

interface WebDocumentWaiter {
  resume: () => void
  reject: (err: Error) => void
}

// WebDocumentTracker is a tracks a set of connected WebDocument and attempts to
// connect to the remote WebRuntime via these documents, retrying if the remote
// document(s) have been closed or are unreachable after a timeout.
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
  // webDocumentWaiters are callbacks waiting for the next WebDocument.
  private webDocumentWaiters: WebDocumentWaiter[] = []
  // lastWebDocumentIdx was the last index used from WebDocuments.
  private lastWebDocumentIdx = 0
  // lastWebDocumentId was the last web document id used from WebDocuments.
  private lastWebDocumentId?: string

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
    )
  }

  // waitConn opens and waits for the connection to be ready.
  public async waitConn() {
    return this.webRuntimeClient.waitConn()
  }

  // handleWebDocumentMessage handles an incoming message from the WebDocument.
  public handleWebDocumentMessage(msg: WebDocumentToWorker) {
    if (typeof msg !== 'object' || !msg.from || !msg.initPort) {
      return
    }

    const { from: webDocumentId, initPort: port } = msg
    console.log(
      `WebDocumentTracker: ${this.clientUuid}: added WebDocument: ${webDocumentId}`,
    )

    this.webDocuments[webDocumentId] = port
    port.onmessage = (ev) => {
      const data: WebDocumentToClient = ev.data
      if (typeof data !== 'object') {
        return
      }

      if (data.close) {
        void (async () => {
          const closePort = this.webDocuments[webDocumentId]
          if (closePort) {
            closePort.close()
            console.log(
              `WebDocumentTracker: ${this.clientUuid}: removed WebDocument: ${webDocumentId}`,
            )
            delete this.webDocuments[webDocumentId]
            if (this.lastWebDocumentId === webDocumentId) {
              this.lastWebDocumentId = undefined
              this.lastWebDocumentIdx = 0
              this.webRuntimeClient.close()
            }
            if (
              !Object.keys(this.webDocuments).length &&
              this.onAllWebDocumentsClosed
            ) {
              await this.onAllWebDocumentsClosed()
            }
          }
        })().catch((err) => {
          console.error(
            `WebDocumentTracker: ${this.clientUuid}: error handling WebDocument close:`,
            err,
          )
        })
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
    const msg: ClientToWebDocument = {
      from: this.clientUuid,
      close: true,
    }
    for (const docID in this.webDocuments) {
      const doc = this.webDocuments[docID]
      doc.postMessage(msg)
      delete this.webDocuments[docID]
    }
    delete this.lastWebDocumentId
    this.rejectWaiters(
      new Error(
        `WebDocumentTracker: ${this.clientUuid}: closed while waiting for WebDocument`,
      ),
    )
  }

  // postMessage posts a message to all connected web documents.
  public postMessage(msg: ClientToWebDocument) {
    for (const docID in this.webDocuments) {
      this.webDocuments[docID]?.postMessage(msg)
    }
  }

  // requestWebRtcBridge requests a WebRTC bridge port from the first available
  // WebDocument. Returns the bridge MessagePort, or null if no WebDocument
  // responds within the timeout.
  public async requestWebRtcBridge(): Promise<MessagePort | null> {
    const webDocumentIds = Object.keys(this.webDocuments)
    if (!webDocumentIds.length) return null

    // Use the last connected WebDocument (most likely to be alive).
    const docId = this.lastWebDocumentId ?? webDocumentIds[0]
    const docPort = this.webDocuments[docId]
    if (!docPort) return null

    return new Promise<MessagePort | null>((resolve) => {
      // Temporarily listen for the bridge ack on the initPort.
      const prev = docPort.onmessage
      const timeout = globalThis.setTimeout(() => {
        docPort.onmessage = prev
        resolve(null)
      }, openViaWebDocumentTimeoutMs)

      docPort.onmessage = (ev: MessageEvent) => {
        const data = ev.data
        if (data && data.bridgePort) {
          clearTimeout(timeout)
          docPort.onmessage = prev
          resolve((data as ConnectWebRtcBridgeAck).bridgePort)
          return
        }
        // Forward other messages to the original handler.
        if (prev) prev.call(docPort, ev)
      }

      const msg: ClientToWebDocument = {
        from: this.clientUuid,
        connectWebRtcBridge: true,
      }
      docPort.postMessage(msg)
    })
  }

  // openWebRuntimeClient attempts to open a client via one of the WebDocuments.
  private async openWebRuntimeClient(
    initMsg: Message<WebRuntimeClientInit>,
  ): Promise<MessagePort> {
    const init = WebRuntimeClientInit.toBinary(initMsg)
    const webDocumentIds = Object.keys(this.webDocuments)
    for (let i = 0; i < webDocumentIds.length; i++) {
      const x = (i + this.lastWebDocumentIdx + 1) % webDocumentIds.length
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
        webDocumentPort.postMessage(connectMsg, [ackChannel.port2])

        // wait for the ack.
        const result = await Promise.race([
          ackPromise,
          disconnectedPromise,
          timeoutPromise(openViaWebDocumentTimeoutMs),
        ])
        if (!result) {
          throw new Error('timed out waiting for ack from WebDocument')
        }
        if (result instanceof Error) {
          throw result
        }
        console.log(
          `WebDocumentTracker: ${this.clientUuid}: opened port with WebRuntime via WebDocument: ${webDocumentId}`,
        )
        this.lastWebDocumentIdx = x
        this.lastWebDocumentId = webDocumentId
        return result.webRuntimePort
      } catch (err) {
        // message port must be closed.
        console.error(
          `ServiceWorker: connecting via WebDocument failed: ${webDocumentId}`,
          err,
        )
        delete this.webDocuments[webDocumentId]
        continue
      } finally {
        lockAbortController.abort()
      }
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

    // notify all WebDocument that we are looking for a connection to them.
    await this.onWebDocumentsExhausted()

    console.log('ServiceWorker: waiting for next WebDocument to proxy conn')
    return Promise.race([
      waitPromise,
      timeoutPromise(waitForNextWebDocumentTimeoutMs).then(() => {
        throw new Error('timed out waiting for next WebDocument to proxy conn')
      }),
    ])
  }

  // waitForWebDocumentDisconnect resolves when the web document liveness lock becomes available.
  private waitForWebDocumentDisconnect(
    webDocumentId: string,
    signal: AbortSignal,
  ): Promise<Error | undefined> {
    if (typeof navigator === 'undefined' || !('locks' in navigator)) {
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

  // rejectWaiters rejects all pending WebDocument waiters.
  private rejectWaiters(err: Error) {
    const waiters = this.webDocumentWaiters.splice(0)
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
