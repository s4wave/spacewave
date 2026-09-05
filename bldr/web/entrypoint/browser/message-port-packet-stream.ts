import { type PacketStream } from 'starpc'

// messagePortPacketStream speaks the Go MessagePort protocol: raw byte packets
// and null for write EOF. Finishing one direction preserves the other; close
// and abort release both directions, including pending reads and writes.
export function messagePortPacketStream(port: MessagePort): PacketStream {
  let closed = false
  let readClosed = false
  let writeClosed = false
  let endRead: ReadableStreamDefaultController<Uint8Array>
  let terminate: (error?: Error) => void
  const termination = new Promise<Error | undefined>((resolve) => {
    terminate = resolve
  })
  const incoming = new ReadableStream<Uint8Array>({
    start(controller) {
      endRead = controller
    },
  })

  // finish owns terminal notification and releases the port exactly once.
  function finish(error?: Error) {
    if (closed) return
    closed = true
    terminate(error)
    if (!readClosed) {
      readClosed = true
      if (error) endRead.error(error)
      else endRead.close()
    }
    if (!writeClosed) {
      writeClosed = true
      port.postMessage(null)
    }
    port.onmessage = null
    port.close()
  }

  port.onmessage = (event: MessageEvent<Uint8Array | null>) => {
    if (closed || readClosed) return
    if (event.data === null) {
      readClosed = true
      endRead.close()
      if (writeClosed) finish()
    } else if (event.data instanceof Uint8Array) {
      endRead.enqueue(event.data)
    } else {
      finish(new Error('Go MessagePort received a non-byte packet'))
    }
  }
  port.start()

  return {
    source: (async function* () {
      const reader = incoming.getReader()
      try {
        while (true) {
          const next = await reader.read()
          if (next.done) return
          yield next.value
        }
      } finally {
        reader.releaseLock()
      }
    })(),
    sink: async (source) => {
      const iterator =
        Symbol.asyncIterator in source
          ? source[Symbol.asyncIterator]()
          : source[Symbol.iterator]()
      const ended = termination.then((error) => ({ error }))
      try {
        while (true) {
          const next = await Promise.race([
            Promise.resolve(iterator.next()),
            ended,
          ])
          if ('error' in next) {
            if (next.error) throw next.error
            break
          }
          if (closed || next.done) break
          port.postMessage(next.value)
        }
        if (!writeClosed) {
          writeClosed = true
          port.postMessage(null)
          if (readClosed) finish()
        }
      } catch (error) {
        finish(error instanceof Error ? error : new Error(String(error)))
        throw error
      } finally {
        void Promise.resolve(iterator.return?.()).catch(() => {})
      }
    },
    close: async () => finish(),
    abort: finish,
  }
}
