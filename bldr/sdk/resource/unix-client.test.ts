import net from 'node:net'
import { mkdtemp, rm } from 'node:fs/promises'
import { join } from 'node:path'

import { describe, expect, it } from 'vitest'
import { Server, StreamConn, castToError, createMux } from 'starpc'

import { ResourceServer } from './server/server.js'
import { createUnixResourceClient } from './unix-client.js'

describe('createUnixResourceClient', () => {
  it('carries Resource RPC streams through a Yamux Unix connection', async () => {
    const dir = await mkdtemp('/tmp/resource-unix-test-')
    const socketPath = join(dir, 'resource.sock')
    const rootMux = createMux()
    const resources = new ResourceServer(rootMux)
    const rpcMux = createMux()
    resources.register(rpcMux)
    const rpcServer = new Server(rpcMux.lookupMethod)
    const sockets = new Set<net.Socket>()
    const listener = net.createServer((socket) => {
      sockets.add(socket)
      socket.once('close', () => sockets.delete(socket))
      serveSocket(socket, rpcServer)
    })
    await new Promise<void>((resolve, reject) => {
      listener.once('error', reject)
      listener.listen(socketPath, resolve)
    })

    const controller = new AbortController()
    try {
      const client = createUnixResourceClient(socketPath, controller.signal)
      const root = await client.accessRootResource()
      expect(root.resourceId).toBe(1)
      expect(sockets.size).toBe(1)
      const socketClosed = Promise.all(
        [...sockets].map(
          (socket) =>
            new Promise<void>((resolve) => socket.once('close', resolve)),
        ),
      )
      root.release()
      controller.abort()
      await socketClosed
      expect(sockets.size).toBe(0)
      client.dispose()
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
