import { RpcStream } from 'starpc'
import type { MessageStream, PacketStream, RpcStreamPacket } from 'starpc'
import { pushable } from 'it-pushable'
import type { Message } from '@aptre/protobuf-es-lite'
import type { ResourceFailure, ResourceRpcPacket } from './resource.pb.js'

const failureBrand = Symbol.for('bldr.ResourceFailureError')

/** ResourceFailureError preserves a lifecycle code across RPC and bundle boundaries. */
export class ResourceFailureError extends Error {
  override readonly name = 'ResourceFailureError'
  readonly [failureBrand] = true

  static [Symbol.hasInstance](value: unknown): boolean {
    return (
      value instanceof Error &&
      failureBrand in value &&
      value[failureBrand] === true
    )
  }

  constructor(readonly failure: Message<ResourceFailure>) {
    super(failure.message || 'resource request failed')
  }
}

/** resourceFailure preserves known lifecycle codes without interpreting diagnostics. */
export function resourceFailure(error: unknown): Message<ResourceFailure> {
  return error instanceof ResourceFailureError
    ? error.failure
    : { message: error instanceof Error ? error.message : String(error) }
}

/** receiveData accepts only SRPC data after the Resource acknowledgement. */
async function* receiveData(
  incoming: AsyncIterator<Message<ResourceRpcPacket>>,
): AsyncGenerator<Message<RpcStreamPacket>> {
  try {
    for (;;) {
      const next = await incoming.next()
      if (next.done) return
      if (next.value.body?.case !== 'data')
        throw new Error('expected ResourceRpc data')
      yield { body: next.value.body }
    }
  } finally {
    await incoming.return?.()
  }
}

/** openResourceRpcStream negotiates a numeric resource with one typed acknowledgement. */
export async function openResourceRpcStream(
  resourceId: number,
  caller: (
    requests: MessageStream<ResourceRpcPacket>,
    signal?: AbortSignal,
  ) => MessageStream<ResourceRpcPacket>,
): Promise<PacketStream> {
  // The outer call owns cancellation even while acknowledgement is pending.
  const controller = new AbortController()
  const outgoing = pushable<Message<RpcStreamPacket>>({ objectMode: true })
  const requests = (async function* (): AsyncGenerator<
    Message<ResourceRpcPacket>
  > {
    yield { body: { case: 'init', value: { resourceId } } }
    for await (const packet of outgoing) {
      if (packet.body?.case !== 'data')
        throw new Error('expected ResourceRpc data')
      yield { body: packet.body }
    }
  })()
  const incoming = caller(requests, controller.signal)[Symbol.asyncIterator]()

  // Keep the existing SRPC transport after accepting the typed route result.
  try {
    const next = await incoming.next()
    if (next.done || next.value.body?.case !== 'ack') {
      throw new Error('expected ResourceRpc acknowledgement')
    }
    const failure = next.value.body.value.failure
    if (failure) throw new ResourceFailureError(failure)
    return new RpcStream(outgoing, receiveData(incoming), () => {
      controller.abort()
      void incoming.return?.().catch(() => {})
    })
  } catch (error) {
    outgoing.end()
    controller.abort()
    await incoming.return?.().catch(() => {})
    throw error
  }
}

/** handleResourceRpcStream resolves a route once and reuses SRPC data framing. */
export async function* handleResourceRpcStream(
  incoming: AsyncIterator<Message<ResourceRpcPacket>>,
  lookup: (
    resourceId: number,
  ) => Promise<(stream: PacketStream) => Promise<void>>,
): AsyncGenerator<Message<ResourceRpcPacket>> {
  const first = await incoming.next()
  if (first.done || first.value.body?.case !== 'init')
    throw new Error('expected ResourceRpc init')
  let handler: (stream: PacketStream) => Promise<void>
  try {
    handler = await lookup(first.value.body.value.resourceId ?? 0)
  } catch (error) {
    yield { body: { case: 'ack', value: { failure: resourceFailure(error) } } }
    return
  }
  yield { body: { case: 'ack', value: {} } }

  // One handler owns the route until cancellation and method teardown finish.
  const outgoing = pushable<Message<RpcStreamPacket>>({ objectMode: true })
  const stream = new RpcStream(outgoing, receiveData(incoming))
  const task = handler(stream).then(
    () => outgoing.end(),
    (error: Error) => outgoing.end(error),
  )
  try {
    for await (const packet of outgoing) {
      if (packet.body?.case !== 'data')
        throw new Error('expected ResourceRpc data')
      yield { body: packet.body }
    }
  } finally {
    await stream.close()
    await task
  }
}
