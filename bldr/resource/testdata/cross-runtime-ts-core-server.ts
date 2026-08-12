import net from 'node:net'
import readline from 'node:readline'

import { pipe } from 'it-pipe'
import { pushable } from 'it-pushable'
import type { Source } from 'it-stream-types'
import {
  Packet,
  Server,
  StaticHandler,
  combineUint8ArrayListTransform,
  createMux,
  parseLengthPrefixTransform,
  prependLengthPrefixTransform,
} from 'starpc'
import type { InvokeFn, PacketStream, ServerContext } from 'starpc'

import { ResourceClientResponse } from '../resource.pb.js'
import { ResourceServer } from '../../sdk/resource/server/server.js'
import { getResourceCall } from '../../sdk/resource/server/context.js'

type FixtureState = {
  adoptAllowed: boolean
  releaseCount: number
  activeHandlers: number
  routeBeforeAdoptAck: boolean
  resolveAdopt: () => void
  adoptAllowedPromise: Promise<void>
}

function report(marker: string): void {
  console.log(marker)
}

function bytes(value: string): Uint8Array {
  return new TextEncoder().encode(value)
}

function encodeID(id: number): Uint8Array {
  const data = new Uint8Array(4)
  new DataView(data.buffer).setUint32(0, id)
  return data
}

async function readOne(source: Source<Uint8Array>): Promise<Uint8Array> {
  const result = await source[Symbol.asyncIterator]().next()
  if (result.done) throw new Error('request stream closed before data')
  return result.value
}

async function* single(data: Uint8Array): AsyncGenerator<Uint8Array> {
  yield data
}

function waitForAbort(signal: AbortSignal): Promise<void> {
  if (signal.aborted) return Promise.resolve()
  return new Promise((resolve) => {
    signal.addEventListener('abort', resolve, { once: true })
  })
}

function tcpPacketStream(socket: net.Socket): PacketStream {
  const source = (async function* (): AsyncGenerator<Uint8Array> {
    const packets = pushable<Uint8Array>({ objectMode: true })
    socket.on('data', (data: Buffer) => packets.push(new Uint8Array(data)))
    socket.on('end', () => packets.end())
    socket.on('error', (error) => packets.end(error))
    socket.on('close', () => packets.end())
    yield* pipe(
      packets,
      parseLengthPrefixTransform(),
      combineUint8ArrayListTransform(),
    )
  })()
  return {
    source,
    sink: async (input: Source<Uint8Array>): Promise<void> => {
      for await (const chunk of pipe(input, prependLengthPrefixTransform())) {
        const data =
          chunk instanceof Uint8Array
            ? chunk
            : (chunk as { subarray(): Uint8Array }).subarray()
        await new Promise<void>((resolve, reject) => {
          socket.write(data, (error) => {
            if (error) reject(error)
            else resolve()
          })
        })
      }
      socket.end()
    },
  }
}

function delayedResourceClientStream(
  socket: net.Socket,
  state: FixtureState,
): PacketStream {
  const stream = tcpPacketStream(socket)
  let resourceClient = false
  return {
    source: stream.source,
    sink: async (input: Source<Uint8Array>): Promise<void> => {
      const outgoing = (async function* (): AsyncGenerator<Uint8Array> {
        for await (const data of input) {
          const packet = Packet.fromBinary(data)
          if (packet.body?.case === 'callData') {
            const response = ResourceClientResponse.fromBinary(
              packet.body.value.data,
            )
            if (response.body?.case === 'init') resourceClient = true
            if (
              resourceClient &&
              response.body?.case === 'controlAck' &&
              !state.adoptAllowed
            ) {
              report('ADOPT_ACK_HELD')
              await state.adoptAllowedPromise
            }
          }
          yield data
        }
      })()
      await stream.sink(outgoing)
    },
  }
}

function rootMux(state: FixtureState) {
  const mux = createMux()
  mux.register(
    new StaticHandler('test.Root', {
      Spawn: async (source, sink, context: ServerContext) => {
        if (!state.adoptAllowed) state.routeBeforeAdoptAck = true
        if (new TextDecoder().decode(await readOne(source)) !== 'spawn') {
          throw new Error('Spawn request mismatch')
        }
        const child = getResourceCall(context).constructChildResource(
          (signal) => ({
            mux: childMux(state, signal),
            result: undefined,
            releaseFn: () => {
              state.releaseCount++
              report('RELEASE_COMPLETE')
            },
          }),
        )
        await sink(single(encodeID(child.resourceId)))
      },
      Echo: async (source, sink) => {
        await sink(single(await readOne(source)))
      },
    } satisfies Record<string, InvokeFn>),
  )
  return mux
}

