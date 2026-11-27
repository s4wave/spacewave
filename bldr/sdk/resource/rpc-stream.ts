import { RpcStream, ERR_RPC_ABORT } from 'starpc'
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
  retain = false,
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
    if (!retain) await incoming.return?.()
  }
}

/** ResourceRpcStream keeps a unary route open until its result is settled. */
export interface ResourceRpcStream extends PacketStream {
  /** finish abandons an unsuccessful unary result and closes its route. */
  finish(abandon: boolean): void
}

/** openResourceRpcStream negotiates a numeric resource with one typed acknowledgement. */
export async function openResourceRpcStream(
  resourceId: number,
  caller: (
    requests: MessageStream<ResourceRpcPacket>,
    signal?: AbortSignal,
  ) => MessageStream<ResourceRpcPacket>,
  retain = false,
  generationSignal?: AbortSignal,
): Promise<ResourceRpcStream> {
  // The outer call owns cancellation even while acknowledgement is pending.
  const controller = new AbortController()
  const outgoing = pushable<Message<ResourceRpcPacket>>({ objectMode: true })
  const generationEnded = () => {
    outgoing.end()
    controller.abort()
  }
  generationSignal?.addEventListener('abort', generationEnded, { once: true })
  if (generationSignal?.aborted) generationEnded()
  let requestsClosed!: () => void
  const requestsDone = new Promise<void>((resolve) => {
    requestsClosed = resolve
  })
  const requests = (async function* (): AsyncGenerator<
    Message<ResourceRpcPacket>
  > {
    try {
      yield { body: { case: 'init', value: { resourceId } } }
      yield* outgoing
    } finally {
      requestsClosed()
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
    const close = () => {
      generationSignal?.removeEventListener('abort', generationEnded)
      controller.abort()
      void Promise.resolve(incoming.return?.()).catch(() => {})
    }
    let finished = false
    // RpcStream calls only push and end on its packet writer.
    const stream = new RpcStream(
      {
        push: (packet) => {
          if (packet.body?.case !== 'data') {
            throw new Error('expected ResourceRpc data')
          }
          outgoing.push({ body: packet.body })
        },
        end: (error) => {
          if (!retain) outgoing.end(error)
        },
      } as ConstructorParameters<typeof RpcStream>[0],
      receiveData(incoming, retain),
      retain ? () => {} : close,
    )
    return Object.assign(stream, {
      finish(abandon: boolean) {
        // Queue abandonment before ending the request stream or canceling it.
        if (finished) return
        finished = true
        if (abandon) outgoing.push({ body: { case: 'abandon', value: {} } })
        outgoing.end()
        if (abandon) {
          void requestsDone.then(close)
          return
        }
        close()
      },
    })
  } catch (error) {
    generationSignal?.removeEventListener('abort', generationEnded)
    outgoing.end()
    controller.abort()
    await Promise.resolve(incoming.return?.()).catch(() => {})
    throw error
  }
}

/** handleResourceRpcStream resolves a route once and reuses SRPC data framing. */
export async function* handleResourceRpcStream(
  incoming: AsyncIterator<Message<ResourceRpcPacket>>,
  lookup: (
    resourceId: number,
    onAbandon: (cleanup: () => void) => void,
  ) => Promise<(stream: PacketStream) => Promise<void>>,
  signal?: AbortSignal,
): AsyncGenerator<Message<ResourceRpcPacket>> {
  const first = await incoming.next()
  if (first.done || first.value.body?.case !== 'init')
    throw new Error('expected ResourceRpc init')
  let handler: (stream: PacketStream) => Promise<void>
  let abandoned = false
  const callbacks = new Set<() => void>()
  const onAbandon = (cleanup: () => void) => {
    if (abandoned) cleanup()
    else callbacks.add(cleanup)
  }
  try {
    handler = await lookup(first.value.body.value.resourceId ?? 0, onAbandon)
  } catch (error) {
    yield { body: { case: 'ack', value: { failure: resourceFailure(error) } } }
    return
  }
  yield { body: { case: 'ack', value: {} } }

  // Keep the route reader alive after nested completion for late Abandon packets.
  const outgoing = pushable<Message<RpcStreamPacket>>({ objectMode: true })
  const data = pushable<Message<RpcStreamPacket>>({ objectMode: true })
  const stream = new RpcStream(
    {
      push: (packet) => outgoing.push(packet),
      end: () => {},
    } as ConstructorParameters<typeof RpcStream>[0],
    data,
    () => {},
  )
  const abort = () => {
    stream.abort(new Error(ERR_RPC_ABORT))
    data.end(new Error(ERR_RPC_ABORT))
    outgoing.end()
    void Promise.resolve(incoming.return?.()).catch(() => {})
  }
  signal?.addEventListener('abort', abort, { once: true })
  if (signal?.aborted) abort()
  const reading = (async () => {
    try {
      for (;;) {
        const next = await incoming.next()
        if (next.done) break
        if (next.value.body?.case === 'abandon') {
          abandoned = true
          for (const cleanup of callbacks) cleanup()
          callbacks.clear()
          data.end(new Error(ERR_RPC_ABORT))
        } else if (next.value.body?.case === 'data') {
          if (!abandoned) data.push({ body: next.value.body })
        } else {
          throw new Error('expected ResourceRpc data or abandonment')
        }
      }
    } finally {
      // EOF ends the route, including a streaming method awaiting cancellation.
      stream.abort(new Error(ERR_RPC_ABORT))
      data.end()
      outgoing.end()
      signal?.removeEventListener('abort', abort)
    }
  })().catch((error: Error) => outgoing.end(error))
  const serving = handler(stream).catch((error: Error) => outgoing.end(error))
  try {
    for await (const packet of outgoing) {
      if (packet.body?.case !== 'data')
        throw new Error('expected ResourceRpc data')
      yield { body: packet.body }
    }
  } finally {
    await stream.close()
    // The reader owns late abandonment until request EOF; do not wait for that
    // EOF before delivering a handler failure to the caller that must send it.
    void reading
    await serving
  }
}
