/**
 * saucer.ts provides the interface for JS to communicate with the Go runtime
 * via the C++ Saucer process. Uses a single yamux-multiplexed connection over
 * two HTTP endpoints routed through the bldr:// custom scheme:
 *
 *   /b/saucer/{docId}/mux  - GET: streaming response (Go -> JS yamux frames)
 *   /b/saucer/{docId}/mux  - POST: send yamux frames (JS -> Go)
 *
 * The page URL contains the webDocumentId as a query parameter:
 *   bldr:///index.html?webDocumentId={uuid}
 */

import { pipe } from 'it-pipe'
import {
  Client,
  HandleStreamFunc,
  StreamConn,
  openRpcStream,
  combineUint8ArrayListTransform,
} from 'starpc'
import { Uint8ArrayList } from 'uint8arraylist'
import { WebRuntimeClientType } from '../runtime/runtime.pb.js'
import { WebRuntimeHostClient } from '../runtime/runtime_srpc.pb.js'
import browserReadableStreamToIt from '../fetch/readablestream-to-it.js'

declare global {
  const BLDR_SAUCER: boolean | undefined
}

/**
 * isSaucer is true when running inside a Saucer webview.
 */
export const isSaucer =
  typeof BLDR_SAUCER !== 'undefined' ||
  (typeof window !== 'undefined' && window.location?.protocol === 'bldr:')

/**
 * getDocId returns the web document ID from the URL query parameters.
 * Uses manual parsing instead of new URL() because bldr: is not a standard
 * scheme and URL parsing may fail or return empty searchParams.
 */
function getDocId(): string {
  if (typeof window === 'undefined') return ''
  const search = window.location.search || ''
  const match = search.match(/[?&]webDocumentId=([^&]+)/)
  return match?.[1] || ''
}

/**
 * SaucerRuntimeClient communicates with the Go runtime via a yamux-multiplexed
 * connection over a single pair of HTTP endpoints. All RPC streams are
 * multiplexed over this single connection instead of using per-stream HTTP requests.
 */
export class SaucerRuntimeClient {
  public readonly rpcClient: Client
  public readonly runtimeHost: WebRuntimeHostClient

  private documentConnected = false
  private connectPromise: Promise<void> | null = null
  private muxConn: StreamConn | null = null
  private abortController: AbortController | null = null
  private unloadHandler: (() => void) | null = null

  constructor(
    public readonly webRuntimeId: string,
    public readonly clientId: string,
    public readonly clientType: WebRuntimeClientType,
    private handleIncomingStream: HandleStreamFunc | null,
  ) {
    this.rpcClient = new Client(this.openStream.bind(this))
    this.runtimeHost = new WebRuntimeHostClient(this.rpcClient)

    if (typeof window !== 'undefined') {
      this.unloadHandler = () => this.resetState()
      window.addEventListener('beforeunload', this.unloadHandler)
    }
  }

  // waitConn waits for the connection to be ready.
  public async waitConn(): Promise<void> {
    await this.connectDocument()
  }

  // openStream opens an RPC stream to the Go runtime via yamux.
  public async openStream() {
    await this.connectDocument()
    if (!this.muxConn) {
      throw new Error('saucer: mux not connected')
    }
    return this.muxConn.openStream()
  }

  // openWebDocumentHostStream opens a stream wrapped in WebRuntimeHost.WebDocumentRpc rpcstream.
  public openWebDocumentHostStream(docUuid: string) {
    return openRpcStream(
      docUuid,
      this.runtimeHost.WebDocumentRpc.bind(this.runtimeHost),
    )
  }

  // close closes the client and cleans up resources.
  public close(): void {
    this.resetState()
    if (this.unloadHandler && typeof window !== 'undefined') {
      window.removeEventListener('beforeunload', this.unloadHandler)
      this.unloadHandler = null
    }
  }

  // resetState resets connection state.
  private resetState(): void {
    this.documentConnected = false
    this.connectPromise = null
    this.muxConn?.close()
    this.muxConn = null
    this.abortController?.abort()
    this.abortController = null
  }

  // connectDocument establishes the yamux mux connection to Go.
  private async connectDocument(): Promise<void> {
    if (this.documentConnected) return
    if (this.connectPromise) return this.connectPromise

    const docId = getDocId()
    if (!docId) throw new Error('saucer: webDocumentId not found in URL')

    this.connectPromise = (async () => {
      console.log('[saucer] Connecting mux:', docId)
      this.abortController = new AbortController()

      // Open the streaming GET for reading yamux frames from Go.
      const muxUrl = `/b/saucer/${docId}/mux`
      const maxAttempts = 20
      const baseDelay = 100
      let resp: Response | null

      for (let attempt = 0; ; attempt++) {
        resp = await fetch(muxUrl, {
          signal: this.abortController?.signal,
        }).catch(() => null)
        if (resp?.ok && resp.body) break
        if (attempt >= maxAttempts - 1) {
          throw new Error(
            `saucer: mux connect failed after ${maxAttempts} attempts`,
          )
        }
        const delay = Math.min(baseDelay * Math.pow(2, attempt), 5000)
        console.log(
          `[saucer] Mux connect attempt ${attempt + 1} failed, retrying in ${delay}ms`,
        )
        await new Promise((resolve) => setTimeout(resolve, delay))
      }

      if (!resp?.body) {
        throw new Error('saucer: mux response has no body')
      }

      // Build a server handler for incoming streams from Go.
      const incomingHandler = this.handleIncomingStream
      const server = incomingHandler
        ? {
            handlePacketStream: (strm: Parameters<HandleStreamFunc>[0]) => {
              incomingHandler(strm).catch((err: Error) => {
                console.error('[saucer] Incoming stream error:', err)
              })
            },
          }
        : undefined

      // Create the yamux StreamConn.
      // JS is outbound (client), Go is inbound (server).
      const conn = new StreamConn(server, {
        direction: 'outbound',
        yamuxParams: {
          enableKeepAlive: false,
          maxMessageSize: 32 * 1024,
        },
      })
      this.muxConn = conn

      // Build a duplex from the streaming GET (source) and POST sink.
      const readSource = browserReadableStreamToIt(resp.body)

      // The sink sends yamux frames to Go via POST requests.
      const writeSink = this.buildPostSink(muxUrl, this.abortController)

      // readSource -> conn.sink (feed yamux input)
      // conn.source -> combineUint8ArrayListTransform -> writeSink (send yamux output)
      pipe(readSource, conn, combineUint8ArrayListTransform(), writeSink)
        .catch((err: Error) => {
          if (err.name !== 'AbortError') {
            console.error('[saucer] Mux pipe error:', err)
          }
        })

      this.documentConnected = true
      console.log('[saucer] Mux connected:', docId)
    })()

    try {
      await this.connectPromise
    } finally {
      this.connectPromise = null
    }
  }

  // buildPostSink returns an async sink that sends chunks to Go via POST.
  private buildPostSink(
    muxUrl: string,
    abortController: AbortController,
  ): (source: AsyncIterable<Uint8Array | Uint8ArrayList>) => Promise<void> {
    return async (source) => {
      for await (const chunk of source) {
        if (abortController.signal.aborted) break
        const data =
          chunk instanceof Uint8Array
            ? chunk
            : chunk instanceof Uint8ArrayList
              ? chunk.subarray()
              : new Uint8Array(chunk as ArrayBuffer)
        const resp = await fetch(muxUrl, {
          method: 'POST',
          body: data as unknown as BodyInit,
          signal: abortController.signal,
        })
        if (!resp.ok) {
          throw new Error(`POST ${muxUrl} failed: ${resp.status}`)
        }
      }
    }
  }
}
