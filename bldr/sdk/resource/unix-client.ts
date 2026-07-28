import net from 'node:net'

import { StreamConn, castToError } from 'starpc'

import { ResourceServiceClient } from '../../resource/resource_srpc.pb.js'
import { Client as ResourceClient } from './client.js'

// UnixResourceConnection owns one Unix socket and its outbound Yamux session.
export interface UnixResourceConnection extends Disposable {
  readonly client: ResourceClient
  readonly closed: Promise<Error | undefined>
  close(): void
}

class UnixResourceConnectionOwner implements UnixResourceConnection {
  public readonly client: ResourceClient
  public readonly closed: Promise<Error | undefined>

  private readonly socket: net.Socket
  private readonly connection: StreamConn
  private readonly controller = new AbortController()
  private readonly signal: AbortSignal
  private readonly ready: Promise<void>
  private resolveReady!: () => void
  private rejectReady!: (error: Error) => void
  private resolveClosed!: (error: Error | undefined) => void
  private connected = false
  private closing = false
  private finished = false
  private closeError: Error | undefined

  constructor(socketPath: string, signal: AbortSignal) {
    this.signal = signal
    this.socket = net.createConnection({ path: socketPath })
    this.connection = new StreamConn(undefined, {
      direction: 'outbound',
      yamuxParams: { enableKeepAlive: false },
    })
    this.client = new ResourceClient(
      new ResourceServiceClient(this.connection.buildClient()),
      this.controller.signal,
    )
    this.ready = new Promise<void>((resolve, reject) => {
      this.resolveReady = resolve
      this.rejectReady = reject
    })
    this.closed = new Promise<Error | undefined>((resolve) => {
      this.resolveClosed = resolve
    })

    this.socket.once('connect', this.handleConnect)
    this.socket.on('error', this.handleError)
    this.socket.once('close', this.handleClose)
    this.signal.addEventListener('abort', this.handleAbort, { once: true })
  }

  public async waitUntilReady(): Promise<void> {
    await this.ready
  }

  public close(): void {
    if (this.closing || this.finished) return
    this.closing = true
    this.controller.abort()
    this.client.dispose()
    this.connection.close()
    if (!this.connected) {
      this.rejectReady(abortError())
    }
    this.socket.destroy()
  }

  public [Symbol.dispose](): void {
    this.close()
  }

  private readonly handleConnect = () => {
    if (this.closing || this.finished) return
    this.connected = true
    this.resolveReady()
    this.runTransport()
  }

  private readonly handleError = (error: Error) => {
    this.terminate(error)
  }

  private readonly handleClose = () => {
    if (this.finished) return
    this.finished = true
    this.signal.removeEventListener('abort', this.handleAbort)
    this.socket.off('error', this.handleError)

    if (!this.closing && !this.closeError) {
      this.closeError = new Error('Resource endpoint connection closed')
    }
    if (!this.connected) {
      this.rejectReady(this.closeError ?? abortError())
    }
    this.controller.abort()
    this.client.dispose(
      this.closeError ? 'CONNECTION_FAILED' : 'CLIENT_DISPOSED',
    )
    this.connection.close(this.closeError)
    this.resolveClosed(this.closeError)
  }

  private readonly handleAbort = () => {
    this.close()
  }

  private runTransport(): void {
    Promise.race([
      Promise.resolve(
        this.connection.sink(socketSource(this.socket, this.controller.signal)),
      ),
      writeConnectionSource(
        this.socket,
        this.connection.source,
        this.controller.signal,
      ),
    ]).then(
      () => {
        if (!this.closing) {
          this.terminate(new Error('Resource endpoint connection closed'))
        }
      },
      (error) => {
        if (!this.closing) {
          this.terminate(castToError(error))
        }
      },
    )
  }

  private terminate(error: Error): void {
    if (this.finished) return
    this.closeError ??= error
    this.closing = true
    this.controller.abort()
    this.client.dispose('CONNECTION_FAILED')
    this.connection.close(this.closeError)
    if (!this.connected) {
      this.rejectReady(this.closeError)
    }
    this.socket.destroy()
  }
}

// connectUnixResourceClient connects the Resource SDK through the private
// per-launch Unix endpoint used by a native TuiView host.
export async function connectUnixResourceClient(
  endpoint: string,
  signal: AbortSignal,
): Promise<UnixResourceConnection> {
  if (signal.aborted) throw abortError()
  const socketPath = parseUnixEndpoint(endpoint)
  const connection = new UnixResourceConnectionOwner(socketPath, signal)
  await connection.waitUntilReady()
  return connection
}

function parseUnixEndpoint(endpoint: string): string {
  const prefix = 'unix://'
  if (!endpoint.startsWith(prefix)) {
    throw new Error('Resource endpoint must use unix://')
  }
  const socketPath = endpoint.slice(prefix.length)
  if (!socketPath.startsWith('/')) {
    throw new Error('Resource Unix socket path must be absolute')
  }
  return socketPath
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
