import net from 'node:net'

import { Client as SrpcClient, StreamConn, castToError } from 'starpc'
import type { PacketStream } from 'starpc'
import type { Source } from 'it-stream-types'

import { ResourceServiceClient } from '../../resource/resource_srpc.pb.js'
import { Client as ResourceClient } from './client.js'

// createUnixResourceClient connects the canonical Resource SDK to a native
// Spacewave Resource listener. The caller owns signal cancellation.
export function createUnixResourceClient(
  socketPath: string,
  signal: AbortSignal,
): ResourceClient {
  if (!socketPath.startsWith('/')) {
    throw new Error('Resource Unix socket path must be absolute')
  }
  const rpc = new SrpcClient(() => openPacketStream(socketPath, signal))
  return new ResourceClient(new ResourceServiceClient(rpc), signal)
}

async function openSocket(
  socketPath: string,
  signal: AbortSignal,
): Promise<net.Socket> {
  const socket = net.createConnection({ path: socketPath })
  if (signal.aborted) {
    socket.destroy()
    throw abortError()
  }
  return await new Promise<net.Socket>((resolve, reject) => {
    const cleanup = () => {
      signal.removeEventListener('abort', onAbort)
      socket.removeListener('connect', onConnect)
      socket.removeListener('error', onError)
    }
    const onAbort = () => {
      cleanup()
      socket.destroy()
      reject(abortError())
    }
    const onConnect = () => {
      cleanup()
      resolve(socket)
    }
    const onError = (error: Error) => {
      cleanup()
      reject(error)
    }
    signal.addEventListener('abort', onAbort, { once: true })
    socket.once('connect', onConnect)
    socket.once('error', onError)
  })
}

async function openPacketStream(
  socketPath: string,
  signal: AbortSignal,
): Promise<PacketStream> {
  const socket = await openSocket(socketPath, signal)
  if (signal.aborted) {
    socket.destroy()
    throw abortError()
  }
  const connection = new StreamConn()
  let closed = false
  const closeConnection = (error?: Error) => {
    if (closed) return
    closed = true
    signal.removeEventListener('abort', onAbort)
    connection.close(error)
    if (error) socket.destroy()
    else socket.end()
  }
  const onAbort = () => closeConnection(abortError())
  signal.addEventListener('abort', onAbort, { once: true })
  socket.once('close', () => {
    closeConnection(signal.aborted ? abortError() : undefined)
  })

  void (async () => {
    try {
      await Promise.race([
        Promise.resolve(connection.sink(socketSource(socket, signal))),
        writeConnectionSource(socket, connection.source, signal),
      ])
      closeConnection()
    } catch (error) {
      closeConnection(castToError(error))
    }
  })()

  try {
    const stream = await connection.openStream()
    return wrapPacketStream(stream, closeConnection)
  } catch (error) {
    closeConnection(castToError(error))
    throw error
  }
}

async function* socketSource(
  socket: net.Socket,
  signal: AbortSignal,
): AsyncGenerator<Uint8Array> {
  try {
    for await (const chunk of socket) {
      if (signal.aborted) return
      yield new Uint8Array(chunk.buffer, chunk.byteOffset, chunk.byteLength)
    }
  } catch (error) {
    if (!signal.aborted) throw error
  }
}

async function writeConnectionSource(
  socket: net.Socket,
  source: AsyncIterable<Uint8Array | { subarray(): Uint8Array }>,
  signal: AbortSignal,
): Promise<void> {
  for await (const chunk of source) {
    const data = chunk instanceof Uint8Array ? chunk : chunk.subarray()
    await writeSocket(socket, data, signal)
  }
}

function wrapPacketStream(
  stream: PacketStream,
  close: () => void,
): PacketStream {
  let sourceDone = false
  let sinkDone = false
  const finish = () => {
    if (sourceDone && sinkDone) close()
  }
  return {
    source: (async function* () {
      try {
        yield* stream.source
      } finally {
        sourceDone = true
        finish()
      }
    })(),
    sink: async (source: Source<Uint8Array>) => {
      try {
        await stream.sink(source)
      } finally {
        sinkDone = true
        finish()
      }
    },
  }
}

async function writeSocket(
  socket: net.Socket,
  data: Uint8Array,
  signal: AbortSignal,
): Promise<void> {
  if (signal.aborted) throw abortError()
  await new Promise<void>((resolve, reject) => {
    const onAbort = () => {
      cleanup()
      reject(abortError())
    }
    const cleanup = () => signal.removeEventListener('abort', onAbort)
    signal.addEventListener('abort', onAbort, { once: true })
    socket.write(data, (error) => {
      cleanup()
      if (error) reject(error)
      else resolve()
    })
  })
}

function abortError(): Error {
  const error = new Error('Resource endpoint connection aborted')
  error.name = 'AbortError'
  return error
}
