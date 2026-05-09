import type { Sink, Source, Duplex } from 'it-stream-types'
import { pushable } from 'it-pushable'
import type { Pushable } from 'it-pushable'

export interface ZeroNativeWebViewIpcStreamCallbacks {
  onPacket: (streamId: number, data: Uint8Array) => void
  onClose: (streamId: number, code: number, message: string) => void
}

export interface ZeroNativeWebViewIpcStreamHandle {
  send(data: Uint8Array): void | Promise<void>
  close(): void | Promise<void>
  cancel?(): void | Promise<void>
}

export interface ZeroNativeWebViewIpcBridge {
  openStream(
    streamId: number,
    callbacks: ZeroNativeWebViewIpcStreamCallbacks,
  ):
    | ZeroNativeWebViewIpcStreamHandle
    | Promise<ZeroNativeWebViewIpcStreamHandle>
}

export interface ZeroNativeWebViewIpcPacketStream extends Duplex<
  AsyncGenerator<Uint8Array>,
  Source<Uint8Array>,
  Promise<void>
> {
  readonly streamId: number
  close(error?: Error): void
  cancel(error?: Error): void
}

export async function openZeroNativeWebViewIpcPacketStream(
  bridge: ZeroNativeWebViewIpcBridge,
  streamId: number,
): Promise<ZeroNativeWebViewIpcPacketStream> {
  return ZeroNativeWebViewIpcPacketStreamImpl.open(bridge, streamId)
}

// This adapter preserves the zero-native backend invariant: StarPC packets cross
// the renderer/backend boundary through the native WebView IPC bridge, not a
// renderer-local MessagePort, shared-worker, or direct callback transport.
class ZeroNativeWebViewIpcPacketStreamImpl implements ZeroNativeWebViewIpcPacketStream {
  public readonly source: AsyncGenerator<Uint8Array>
  public readonly sink: Sink<Source<Uint8Array>, Promise<void>>

  private readonly _source: Pushable<Uint8Array>
  private closed = false

  private constructor(
    public readonly streamId: number,
    private readonly handle: ZeroNativeWebViewIpcStreamHandle,
    source: Pushable<Uint8Array>,
  ) {
    this._source = source
    this.source = source
    this.sink = this.createSink()
  }

  public static async open(
    bridge: ZeroNativeWebViewIpcBridge,
    streamId: number,
  ): Promise<ZeroNativeWebViewIpcPacketStreamImpl> {
    const source = pushable<Uint8Array>({ objectMode: true })
    let stream: ZeroNativeWebViewIpcPacketStreamImpl | null = null
    const handle = await bridge.openStream(streamId, {
      onPacket(packetStreamId, data) {
        const active = stream
        if (packetStreamId !== streamId || active === null || active.closed) {
          return
        }
        active._source.push(data)
      },
      onClose(packetStreamId, code, message) {
        const active = stream
        if (packetStreamId !== streamId || active === null || active.closed) {
          return
        }
        active.closed = true
        if (code === 0) {
          active._source.end()
          return
        }
        active._source.end(
          new Error(message || `native stream closed: ${code}`),
        )
      },
    })
    stream = new ZeroNativeWebViewIpcPacketStreamImpl(streamId, handle, source)
    return stream
  }

  public close(error?: Error): void {
    if (this.closed) {
      return
    }
    this.closed = true
    this._source.end(error)
    void this.handle.close()
  }

  public cancel(error?: Error): void {
    if (this.closed) {
      return
    }
    this.closed = true
    this._source.end(error)
    if (this.handle.cancel) {
      void this.handle.cancel()
      return
    }
    void this.handle.close()
  }

  private createSink(): Sink<Source<Uint8Array>, Promise<void>> {
    return async (source: Source<Uint8Array>) => {
      try {
        for await (const msg of source) {
          if (this.closed) {
            break
          }
          await this.handle.send(msg)
        }
      } catch (err) {
        this.cancel(err instanceof Error ? err : new Error(String(err)))
        throw err
      }
    }
  }
}
