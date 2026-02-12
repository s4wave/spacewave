/**
 * saucer.ts provides the interface for JS to communicate with the C++ Saucer process.
 * Uses HTTP endpoints routed through the bldr:// custom scheme:
 *
 *   /b/saucer/{docId}/connect          - GET: Register document
 *   /b/saucer/{docId}/control          - GET: Persistent control stream
 *   /b/saucer/{docId}/stream/{id}/read - GET: Stream data from C++ to JS
 *   /b/saucer/{docId}/stream/{id}/write - POST: Send data from JS to C++
 *
 * The page URL contains the webDocumentId as a query parameter:
 *   bldr:///index.html?webDocumentId={uuid}
 */

import { Pushable, pushable } from 'it-pushable'
import { Source } from 'it-stream-types'
import { pipe } from 'it-pipe'
import {
  Client,
  PacketStream,
  HandleStreamFunc,
  openRpcStream,
  parseLengthPrefixTransform,
  prependLengthPrefixTransform,
  combineUint8ArrayListTransform,
} from 'starpc'
import { WebRuntimeClientType } from '../runtime/runtime.pb.js'
import { WebRuntimeHostClient } from '../runtime/runtime_srpc.pb.js'

declare global {
  const BLDR_SAUCER: boolean | undefined
}

/**
 * isSaucer is true when running inside a Saucer webview.
 */
export const isSaucer =
  typeof BLDR_SAUCER !== 'undefined' ||
  (typeof window !== 'undefined' && window.location?.protocol === 'bldr:')

// IncomingStreamHandler handles an incoming stream from Go.
type IncomingStreamHandler = (streamId: number, stream: PacketStream) => void

/**
 * getDocId returns the web document ID from the URL query parameters.
 * Uses manual parsing instead of new URL() because bldr: is not a standard
 * scheme and URL parsing may fail or return empty searchParams.
 */
export function getDocId(): string {
  if (typeof window === 'undefined') return ''
  const search = window.location.search || ''
  const match = search.match(/[?&]webDocumentId=([^&]+)/)
  return match?.[1] || ''
}

/**
 * SaucerRuntimeClient provides the same interface as WebRuntimeClient
 * but uses HTTP-based PacketStreams instead of MessagePorts.
 */
export class SaucerRuntimeClient {
  public readonly rpcClient: Client
  public readonly runtimeHost: WebRuntimeHostClient

  private nextStreamId = 1
  private documentConnected = false
  private connectPromise: Promise<void> | null = null
  private controlAbortController: AbortController | null = null
  private incomingStreamHandler: IncomingStreamHandler | null = null
  private unloadHandler: (() => void) | null = null

  constructor(
    public readonly webRuntimeId: string,
    public readonly clientId: string,
    public readonly clientType: WebRuntimeClientType,
    private handleIncomingStream: HandleStreamFunc | null,
  ) {
    this.rpcClient = new Client(this.openStream.bind(this))
    this.runtimeHost = new WebRuntimeHostClient(this.rpcClient)

    // Set up handler for incoming streams from Go.
    this.incomingStreamHandler = (streamId, stream) => {
      if (!this.handleIncomingStream) {
        console.warn('[saucer] No handler for incoming stream:', streamId)
        return
      }
      this.handleIncomingStream(stream).catch((err) => {
        console.error('[saucer] Incoming stream error:', err)
      })
    }

    if (typeof window !== 'undefined') {
      this.unloadHandler = () => this.resetState()
      window.addEventListener('beforeunload', this.unloadHandler)
    }
  }

  // waitConn waits for the connection to be ready.
  public async waitConn(): Promise<void> {
    await this.connectDocument()
  }

  // openStream opens an RPC stream to the Go runtime.
  public async openStream(): Promise<PacketStream> {
    return this.openSaucerStream()
  }

  // openWebDocumentHostStream opens a stream wrapped in WebRuntimeHost.WebDocumentRpc rpcstream.
  // This allows Go to route the inner call to the per-document mux.
  public openWebDocumentHostStream(docUuid: string): Promise<PacketStream> {
    return openRpcStream(
      docUuid,
      this.runtimeHost.WebDocumentRpc.bind(this.runtimeHost),
    )
  }

