import { pushable } from 'it-pushable'
import type { PacketStream } from 'starpc'

// GoPushableSink is the sink handed to the caller through onResolve.
export type GoPushableSink = {
  push: (message: Uint8Array) => void
  end: () => void
}

// OpenStreamFunc is the callback contract used across the Worker boundary.
export type OpenStreamFunc = (
  onMessage: (message: Uint8Array) => void,
  onClose: (errMsg?: string) => void,
  onResolve: (sink: GoPushableSink) => void,
  onReject: (errMsg: string) => void,
) => void

// OpenStreamBridge adapts and closes both directions of a PacketStream.
export type OpenStreamBridge = {
  open: OpenStreamFunc
  close: (err?: Error) => void
}

// castToError converts an unknown thrown value to an Error.
export function castToError(err: unknown, fallback = 'unknown error'): Error {
  if (err instanceof Error) return err
  if (typeof err === 'string') return new Error(err)
  return new Error(fallback)
}

// packetStreamToOpenStreamCallbacks adapts a PacketStream to callback IO.
export function packetStreamToOpenStreamCallbacks(
  channel: PacketStream,
): OpenStreamBridge {
  const out = pushable<Uint8Array>({ objectMode: true })
  let closed = false
  let opened = false
  let closeError: Error | undefined
  let onClose: ((errMsg?: string) => void) | undefined

  const close = (err?: Error) => {
    if (closed) return
    closed = true
    closeError = err
    out.end(err)
    if (err) {
      channel.abort(err)
    } else {
      void channel.close().catch((closeErr) => {
        channel.abort(castToError(closeErr))
      })
    }
    onClose?.(err?.toString())
  }

  const open: OpenStreamFunc = (message, closedByPeer, resolve, reject) => {
    if (closed || opened) {
      reject(
        (closeError ?? new Error('stream already closed or opened')).toString(),
      )
      return
    }
    opened = true
    onClose = closedByPeer
    resolve({
      push: (packet) => {
        if (!closed) out.push(packet)
      },
      end: () => close(),
    })
    void channel.sink(out).catch((err) => close(castToError(err)))
    void (async () => {
      try {
        for await (const packet of channel.source) {
          if (closed) return
          message(packet)
        }
        close()
      } catch (err) {
        close(castToError(err))
      }
    })()
  }

  return { open, close }
}
