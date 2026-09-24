import { createServer as createHTTPServer } from 'node:http'
import type { Server as HTTPServer, IncomingMessage } from 'node:http'
import type { Duplex } from 'node:stream'
import { WebSocketServer, type WebSocket } from 'ws'
import {
  createHandler,
  createMux,
  Server as RpcServer,
  StreamConn,
  combineUint8ArrayListTransform,
} from 'starpc'
import type { Mux } from 'starpc'
import websocketDuplex from '@aptre/it-ws/duplex'
import { pipe } from 'it-pipe'

import type { Principal, Schema } from '../../sdk/sync/schema.js'
import { SyncError } from '../../sdk/sync/errors.js'
import { toFailure, wireVersion } from '../../sdk/sync/wire.js'
import {
  GatewayDefinition,
  GatewayServiceName,
} from '../../sdk/sync/sync_srpc.pb.js'
import { ResourceServiceDefinition } from '@aptre/bldr-sdk/resource/resource_srpc.pb.js'
import type { Application } from './application.js'
import { applicationResources } from './rpc.js'
import type {
  Attachment,
  AttachmentOptions,
  Authenticate,
  Listener,
  ListenerOptions,
} from './config.js'
export type {
  Attachment,
  AttachmentOptions,
  Authenticate,
  Listener,
  ListenerOptions,
} from './config.js'

// attachListener owns upgrade routing and accepted sockets, preserving the supplied HTTP server.
export function attachListener<S extends Schema, P extends Principal>(
  http: HTTPServer,
  application: Application<S, P>,
  authenticate: Authenticate<P>,
  options: AttachmentOptions = {},
): Attachment {
  const path = options.path ?? '/sync'
  const websocket = new WebSocketServer({
    noServer: true,
    maxPayload: options.maxRequestBytes ?? 1024 * 1024,
  })
  const connections = new Set<Promise<void>>()
  let closing: Promise<void> | undefined
  const upgrade = (request: IncomingMessage, socket: Duplex, head: Buffer) => {
    const requested = new URL(request.url ?? '/', 'http://sync.local')
    if (requested.pathname !== path) return
    const origin = request.headers.origin
    const allowed =
      !origin ||
      (options.allowedOrigins
        ? options.allowedOrigins.includes(origin)
        : origin === `http://${request.headers.host}` ||
          origin === `https://${request.headers.host}`)
    if (closing || !allowed || requested.search) {
      socket.end('HTTP/1.1 403 Forbidden\r\nConnection: close\r\n\r\n')
      return
    }
    websocket.handleUpgrade(request, socket, head, (accepted) => {
      const lifetime = serveConnection(
        accepted,
        request,
        application,
        authenticate,
      )
      connections.add(lifetime)
      void lifetime
        .finally(() => connections.delete(lifetime))
        .catch(() => accepted.terminate())
    })
  }
  http.on('upgrade', upgrade)
  const close = (): Promise<void> => {
    closing ??= (async () => {
      http.off('upgrade', upgrade)
      for (const socket of websocket.clients)
        socket.close(1001, 'Server closed')
      const deadline = setTimeout(() => {
        for (const socket of websocket.clients) socket.terminate()
      }, 1000)
      try {
        await Promise.allSettled(connections)
        await new Promise<void>((resolve, reject) =>
          websocket.close((error) => (error ? reject(error) : resolve())),
        )
      } finally {
        clearTimeout(deadline)
      }
    })()
    return closing
  }
  return { close, [Symbol.asyncDispose]: close }
}

// listen owns the quickstart HTTP listener and releases it after its attachment.
export async function listen<S extends Schema, P extends Principal>(
  application: Application<S, P>,
  authenticate: Authenticate<P>,
  options: ListenerOptions = {},
): Promise<Listener> {
  const http = createHTTPServer((_request, response) => {
    response.writeHead(404)
    response.end()
  })
  const attachment = attachListener(http, application, authenticate, options)
  try {
    await new Promise<void>((resolve, reject) => {
      const error = (error: Error) => reject(error)
      http.once('error', error)
      http.listen(options.port ?? 8787, options.host ?? '127.0.0.1', () => {
        http.off('error', error)
        resolve()
      })
    })
  } catch (error) {
    await attachment.close()
    throw error
  }
  const address = http.address()
  if (!address || typeof address === 'string')
    throw new Error('Listener has no TCP address')
  const host = address.address.includes(':')
    ? `[${address.address}]`
    : address.address
  let closing: Promise<void> | undefined
  const close = (): Promise<void> =>
    (closing ??= (async () => {
      await attachment.close()
      await new Promise<void>((resolve, reject) =>
        http.close((error) => (error ? reject(error) : resolve())),
      )
    })())
  return {
    url: `ws://${host}:${address.port}${options.path ?? '/sync'}`,
    close,
    [Symbol.asyncDispose]: close,
  }
}

