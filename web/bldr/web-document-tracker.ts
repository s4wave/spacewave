import { PartialMessage } from '@bufbuild/protobuf'
import {
  ClientToWebDocument,
  ConnectWebRuntimeAck,
  WebDocumentToClient,
  WebDocumentToWorker,
} from '../runtime/runtime.js'
import {
  WebRuntimeClientInit,
  WebRuntimeClientType,
} from '../runtime/runtime_pb.js'
import { timeoutPromise } from './timeout.js'
import { HandleStreamFn, WebRuntimeClient } from './web-runtime-client.js'

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
  private webDocumentWaiters: ((docID: string) => void)[] = []
  // lastWebDocumentIdx was the last index used from WebDocuments.
  private lastWebDocumentIdx = 0
  // lastWebDocumentId was the last web document id used from WebDocuments.
  private lastWebDocumentId?: string

  constructor(
    clientUuid: string,
    clientType: WebRuntimeClientType,
    private readonly onWebDocumentsExhausted: () => Promise<void>,
    handleIncomingStream: HandleStreamFn | null,
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
        }
      }
    }

    for (const waiter of this.webDocumentWaiters) {
      waiter(webDocumentId)
    }
    this.webDocumentWaiters.length = 0

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
  }

  // openWebRuntimeClient attempts to open a client via one of the WebDocuments.
  private async openWebRuntimeClient(
    initMsg: PartialMessage<WebRuntimeClientInit>,
  ): Promise<MessagePort> {
    const init = new WebRuntimeClientInit(initMsg).toBinary()
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
        const result = await Promise.race([ackPromise, timeoutPromise(1000)])
        if (!result) {
          throw new Error('timed out waiting for ack from WebDocument')
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
      }
    }

    // construct a promise to catch any new incoming WebDocument client
    const waitPromise = new Promise<MessagePort>((resolve) => {
      // try again once a new WebDocument is added.
      this.webDocumentWaiters.push(() => {
        resolve(this.openWebRuntimeClient(initMsg))
      })
    })

    // notify all WebDocument that we are looking for a connection to them.
    await this.onWebDocumentsExhausted()

    console.log('ServiceWorker: waiting for next WebDocument to proxy conn')
    return waitPromise
  }
}
