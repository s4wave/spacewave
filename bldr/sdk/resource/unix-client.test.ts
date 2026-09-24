import net from 'node:net'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

import { describe, expect, it } from 'vitest'
import {
  Server,
  type ServerContext,
  StreamConn,
  castToError,
  createHandler,
  createMux,
} from 'starpc'
import { EchoerClient, EchoerDefinition } from 'starpc/echo'

import { ResourceServer } from './server/server.js'
import { getResourceCall } from './server/context.js'
import { connectUnixResourceClient } from './unix-client.js'

describe('connectUnixResourceClient', () => {
  it('mounts a child resource and streams from it over a Unix socket', async () => {
    const dir = await mkdtemp(join(tmpdir(), 'resource-unix-test-'))
    const socketPath = join(dir, 'resource.sock')
    const childMux = createMux()
    childMux.register(
      createHandler(EchoerDefinition, {
        async *EchoServerStream(request) {
          yield { body: `child ${request.body}` }
        },
      }),
    )
    const rootMux = createMux()
    rootMux.register(
      createHandler(EchoerDefinition, {
        Echo(_request, _abortSignal: AbortSignal, context: ServerContext) {
          const { resourceId } = getResourceCall(
            context,
          ).constructChildResource(() => ({
            mux: childMux,
            result: undefined,
          }))
          return Promise.resolve({ body: String(resourceId) })
        },
      }),
    )
    const resources = new ResourceServer(rootMux)
    const rpcMux = createMux()
    resources.register(rpcMux)
    const rpcServer = new Server(rpcMux.lookupMethod)
    const sockets = new Set<net.Socket>()
    let resolveServerClosed!: () => void
    const serverClosed = new Promise<void>((resolve) => {
      resolveServerClosed = resolve
    })
    const listener = net.createServer((socket) => {
      sockets.add(socket)
      socket.once('close', () => {
        sockets.delete(socket)
        resolveServerClosed()
      })
      serveSocket(socket, rpcServer)
    })
    await new Promise<void>((resolve, reject) => {
      listener.once('error', reject)
      listener.listen(socketPath, resolve)
    })

    const controller = new AbortController()
    try {
      const connection = await connectUnixResourceClient(
        `unix://${socketPath}`,
        controller.signal,
      )
      using root = await connection.client.accessRootResource()
      const mounted = await new EchoerClient(root.client).Echo(
        {},
        controller.signal,
      )
      using child = root.createRef(Number(mounted.body))

      const stream = new EchoerClient(child.client).EchoServerStream(
        { body: 'Terminal' },
        controller.signal,
      )
      const snapshot = await stream[Symbol.asyncIterator]().next()

      expect(snapshot.done).toBe(false)
      expect(snapshot.value?.body).toBe('child Terminal')
      expect(sockets.size).toBe(1)

      connection.close()
      expect(await connection.closed).toBeUndefined()
      await serverClosed
      expect(sockets.size).toBe(0)
    } finally {
      controller.abort()
      for (const socket of sockets) socket.destroy()
      await new Promise<void>((resolve) => listener.close(() => resolve()))
      await rm(dir, { recursive: true, force: true })
    }
  })
})

function serveSocket(socket: net.Socket, server: Server): void {
  const connection = new StreamConn(server, { direction: 'inbound' })
  let closed = false
  const close = (error?: Error) => {
    if (closed) return
    closed = true
    connection.close(error)
    socket.destroy()
  }
  const source = (async function* () {
    for await (const chunk of socket) {
      yield new Uint8Array(chunk.buffer, chunk.byteOffset, chunk.byteLength)
    }
  })()
  const write = async () => {
    for await (const chunk of connection.source) {
      const data = chunk.subarray()
      await new Promise<void>((resolve, reject) => {
        socket.write(data, (error) => {
          if (error) reject(error)
          else resolve()
        })
      })
    }
  }
  void (async () => {
    try {
      await Promise.race([Promise.resolve(connection.sink(source)), write()])
      close()
    } catch (error) {
      close(castToError(error))
    }
  })()
}
