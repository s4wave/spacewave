import { describe, expect, it } from 'vitest'
import {
  Client,
  Server,
  createHandler,
  createMux,
  type PacketStream,
} from 'starpc'
import { EchoerClient, EchoerDefinition, EchoerServer } from 'starpc/echo'

import {
  openZeroNativeWebViewIpcPacketStream,
  type ZeroNativeWebViewIpcBridge,
  type ZeroNativeWebViewIpcStreamCallbacks,
  type ZeroNativeWebViewIpcStreamHandle,
} from './zero-native-webview-ipc.js'

class PairedNativeWebViewBridge implements ZeroNativeWebViewIpcBridge {
  private readonly waiting = new Map<number, PairedNativeStream>()

  public openStream(
    streamId: number,
    callbacks: ZeroNativeWebViewIpcStreamCallbacks,
  ): ZeroNativeWebViewIpcStreamHandle {
    const stream = new PairedNativeStream(streamId, callbacks)
    const peer = this.waiting.get(streamId)
    if (peer) {
      this.waiting.delete(streamId)
      stream.connect(peer)
      peer.connect(stream)
      return stream
    }
    this.waiting.set(streamId, stream)
    return stream
  }
}

class PairedNativeStream implements ZeroNativeWebViewIpcStreamHandle {
  private peer: PairedNativeStream | null = null
  private closed = false

  public constructor(
    private readonly streamId: number,
    private readonly callbacks: ZeroNativeWebViewIpcStreamCallbacks,
  ) {}

  public connect(peer: PairedNativeStream): void {
    this.peer = peer
  }

  public send(data: Uint8Array): void {
    if (this.closed) {
      throw new Error('native stream is closed')
    }
    const peer = this.peer
    if (!peer || peer.closed) {
      throw new Error('native stream peer is closed')
    }
    const packet = new Uint8Array(data)
    queueMicrotask(() => peer.callbacks.onPacket(peer.streamId, packet))
  }

  public close(): void {
    if (this.closed) {
      return
    }
    this.closed = true
    const peer = this.peer
    queueMicrotask(() => {
      this.callbacks.onClose(this.streamId, 0, '')
      peer?.callbacks.onClose(peer.streamId, 0, '')
    })
  }

  public cancel(): void {
    if (this.closed) {
      return
    }
    this.closed = true
    const peer = this.peer
    queueMicrotask(() => {
      this.callbacks.onClose(this.streamId, 8, 'stream cancelled')
      peer?.callbacks.onClose(peer.streamId, 8, 'stream cancelled')
    })
  }
}

describe('zero-native WebView IPC packet stream', () => {
  it('carries browser-side StarPC framing over the native bridge shape', async () => {
    const bridge = new PairedNativeWebViewBridge()
    const mux = createMux()
    mux.register(createHandler(EchoerDefinition, new EchoerServer()))
    const server = new Server(mux.lookupMethod)

    const serverStream = await openZeroNativeWebViewIpcPacketStream(bridge, 11)
    const clientStream = await openZeroNativeWebViewIpcPacketStream(bridge, 11)

    const serverTask = server
      .rpcStreamHandler(serverStream as PacketStream)
      .catch(() => {})
    const client = new Client(async () => clientStream as PacketStream)
    const echoer = new EchoerClient(client)

    const resp = await echoer.Echo({ body: 'webview-ipc' })

    expect(resp.body).toBe('webview-ipc')

    clientStream.close()
    serverStream.close()
    await serverTask
  })
})