function childMux(state: FixtureState, resourceSignal: AbortSignal) {
  const mux = createMux()
  mux.register(
    new StaticHandler('test.Child', {
      Echo: async (source, sink) => {
        await sink(single(await readOne(source)))
      },
      Stream: async (source, sink) => {
        if (new TextDecoder().decode(await readOne(source)) !== 'stream') {
          throw new Error('Stream request mismatch')
        }
        await sink(single(bytes('later')))
      },
      Block: async (source, sink, context: ServerContext) => {
        if (new TextDecoder().decode(await readOne(source)) !== 'block') {
          throw new Error('Block request mismatch')
        }
        state.activeHandlers++
        try {
          await sink(
            (async function* (): AsyncGenerator<Uint8Array> {
              yield bytes('active')
              await Promise.race([
                waitForAbort(context.signal),
                waitForAbort(resourceSignal),
              ])
            })(),
          )
        } finally {
          state.activeHandlers--
          report('HANDLER_FINALLY')
        }
      },
    } satisfies Record<string, InvokeFn>),
  )
  return mux
}

async function main(): Promise<void> {
  const requested = process.argv[2]
  if (!requested) {
    throw new Error('usage: cross-runtime-ts-core-server <host:port>')
  }
  const [host, portText] = requested.split(':')
  const port = Number.parseInt(portText ?? '', 10)
  if (!host || !portText || Number.isNaN(port)) {
    throw new Error(`invalid address: ${requested}`)
  }
  let resolveAdopt!: () => void
  const state: FixtureState = {
    adoptAllowed: false,
    releaseCount: 0,
    activeHandlers: 0,
    routeBeforeAdoptAck: false,
    resolveAdopt: () => resolveAdopt(),
    adoptAllowedPromise: new Promise<void>((resolve) => {
      resolveAdopt = resolve
    }),
  }
  const resources = new ResourceServer(rootMux(state))
  const outerMux = createMux()
  resources.register(outerMux)
  const rpcServer = new Server(outerMux.lookupMethod)
  const sockets = new Set<net.Socket>()
  let resolveClean!: () => void
  const clean = new Promise<void>((resolve) => {
    resolveClean = resolve
  })
  let cleaned = false
  const verifyClean = () => {
    if (cleaned) return
    const clients = Reflect.get(resources, 'clients') as Map<
      number,
      {
        released: boolean
        resources: Map<number, unknown>
        attachedResources: Map<number, unknown>
      }
    >
    if (
      state.activeHandlers !== 0 ||
      [...clients.values()].some(
        (client) =>
          !client.released ||
          client.resources.size !== 0 ||
          client.attachedResources.size !== 0,
      )
    ) {
      return
    }
    if (state.releaseCount !== 1) {
      throw new Error(`release count = ${state.releaseCount}, want 1`)
    }
    if (state.routeBeforeAdoptAck) {
      throw new Error('ResourceRpc opened before delayed Adopt acknowledgement')
    }
    cleaned = true
    report('TS_SERVER_OWNER_ZERO')
    resolveClean()
  }
  const listener = net.createServer((socket) => {
    sockets.add(socket)
    socket.once('close', () => {
      sockets.delete(socket)
      queueMicrotask(verifyClean)
    })
    rpcServer.handlePacketStream(delayedResourceClientStream(socket, state))
  })
  await new Promise<void>((resolve, reject) => {
    listener.once('error', reject)
    listener.listen(port, host, resolve)
  })
  const address = listener.address()
  if (typeof address === 'string' || address === null) {
    throw new Error('TCP listener did not provide an address')
  }
  report(`READY ${address.address}:${address.port}`)

  const commands = readline.createInterface({ input: process.stdin })
  commands.on('line', (line) => {
    if (line === 'ALLOW_ADOPT') {
      state.adoptAllowed = true
      state.resolveAdopt()
      return
    }
    if (line === 'INVALIDATE') {
      const clients = Reflect.get(resources, 'clients') as Map<
        number,
        { releaseAll(): void }
      >
      for (const client of clients.values()) client.releaseAll()
      for (const socket of sockets) socket.destroy()
      queueMicrotask(verifyClean)
      return
    }
    throw new Error(`unknown fixture command: ${line}`)
  })
  await clean
  commands.close()
  for (const socket of sockets) socket.destroy()
  await new Promise<void>((resolve) => listener.close(() => resolve()))
}

main().catch((error: unknown) => {
  console.error(error)
  process.exitCode = 1
})
