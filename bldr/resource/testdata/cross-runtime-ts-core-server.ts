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
import { Resource } from '../../sdk/resource/resource.js'
import { getResourceCall } from '../../sdk/resource/server/context.js'

type FixtureState = {
  adoptAllowed: boolean
  releaseCount: number
  activeHandlers: number
  routeBeforeAdoptAck: boolean
  resolveAdopt: () => void
  adoptAllowedPromise: Promise<void>
  assertAbort: () => void
  assertAbortPromise: Promise<void>
}

function report(marker: string): void {
  console.log(marker)
}

function bytes(value: string): Uint8Array {
  return new TextEncoder().encode(value)
}

function decodeID(data: Uint8Array): number {
  if (data.length !== 4) throw new Error(`resource ID length = ${data.length}`)
  return new DataView(data.buffer, data.byteOffset, data.byteLength).getUint32(
    0,
  )
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

class AttachedChild extends Resource {
  async invokeUntilDetached(assertAbort: Promise<void>): Promise<void> {
    const responses = this.client
      .serverStreamingRequest('test.AttachedChild', 'Invoke', bytes('invoke'))
      [Symbol.asyncIterator]()
    const active = await responses.next()
    if (active.done || new TextDecoder().decode(active.value) !== 'active') {
      throw new Error('attached child did not enter invocation')
    }
    report('ATTACHED_CHILD_INVOKE_ACTIVE')
    this.release()
    report('ATTACHED_CHILD_DETACHED')
    try {
      const result = await Promise.race([
        responses.next(),
        assertAbort.then(() => {
          throw new Error('attached child invocation was not aborted by detach')
        }),
      ])
      throw new Error(
        `attached child invocation completed after detach: done=${result.done}`,
      )
    } catch (error) {
      if (error instanceof Error && error.message.includes('RPC_ABORT')) {
        report('ATTACHED_CHILD_INVOKE_ABORTED')
        return
      }
      throw error
    }
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
      UseAttached: async (source, sink, context: ServerContext) => {
        const attachedRootID = decodeID(await readOne(source))
        const resourceCall = getResourceCall(context)
        const init = Reflect.get(resourceCall, 'init') as {
          client: {
            clientID: number
            attachedResources: Map<number, unknown>
          }
        }
        if (!init.client.attachedResources.has(attachedRootID)) {
          throw new Error(
            `attached AddAck ID ${attachedRootID} missing from client ${init.client.clientID}`,
          )
        }
        report(`ATTACHED_GENERATION ${init.client.clientID} ${attachedRootID}`)
        const attachedRoot = resourceCall.getAttachedRef(attachedRootID)
        const childID = decodeID(
          await attachedRoot.client.request(
            'test.AttachedEngine',
            'Construct',
            bytes('construct'),
          ),
        )
        report(`ATTACHED_CHILD_ADDED ${childID}`)
        const child = attachedRoot.createResource(childID, AttachedChild)
        await child.invokeUntilDetached(state.assertAbortPromise)
        attachedRoot.release()
        report('ATTACHED_ROOT_DETACHED')
        await sink(single(bytes('attached-complete')))
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
  const fixtureMode = process.argv[3] ?? 'lifecycle'
  if (!requested) {
    throw new Error('usage: cross-runtime-ts-core-server <host:port>')
  }
  const [host, portText] = requested.split(':')
  const port = Number.parseInt(portText ?? '', 10)
  if (!host || !portText || Number.isNaN(port)) {
    throw new Error(`invalid address: ${requested}`)
  }
  let resolveAdopt!: () => void
  let resolveAssertAbort!: () => void
  const state: FixtureState = {
    adoptAllowed: false,
    releaseCount: 0,
    activeHandlers: 0,
    routeBeforeAdoptAck: false,
    resolveAdopt: () => resolveAdopt(),
    adoptAllowedPromise: new Promise<void>((resolve) => {
      resolveAdopt = resolve
    }),
    assertAbort: () => resolveAssertAbort(),
    assertAbortPromise: new Promise<void>((resolve) => {
      resolveAssertAbort = resolve
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
    const expectedReleaseCount = fixtureMode === 'attached' ? 0 : 1
    if (state.releaseCount !== expectedReleaseCount) {
      throw new Error(
        `release count = ${state.releaseCount}, want ${expectedReleaseCount}`,
      )
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
    rpcServer.handlePacketStream(
      fixtureMode === 'attached'
        ? tcpPacketStream(socket)
        : delayedResourceClientStream(socket, state),
    )
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
    if (line === 'ASSERT_ABORT') {
      state.assertAbort()
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
