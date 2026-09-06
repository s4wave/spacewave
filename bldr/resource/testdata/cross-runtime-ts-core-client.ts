import net from 'node:net'

import { pipe } from 'it-pipe'
import { pushable } from 'it-pushable'
import type { Source } from 'it-stream-types'
import {
  Client as SRPCClient,
  combineUint8ArrayListTransform,
  parseLengthPrefixTransform,
  prependLengthPrefixTransform,
} from 'starpc'
import type { PacketStream } from 'starpc'

import { Client as ResourceClient } from '../../sdk/resource/client.js'
import { ResourceServiceClient } from '../resource_srpc.pb.js'

// report publishes lifecycle barriers to the parent test process.
function report(marker: string): void {
  console.log(marker)
}

// bytes encodes domain fixture requests.
function bytes(value: string): Uint8Array {
  return new TextEncoder().encode(value)
}

// text decodes domain fixture responses.
function text(value: Uint8Array): string {
  return new TextDecoder().decode(value)
}

// decodeID requires the fixed-width child resource identifier.
function decodeID(data: Uint8Array): number {
  if (data.length !== 4) throw new Error(`child ID length = ${data.length}`)
  return new DataView(data.buffer, data.byteOffset, data.byteLength).getUint32(
    0,
  )
}

// tcpSocketToPacketStream owns one RPC connection, including cancellation.
function tcpSocketToPacketStream(socket: net.Socket): PacketStream {
  const source = async function* (): AsyncGenerator<Uint8Array> {
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
  }
  return {
    async close() {
      socket.destroy()
    },
    abort(error) {
      socket.destroy(error)
    },
    source: source(),
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

// nextStreamValue retains an active iterator after checking its first response.
async function nextStreamValue(
  stream: AsyncIterable<Uint8Array>,
  want: string,
): Promise<AsyncIterator<Uint8Array>> {
  const iterator = stream[Symbol.asyncIterator]()
  const first = await iterator.next()
  if (first.done || text(first.value) !== want) {
    throw new Error(
      `stream value = ${first.done ? 'done' : text(first.value)}, want ${want}`,
    )
  }
  return iterator
}

// expectAbort rejects normal completion of an explicitly canceled RPC.
async function expectAbort(iterator: AsyncIterator<Uint8Array>): Promise<void> {
  try {
    await iterator.next()
  } catch (error) {
    if (error instanceof Error && error.message.includes('RPC_ABORT')) return
    throw error
  }
  throw new Error('active stream completed instead of aborting')
}

// main checks resource adoption, cancellation, release, and server invalidation.
async function main(): Promise<void> {
  const address = process.argv[2]
  if (!address)
    throw new Error('usage: cross-runtime-ts-core-client <host:port>')
  const [host, portText] = address.split(':')
  const port = Number.parseInt(portText ?? '', 10)
  if (!host || !portText || Number.isNaN(port)) {
    throw new Error(`invalid address: ${address}`)
  }
  const controller = new AbortController()
  const srpc = new SRPCClient(async () => {
    const socket = await new Promise<net.Socket>((resolve, reject) => {
      const next = net.connect(port, host, () => resolve(next))
      next.once('error', reject)
    })
    return tcpSocketToPacketStream(socket)
  })
  const client = new ResourceClient(
    new ResourceServiceClient(srpc),
    controller.signal,
  )
  const root = await client.accessRootResource()
  const childID = decodeID(
    await root.client.request('test.Root', 'Spawn', bytes('spawn')),
  )
  if (childID === 0) throw new Error('Spawn returned an empty child ID')
  const child = client.createResourceReference(childID)
  if (
    text(
      await child.client.request('test.Child', 'Stream', bytes('stream')),
    ) !== 'later'
  ) {
    throw new Error('Stream did not deliver later data')
  }

  const cancellation = new AbortController()
  const canceled = await nextStreamValue(
    child.client.serverStreamingRequest(
      'test.Child',
      'Block',
      bytes('block'),
      cancellation.signal,
    ),
    'active',
  )
  cancellation.abort()
  await expectAbort(canceled)

  const active = await nextStreamValue(
    child.client.serverStreamingRequest('test.Child', 'Block', bytes('block')),
    'active',
  )
  child.release()
  await expectAbort(active)
  await (
    client as unknown as { waitForControls(): Promise<void> }
  ).waitForControls()
  if (
    text(
      await root.client.request('test.Root', 'Echo', bytes('after-release')),
    ) !== 'after-release'
  ) {
    throw new Error('Root route did not wait for the release acknowledgement')
  }

  root.release()
  const reused = await client.accessRootResource()
  if (
    text(await reused.client.request('test.Root', 'Echo', bytes('reused'))) !==
    'reused'
  ) {
    throw new Error('retained root did not route after release')
  }
  reused.release()
  await (
    client as unknown as { waitForControls(): Promise<void> }
  ).waitForControls()

  const disconnected = new Promise<void>((resolve) => {
    client.onConnectionLost(resolve)
  })
  report('TS_CLIENT_READY_TO_INVALIDATE')
  await disconnected
  if (!root.released || !reused.released || !child.released) {
    throw new Error('server invalidation did not release every reference')
  }
  controller.abort()
  report('TS_CLIENT_OWNER_ZERO')
}

main().catch((error: unknown) => {
  console.error(error)
  process.exitCode = 1
})