  // openWebWorkerHostStream opens a stream wrapped in WebRuntimeHost.WebWorkerRpc rpcstream.
  public openWebWorkerHostStream(workerUuid: string): Promise<PacketStream> {
    return openRpcStream(
      workerUuid,
      this.runtimeHost.WebWorkerRpc.bind(this.runtimeHost),
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
    this.nextStreamId = 1
    this.controlAbortController?.abort()
    this.controlAbortController = null
  }

  // connectDocument connects the document to the C++ runtime.
  // Safe to call multiple times - will only connect once.
  private async connectDocument(): Promise<void> {
    if (this.documentConnected) return
    if (this.connectPromise) return this.connectPromise

    const docId = getDocId()
    if (!docId) throw new Error('saucer: webDocumentId not found in URL')

    this.connectPromise = (async () => {
      console.log('[saucer] Connecting document:', docId)

      // Retry with backoff: Go may not be ready to accept yamux streams yet.
      const maxAttempts = 20
      const baseDelay = 100
      for (let attempt = 0; ; attempt++) {
        const resp = await fetch(`/b/saucer/${docId}/connect`).catch(
          () => null,
        )
        if (resp?.ok) break
        if (attempt >= maxAttempts - 1) {
          throw new Error(
            `saucer: connect failed after ${maxAttempts} attempts`,
          )
        }
        const delay = Math.min(baseDelay * Math.pow(2, attempt), 5000)
        console.log(
          `[saucer] Connect attempt ${attempt + 1} failed, retrying in ${delay}ms`,
        )
        await new Promise((resolve) => setTimeout(resolve, delay))
      }

      this.documentConnected = true
      console.log('[saucer] Document connected:', docId)

      this.startControlStream(docId)
    })()

    try {
      await this.connectPromise
    } finally {
      this.connectPromise = null
    }
  }

  // startControlStream starts the control stream to receive notifications from C++.
  private startControlStream(docId: string): void {
    this.controlAbortController = new AbortController()
    const controlUrl = `/b/saucer/${docId}/control`

    ;(async () => {
      try {
        const resp = await fetch(controlUrl, {
          signal: this.controlAbortController?.signal,
        })
        if (!resp.ok || !resp.body) {
          console.error('[saucer] Control stream failed:', resp.status)
          return
        }

        const reader = resp.body.getReader()
        const decoder = new TextDecoder()
        let buffer = ''

        while (true) {
          const { done, value } = await reader.read()
          if (done) break

          buffer += decoder.decode(value, { stream: true })
          const lines = buffer.split('\n')
          buffer = lines.pop() || ''

          for (const line of lines) {
            if (!line.trim()) continue
            try {
              const msg = JSON.parse(line)
              if (msg.type === 'stream' && typeof msg.id === 'number') {
                console.log('[saucer] Incoming stream:', msg.id)
                if (this.incomingStreamHandler) {
                  this.incomingStreamHandler(
                    msg.id,
                    this.createPacketStream(docId, msg.id),
                  )
                }
              }
            } catch (err) {
              console.error('[saucer] Control parse error:', err)
            }
          }
        }
      } catch (err) {
        if ((err as Error).name !== 'AbortError') {
          console.error('[saucer] Control stream error:', err)
        }
      }
    })()
  }

  // openSaucerStream opens a bidirectional PacketStream to the Go runtime via C++.
  private async openSaucerStream(): Promise<PacketStream> {
    const docId = getDocId()
    if (!docId) throw new Error('saucer: webDocumentId not found in URL')

    await this.connectDocument()
    return this.createPacketStream(docId, this.nextStreamId++)
  }

  // createPacketStream creates a PacketStream from HTTP endpoints with proper length-prefix framing.
  private createPacketStream(
    docId: string,
    streamId: number,
  ): PacketStream {
    const readUrl = `/b/saucer/${docId}/stream/${streamId}/read`
    const writeUrl = `/b/saucer/${docId}/stream/${streamId}/write`

    const state = { closed: false }

    // Raw byte source from HTTP
    const rawSource: Pushable<Uint8Array> = pushable()

    // Start reading from HTTP endpoint
    ;(async () => {
      try {
        const resp = await fetch(readUrl)
        if (!resp.ok || !resp.body) {
          rawSource.end(new Error(`Read failed: ${resp.status}`))
          return
        }

        const reader = resp.body.getReader()
        while (!state.closed) {
          const { done, value } = await reader.read()
          if (done) break
          if (value?.byteLength) rawSource.push(value)
        }
      } catch (err) {
        if (!state.closed) rawSource.end(err as Error)
        return
      }
      rawSource.end()
    })()

    // Parse length-prefixed packets from raw bytes
    const source = pipe(
      rawSource,
      parseLengthPrefixTransform(),
      combineUint8ArrayListTransform(),
    )

    // Sink: prepend length prefix and send via HTTP
    const sink = async (packets: Source<Uint8Array>): Promise<void> => {
      try {
        const framed = pipe(
          packets,
          prependLengthPrefixTransform(),
          combineUint8ArrayListTransform(),
        )
        for await (const chunk of framed) {
          if (state.closed) break
          await this.postData(writeUrl, chunk)
        }
      } catch (err) {
        if (!state.closed) console.error('[saucer] Sink error:', err)
      } finally {
        state.closed = true
      }
    }

    return { source, sink }
  }

  // postData posts binary data to an endpoint.
  private async postData(url: string, data: Uint8Array): Promise<void> {
    // Cast to BodyInit - fetch() accepts Uint8Array at runtime but TypeScript's
    // DOM typings are stricter. This avoids an unnecessary ArrayBuffer.slice() copy.
    const resp = await fetch(url, {
      method: 'POST',
      body: data as unknown as BodyInit,
    })
    if (!resp.ok) {
      throw new Error(`POST ${url} failed: ${resp.status}`)
    }
  }

}
