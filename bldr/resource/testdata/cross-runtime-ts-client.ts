import net from 'net'

import { pipe } from 'it-pipe'
import { pushable } from 'it-pushable'
import type { Source } from 'it-stream-types'
import {
  Client as SRPCClient,
  StaticHandler,
  combineUint8ArrayListTransform,
  createMux,
  parseLengthPrefixTransform,
  prependLengthPrefixTransform,
} from 'starpc'
import type { InvokeFn, PacketStream } from 'starpc'

import { Client as ResourceClient } from '../../sdk/resource/client.js'
import {
  ResourceAttachAddAck,
  ResourceClientInitRequest,
} from '../resource.pb.js'
import { ResourceServiceClient } from '../resource_srpc.pb.js'

function tcpSocketToPacketStream(socket: net.Socket): PacketStream {
  const socketSource = async function* (): AsyncGenerator<Uint8Array> {
    const source = pushable<Uint8Array>({ objectMode: true })
    socket.on('data', (data: Buffer) => {
      source.push(new Uint8Array(data))
    })
    socket.on('end', () => source.end())
    socket.on('error', (err) => source.end(err))
    socket.on('close', () => source.end())
    yield* pipe(
      source,
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
    source: socketSource(),
    sink: async (source: Source<Uint8Array>): Promise<void> => {
      for await (const chunk of pipe(source, prependLengthPrefixTransform())) {
        const data =
          chunk instanceof Uint8Array
            ? chunk
            : (chunk as { subarray(): Uint8Array }).subarray()
        await new Promise<void>((resolve, reject) => {
          socket.write(data, (err) => {
            if (err) reject(err)
            else resolve()
          })
        })
      }
      socket.end()
    },
  }
}

async function readOne(source: Source<Uint8Array>): Promise<void> {
  await source[Symbol.asyncIterator]().next()
}

async function* single(data: Uint8Array): AsyncGenerator<Uint8Array> {
  yield data
}

function unaryResponse(data: Uint8Array): Source<Uint8Array> {
  return single(data)
}

async function main(): Promise<void> {
  const addr = process.argv[2]
  if (!addr) throw new Error('usage: cross-runtime-ts-client <host:port>')

  const [host, portStr] = addr.split(':')
  const port = Number.parseInt(portStr, 10)
  if (!host || !port) throw new Error(`invalid address: ${addr}`)

  const controller = new AbortController()
  const openStream = async (): Promise<PacketStream> => {
    return new Promise((resolve, reject) => {
      const socket = net.connect(port, host, () => {
        resolve(tcpSocketToPacketStream(socket))
      })
      socket.on('error', reject)
    })
  }

  const srpc = new SRPCClient(openStream)
  const service = new ResourceServiceClient(srpc)
  const client = new ResourceClient(service, controller.signal)

  let releaseChild: (() => void) | undefined
  const childReleased = new Promise<void>((resolve) => {
    releaseChild = resolve
  })

  const childMux = createMux()
  childMux.register(
    new StaticHandler('test.Child', {
      Ping: async (dataSource, dataSink) => {
        await readOne(dataSource)
        await dataSink(unaryResponse(ResourceClientInitRequest.toBinary({})))
      },
    } satisfies Record<string, InvokeFn>),
  )

  const rootMux = createMux()
  rootMux.register(
    new StaticHandler('test.Root', {
      CreateChild: async (dataSource, dataSink) => {
        await readOne(dataSource)
        const child = await client.attachResourceTree(
          'ts-child',
          childMux.lookupMethod,
          undefined,
          releaseChild,
        )
        await dataSink(
          unaryResponse(
            ResourceAttachAddAck.toBinary({
              resourceId: child.resourceId,
            }),
          ),
        )
      },
    } satisfies Record<string, InvokeFn>),
  )

  const root = await client.attachResourceTree('ts-root', rootMux.lookupMethod)
  const rootRef = client.createResourceReference(root.resourceId)
  const childBytes = await rootRef.client.request(
    'test.Root',
    'CreateChild',
    ResourceClientInitRequest.toBinary({}),
    controller.signal,
  )
  const child = ResourceAttachAddAck.fromBinary(childBytes)
  if (!child.resourceId)
    throw new Error('CreateChild returned empty resource id')

  const childRef = client.createResourceReference(child.resourceId)
  const pingBytes = await childRef.client.request(
    'test.Child',
    'Ping',
    ResourceClientInitRequest.toBinary({}),
    controller.signal,
  )
  ResourceClientInitRequest.fromBinary(pingBytes)

  childRef.release()
  await childReleased

  root.cleanup()
  controller.abort()
  client.dispose()

  console.log(
    JSON.stringify({
      childResourceId: child.resourceId,
      ok: true,
      rootResourceId: root.resourceId,
    }),
  )
}

main().catch((err) => {
  console.error(err)
  process.exit(1)
})
