import { MessageChannel, Worker } from 'node:worker_threads'
import { Client as RpcClient, type PacketStream } from 'starpc'

import { Client as ResourceClient } from '@aptre/bldr-sdk/resource/client.js'
import { ResourceServiceClient } from '../../../bldr/resource/resource_srpc.pb.js'
import { messagePortPacketStream } from '../../../bldr/web/entrypoint/browser/message-port-packet-stream.js'
import { Engine } from '../../../sdk/world/engine.js'
import { RuntimeInit, RuntimeMessage, RuntimeMessage_Kind } from './node.pb.js'

// OwnedEngine retains one Worker and its private Resource connection.
export interface OwnedEngine {
  readonly engine: Engine
  readonly resources: ResourceClient
  readonly signal: AbortSignal
  close(): Promise<void>
}

// openNodeEngine owns the directory until its joined close operation completes.
// Importing this module starts no Worker and opens no storage.
export async function openNodeEngine(directory: string): Promise<OwnedEngine> {
  if (!directory) throw new Error('A sync storage directory is required')
  const worker = new Worker(new URL('./engine-worker.mjs', import.meta.url), {
    workerData: RuntimeInit.toBinary({ directory }),
    // The packaged module has its own compiled entry point and runtime setup.
    execArgv: [],
  })
  const ready = Promise.withResolvers<void>()
  const exited = Promise.withResolvers<void>()
  const controller = new AbortController()
  const streams = new Set<PacketStream>()
  let resources: ResourceClient | undefined
  let failure: Error | undefined
  let closing: Promise<void> | undefined
  let acknowledgedClose = false
  let hasExited = false

  const fail = (error: Error) => {
    failure ??= error
    ready.reject(failure)
    resources?.dispose('CONNECTION_FAILED')
    controller.abort(failure)
    for (const stream of streams) stream.abort(failure)
    streams.clear()
  }
  worker.on('error', fail)
  worker.on('exit', (code) => {
    hasExited = true
    if (!acknowledgedClose)
      fail(new Error(`Sync engine Worker exited unexpectedly (${code})`))
    controller.abort(failure)
    exited.resolve()
  })
  worker.on('message', (data: Uint8Array) => {
    try {
      const message = RuntimeMessage.fromBinary(data)
      switch (message.kind) {
        case RuntimeMessage_Kind.READY:
          ready.resolve()
          break
        case RuntimeMessage_Kind.CLOSED:
          acknowledgedClose = true
          if (message.error) fail(new Error(message.error))
          break
        case RuntimeMessage_Kind.FAILED:
          fail(new Error(message.error || 'Sync engine startup failed'))
          break
        default:
          fail(
            new Error('Sync engine returned an unsupported lifecycle message'),
          )
      }
    } catch (error) {
      fail(error instanceof Error ? error : new Error(String(error)))
    }
  })

  const close = (): Promise<void> => {
    closing ??= (async () => {
      resources?.dispose()
      if (!hasExited) {
        worker.postMessage({
          message: RuntimeMessage.toBinary({ kind: RuntimeMessage_Kind.CLOSE }),
        })
        const timeout = setTimeout(() => {
          fail(new Error('Sync engine shutdown timed out'))
          void worker.terminate()
        }, 10_000)
        try {
          await exited.promise
        } finally {
          clearTimeout(timeout)
        }
      }
      await Promise.all(Array.from(streams, (stream) => stream.close()))
      streams.clear()
      if (failure) throw failure
    })()
    return closing
  }

  const timeout = setTimeout(() => {
    fail(new Error('Sync engine startup timed out'))
    void worker.terminate()
  }, 30_000)
  try {
    await ready.promise
    const rpc = new RpcClient(async () => {
      if (failure) throw failure
      if (closing || hasExited) throw new Error('Sync engine is closed')
      const { port1, port2 } = new MessageChannel()
      const stream = messagePortPacketStream(port1)
      const closeStream = stream.close.bind(stream)
      const abortStream = stream.abort.bind(stream)
      stream.close = async () => {
        streams.delete(stream)
        await closeStream()
      }
      stream.abort = (error) => {
        streams.delete(stream)
        abortStream(error)
      }
      streams.add(stream)
      worker.postMessage(
        {
          message: RuntimeMessage.toBinary({
            kind: RuntimeMessage_Kind.OPEN_STREAM,
          }),
          port: port2,
        },
        [port2],
      )
      return stream
    })
    resources = new ResourceClient(
      new ResourceServiceClient(rpc),
      controller.signal,
    )
    const engine = new Engine(await resources.accessRootResource())
    return { engine, resources, signal: controller.signal, close }
  } catch (error) {
    try {
      await close()
    } catch {
      // Construction reports its original failure after joining the Worker.
    }
    throw error
  } finally {
    clearTimeout(timeout)
  }
}
