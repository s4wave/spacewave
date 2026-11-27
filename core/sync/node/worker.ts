import { isMainThread, parentPort, workerData } from 'node:worker_threads'
import { createHandler, createMux, Server } from 'starpc'

import { SqliteNodeServer } from '../../../db/sql/sqlite-node/server.js'
import { SqliteBridgeDefinition } from '../../../db/sql/sqlite-wasm/rpc/sqlite-bridge_srpc.pb.js'
import { messagePortPacketStream } from '../../../bldr/web/entrypoint/browser/message-port-packet-stream.js'
import { acquireDirectoryLock } from '../../../db/sql/sqlite-node/lock.js'
import { RuntimeInit, RuntimeMessage, RuntimeMessage_Kind } from './node.pb.js'

// EngineRuntime is the compiled engine capability supplied by the build entrypoint.
interface EngineRuntime {
  accept(port: MessagePort): Promise<void>
  close(): Promise<void>
}

// OpenRuntime binds the compiled engine to this Worker's private SQL-port opener.
type OpenRuntime = (
  directory: string,
  openSQLPort: () => MessagePort,
) => Promise<EngineRuntime>

// openEngine connects the compiled engine to SQLite within this Worker.
async function openEngine(directory: string, openRuntime: OpenRuntime) {
  const sqlite = new SqliteNodeServer()
  const mux = createMux()
  mux.register(createHandler(SqliteBridgeDefinition, sqlite))
  const server = new Server(mux.lookupMethod)
  const openPort = () => {
    const { port1, port2 } = new MessageChannel()
    server.handlePacketStream(messagePortPacketStream(port1))
    return port2
  }
  try {
    const runtime = await openRuntime(directory, openPort)
    return { runtime, sqlite }
  } catch (error) {
    sqlite.close()
    throw error
  }
}

// describeFailure renders a thrown value as text. An unrecovered Go panic is an
// Error whose message reads "panic: <value>". A compiled Go error that is not
// an Error carries its text behind an Error() method, which may be async.
async function describeFailure(reason: unknown): Promise<string> {
  if (reason instanceof Error) return reason.message
  const goError = reason as { Error?: () => unknown } | null | undefined
  if (typeof goError?.Error === 'function') return String(await goError.Error())
  return String(reason)
}

// serveWorker publishes readiness only after SQLite and the World are available.
async function serveWorker(openRuntime: OpenRuntime): Promise<void> {
  if (!parentPort) throw new Error('Sync engine requires a Worker parent')
  const parent = parentPort
  const init = RuntimeInit.fromBinary(workerData)
  const lock = acquireDirectoryLock(init.directory ?? '')
  let opened: Awaited<ReturnType<typeof openEngine>> | undefined
  try {
    opened = await openEngine(lock.directory, openRuntime)
    const { runtime, sqlite } = opened
    let closing: Promise<void> | undefined
    parent.on(
      'message',
      (frame: { message: Uint8Array; port?: MessagePort }) => {
        const message = RuntimeMessage.fromBinary(frame.message)
        if (message.kind === RuntimeMessage_Kind.OPEN_STREAM && frame.port) {
          if (closing) {
            frame.port.close()
            return
          }
          void runtime.accept(frame.port).catch(() => frame.port?.close())
        } else if (message.kind === RuntimeMessage_Kind.CLOSE) {
          closing ??= (async () => {
            let failure = ''
            try {
              await runtime.close()
            } catch (error) {
              failure = await describeFailure(error)
            } finally {
              try {
                sqlite.close()
              } catch (error) {
                failure ||= await describeFailure(error)
              } finally {
                try {
                  lock.close()
                } catch (error) {
                  failure ||= await describeFailure(error)
                }
              }
            }
            parent.postMessage(
              RuntimeMessage.toBinary({
                kind: RuntimeMessage_Kind.CLOSED,
                error: failure,
              }),
            )
          })()
        } else {
          frame.port?.close()
          parent.postMessage(
            RuntimeMessage.toBinary({
              kind: RuntimeMessage_Kind.FAILED,
              error: 'Unsupported sync engine lifecycle request',
            }),
          )
        }
      },
    )
    parent.postMessage(
      RuntimeMessage.toBinary({ kind: RuntimeMessage_Kind.READY }),
    )
  } catch (error) {
    try {
      opened?.sqlite.close()
    } finally {
      lock.close()
    }
    throw error
  }
}

// runWorker starts the host after the generated entrypoint supplies its compiled binding.
//
// A failed start and an unrecovered panic in a compiled goroutine both reach
// the host as FAILED with their message. A Go process exits on such a panic,
// so the host ends the Worker when FAILED arrives.
export function runWorker(openRuntime: OpenRuntime): void {
  if (isMainThread) throw new Error('Sync engine requires a Worker')
  const fail = (reason: unknown) => {
    void describeFailure(reason).then((error) =>
      parentPort?.postMessage(
        RuntimeMessage.toBinary({ kind: RuntimeMessage_Kind.FAILED, error }),
      ),
    )
  }
  process.on('uncaughtException', fail)
  process.on('unhandledRejection', fail)
  void serveWorker(openRuntime).catch(fail)
}
