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
  callCancelsFromClient: number
  callCancelsFromServer: number
  attachedBlockAborts: number
  attachedObjectReleases: number
  detachAttachedChild?: () => void
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

function tcpPacketStream(
  socket: net.Socket,
  state?: FixtureState,
): PacketStream {
  const source = (async function* (): AsyncGenerator<Uint8Array> {
    const packets = pushable<Uint8Array>({ objectMode: true })
    socket.on('data', (data: Buffer) => packets.push(new Uint8Array(data)))
    socket.on('end', () => packets.end())
    socket.on('error', (error) => packets.end(error))
    socket.on('close', () => packets.end())
    for await (const data of pipe(
      packets,
      parseLengthPrefixTransform(),
      combineUint8ArrayListTransform(),
    )) {
      if (state && Packet.fromBinary(data).body?.case === 'callCancel') {
        state.callCancelsFromClient++
      }
      yield data
    }
  })()
  return {
    async close() {
      socket.destroy()
    },
    abort(error) {
      socket.destroy(error)
    },
    source,
    sink: async (input: Source<Uint8Array>): Promise<void> => {
      const observed = (async function* (): AsyncGenerator<Uint8Array> {
        for await (const data of input) {
          if (state && Packet.fromBinary(data).body?.case === 'callCancel') {
            state.callCancelsFromServer++
          }
          yield data
        }
      })()
      for await (const chunk of pipe(
        observed,
        prependLengthPrefixTransform(),
      )) {
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
    close: () => stream.close(),
    abort: (error) => stream.abort(error),
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
  async invoke(): Promise<Uint8Array> {
    return await this.client.request(
      'test.AttachedChild',
      'Invoke',
      bytes('invoke'),
    )
  }

  async block(): Promise<void> {
    const responses = this.client
      .serverStreamingRequest('test.AttachedChild', 'Block', bytes('block'))
      [Symbol.asyncIterator]()
    const active = await responses.next()
    if (active.done || new TextDecoder().decode(active.value) !== 'active') {
      throw new Error('attached child did not enter blocking invocation')
    }
    try {
      await responses.next()
      throw new Error('attached child invocation completed without detach')
    } catch (error) {
      if (error instanceof Error && error.message.includes('RPC_ABORT')) return
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
      ConstructAttachedObject: async (source, sink, context: ServerContext) => {
        const attachedRootID = decodeID(await readOne(source))
        const resourceCall = getResourceCall(context)
        const attachedRoot = resourceCall.getAttachedRef(attachedRootID)
        report(`ATTACHED_ADD_ACK ${attachedRootID}`)
        const childID = decodeID(
          await attachedRoot.client.request(
            'test.AttachedEngine',
            'Construct',
            bytes('construct'),
          ),
        )
        report(`ATTACHED_CHILD_ADDED ${childID}`)
        const child = attachedRoot.createResource(childID, AttachedChild)
        state.detachAttachedChild = () => {
          state.detachAttachedChild = undefined
          child.release()
          report('ATTACHED_CHILD_DETACHED')
        }
        const object = resourceCall.constructChildResource(() => {
          const mux = createMux()
          mux.register(
            new StaticHandler('test.AttachedObject', {
              Use: async (objectSource, objectSink) => {
                if (
                  new TextDecoder().decode(await readOne(objectSource)) !==
                  'use'
                ) {
                  throw new Error('attached object use request mismatch')
                }
                await objectSink(single(await child.invoke()))
                report('ATTACHED_USE_COMPLETE')
              },
              Block: async (objectSource) => {
                if (
                  new TextDecoder().decode(await readOne(objectSource)) !==
                  'block'
                ) {
                  throw new Error('attached object block request mismatch')
                }
                try {
                  await child.block()
                  throw new Error(
                    'attached object block returned without detach',
                  )
                } catch (error) {
                  state.attachedBlockAborts++
                  report('ATTACHED_BLOCK_ABORTED')
                  throw error
                }
              },
            } satisfies Record<string, InvokeFn>),
          )
          return {
            mux,
            result: undefined,
            releaseFn: () => {
              state.attachedObjectReleases++
              child.release()
              attachedRoot.release()
            },
          }
        })
        await sink(single(encodeID(object.resourceId)))
        report(`ATTACHED_OBJECT_CONSTRUCTED ${object.resourceId}`)
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
  const state: FixtureState = {
    adoptAllowed: false,
    releaseCount: 0,
    activeHandlers: 0,
    routeBeforeAdoptAck: false,
    resolveAdopt: () => resolveAdopt(),
    adoptAllowedPromise: new Promise<void>((resolve) => {
      resolveAdopt = resolve
    }),
    callCancelsFromClient: 0,
    callCancelsFromServer: 0,
    attachedBlockAborts: 0,
    attachedObjectReleases: 0,
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
  let cleanRequested = false
  const verifyClean = () => {
    if (cleaned || state.activeHandlers !== 0) return
    const expectedReleaseCount = fixtureMode === 'attached' ? 0 : 1
    if (state.releaseCount !== expectedReleaseCount) {
      throw new Error(
        `release count = ${state.releaseCount}, want ${expectedReleaseCount}`,
      )
    }
    if (fixtureMode === 'attached' && state.attachedObjectReleases !== 1) {
      throw new Error(
        `attached object releases = ${state.attachedObjectReleases}, want 1`,
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
      if (cleanRequested && sockets.size === 0) queueMicrotask(verifyClean)
    })
    rpcServer.handlePacketStream(
      fixtureMode === 'attached'
        ? tcpPacketStream(socket, state)
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
    if (line === 'CHECK_LIVE') {
      if (
        state.callCancelsFromClient !== 0 ||
        state.callCancelsFromServer !== 0
      ) {
        throw new Error(
          `two-hop lifecycle canceled before detach: client-to-server=${state.callCancelsFromClient} server-to-client=${state.callCancelsFromServer}`,
        )
      }
      report('ATTACHED_LIVE_NO_CANCEL')
      return
    }
    if (line === 'CHECK_CLEAN') {
      verifyClean()
      if (!cleaned) {
        throw new Error(
          `server still has ${state.activeHandlers} active handlers after ResourceClient completion`,
        )
      }
      return
    }
    if (line === 'DETACH_ATTACHED_CHILD') {
      if (!state.detachAttachedChild) {
        throw new Error('no retained attached child is available to detach')
      }
      state.detachAttachedChild()
      return
    }
    if (line === 'CHECK_ABORT_ONCE') {
      if (state.callCancelsFromClient !== 0) {
        throw new Error(
          `retained child detach emitted ${state.callCancelsFromClient} unexpected client-to-server outer CallCancel packets`,
        )
      }
      if (state.callCancelsFromServer !== 0) {
        throw new Error(
          `retained child detach emitted ${state.callCancelsFromServer} unexpected server-to-client outer CallCancel packets`,
        )
      }
      if (state.attachedBlockAborts !== 1) {
        throw new Error(
          `attached block aborts = ${state.attachedBlockAborts}, want 1`,
        )
      }
      report('ATTACHED_ABORT_ONCE')
      return
    }
    if (line === 'INVALIDATE') {
      cleanRequested = true
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
