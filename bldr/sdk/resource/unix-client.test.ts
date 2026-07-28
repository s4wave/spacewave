import net from 'node:net'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

import { describe, expect, it } from 'vitest'
import {
  Server,
  StreamConn,
  castToError,
  createHandler,
  createMux,
} from 'starpc'

import {
  RootResourceServiceDefinition,
  type RootResourceService,
} from '../../../sdk/root/root_srpc.pb.js'
import { Root } from '../../../sdk/root/root.js'
import {
  SessionResourceServiceDefinition,
  type SessionResourceService,
} from '../../../sdk/session/session_srpc.pb.js'
import { ResourceServer } from './server/server.js'
import { constructChildResource } from './server/construct.js'
import { connectUnixResourceClient } from './unix-client.js'

describe('connectUnixResourceClient', () => {
  it('mounts a Session and streams its Space list over a Unix socket', async () => {
    const dir = await mkdtemp(join(tmpdir(), 'resource-unix-test-'))
    const socketPath = join(dir, 'resource.sock')
    const sessionMux = createMux()
    sessionMux.register(
      createHandler(SessionResourceServiceDefinition, {
        async *WatchResourcesList() {
          yield {
            spacesList: [{ spaceMeta: { name: 'Terminal' } }],
          }
        },
      } satisfies Partial<SessionResourceService>),
    )
    const rootMux = createMux()
    rootMux.register(
      createHandler(RootResourceServiceDefinition, {
        MountSessionByIdx(request) {
          if (request.sessionIdx !== 4) {
            throw new Error(`unexpected Session index ${request.sessionIdx}`)
          }
          const { resourceId } = constructChildResource(() => ({
            mux: sessionMux,
            result: undefined,
          }))
          return Promise.resolve({ resourceId })
        },
      } satisfies Partial<RootResourceService>),
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
      using root = new Root(await connection.client.accessRootResource())
      using session = (await root.mountSessionByIdx(
        { sessionIdx: 4 },
        controller.signal,
      ))!.session

      const stream = session.watchResourcesList({}, controller.signal)
      const snapshot = await stream[Symbol.asyncIterator]().next()

      expect(snapshot.done).toBe(false)
      expect(snapshot.value?.spacesList?.[0]?.spaceMeta?.name).toBe('Terminal')
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