async function serveConnection<S extends Schema, P extends Principal>(
  socket: WebSocket,
  request: IncomingMessage,
  application: Application<S, P>,
  authenticate: Authenticate<P>,
): Promise<void> {
  const controller = new AbortController()
  const closed = Promise.withResolvers<void>()
  let resourceMux: Mux | undefined
  let attempted = false
  let expiry: ReturnType<typeof setTimeout> | undefined
  let principal: P | undefined
  const active = new Set<Promise<void>>()
  const retire = () => {
    controller.abort()
    socket.close(4001, 'Authentication ended')
  }
  const gateway = createMux()
  gateway.register(
    createHandler(GatewayDefinition, {
      Authenticate: async (request, signal) => {
        try {
          if (attempted)
            throw new SyncError(
              'AUTHENTICATION',
              'This connection already attempted authentication',
            )
          attempted = true
          if (
            request.wireVersion !== wireVersion ||
            request.applicationId !== application.schema.id ||
            request.applicationVersion !== application.schema.version
          )
            throw new SyncError(
              'SCHEMA_MISMATCH',
              'Client and server contracts are incompatible',
            )
          const identity = await authenticate(
            request.token ?? '',
            AbortSignal.any([controller.signal, signal]),
          )
          application.checkPrincipal(identity)
          principal = { ...identity }
          resourceMux = applicationResources(application, principal)
          principal.signal?.addEventListener('abort', retire, { once: true })
          if (principal.signal?.aborted) retire()
          const expire = () => {
            const remaining = (principal?.expiresAt ?? Infinity) - Date.now()
            if (remaining <= 0) retire()
            else if (Number.isFinite(remaining))
              expiry = setTimeout(expire, Math.min(remaining, 0x7fffffff))
          }
          expire()
          return {}
        } catch (error) {
          return {
            error: toFailure(
              error instanceof SyncError
                ? error
                : new SyncError('AUTHENTICATION', 'Authentication failed'),
            ),
          }
        }
      },
    }),
  )
  const rpc = new RpcServer(async (service, method) => {
    const allowed =
      service === GatewayServiceName
        ? await gateway.lookupMethod(service, method)
        : resourceMux &&
            service === ResourceServiceDefinition.typeName &&
            method !== 'ResourceAttach'
          ? await resourceMux.lookupMethod(service, method)
          : null
    if (!allowed) return null
    return (source, sink, context) => {
      const invocation = allowed(source, sink, {
        ...context,
        signal: AbortSignal.any([context.signal, controller.signal]),
      })
      active.add(invocation)
      void invocation.finally(() => active.delete(invocation)).catch(() => {})
      return invocation
    }
  })
  const connection = new StreamConn(rpc, { direction: 'inbound' })
  const duplex = websocketDuplex(socket, {
    remoteAddress: request.socket.remoteAddress,
    remotePort: request.socket.remotePort,
  })
  const transport = pipe(
    duplex,
    connection,
    combineUint8ArrayListTransform(),
    duplex,
  )
    .catch((error: Error) => connection.close(error))
    .finally(() => socket.close())
  const onClose = () => {
    controller.abort()
    connection.close(new Error('WebSocket connection ended'))
    closed.resolve()
  }
  socket.once('close', onClose)
  socket.once('error', onClose)
  application.signal.addEventListener('abort', retire, { once: true })
  const handshakeDeadline = setTimeout(() => {
    if (!resourceMux) retire()
  }, 10_000)
  try {
    await closed.promise
    await Promise.allSettled(active)
  } finally {
    clearTimeout(handshakeDeadline)
    if (expiry) clearTimeout(expiry)
    principal?.signal?.removeEventListener('abort', retire)
    application.signal.removeEventListener('abort', retire)
    socket.off('close', onClose)
    socket.off('error', onClose)
    controller.abort()
    await connection.muxer.close()
    await transport
  }
}
