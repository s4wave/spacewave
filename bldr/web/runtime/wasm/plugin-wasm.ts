import { pipe } from 'it-pipe'
import { pushable } from 'it-pushable'
import { MessagePortDuplex, PacketStream, castToError } from 'starpc'
import { GoWasmProcess } from '../../runtime/wasm/go-process.js'
import { BackendAPI } from '@aptre/bldr-sdk'
import { PluginStartInfo } from '../../../plugin/plugin.pb.js'

interface Global {
  BLDR_BASE_URL: string
  BLDR_PLUGIN_START_INFO?: string
  BLDR_PLUGIN_REPORT_RUNTIME_FAILURE?: (err: unknown) => void
  BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME?: (
    onMessage: (message: Uint8Array) => void,
    onClose: (errMsg?: string) => void,
    onResolve: (sink: GoPushableSink) => void,
    onReject: (errMsg: string) => void,
  ) => void
  BLDR_PLUGIN_SET_ACCEPT_STREAM?: (
    acceptStream: (localPort: MessagePort) => void,
  ) => void
}

type GoPushableSink = {
  push: (message: Uint8Array) => void
  end: () => void
}

// globalScope is globalThis but with the bldr globals.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const globalScope: Global = globalThis as any

// baseURL is the base URL to use for paths relative to this module.
const baseURL = import.meta?.url
globalScope.BLDR_BASE_URL = baseURL

const goCallbackQueue: (() => void)[] = []
let goCallbackScheduled = false
let goCallbackChannel: MessageChannel | undefined

// BLDR_PLUGIN_ENTRYPOINT is declared at build time by the plugin compiler.
declare const BLDR_PLUGIN_ENTRYPOINT: string
const pluginEntrypointPath = BLDR_PLUGIN_ENTRYPOINT

class WasmPluginGeneration {
  private readonly activeAcceptedStreams = new Set<
    MessagePortDuplex<Uint8Array>
  >()
  private terminalError?: Error

  public constructor(
    private readonly api: BackendAPI,
    private readonly abortSignal?: AbortSignal,
  ) {}

  public start(startInfo: PluginStartInfo) {
    const pluginStartInfoJsonB64 = btoa(PluginStartInfo.toJsonString(startInfo))
    globalScope.BLDR_PLUGIN_START_INFO = pluginStartInfoJsonB64
    const goProcess = new GoWasmProcess(
      new URL(pluginEntrypointPath, baseURL).toString(),
      {
        argv: ['plugin.wasm'],
        env: {
          BLDR_PLUGIN_START_INFO: pluginStartInfoJsonB64,
        },
        abortSignal: this.abortSignal,
        retry: false,
      },
    )

    const result = goProcess.start()
    void result.then(
      () => {
        if (!this.abortSignal?.aborted) {
          this.fail(new Error('Go WASM process exited'))
        }
      },
      (err) => {
        if (!this.abortSignal?.aborted) {
          this.fail(err)
        }
      },
    )
  }

  public setAcceptStream(acceptStrm?: (localPort: MessagePort) => void) {
    if (this.terminalError) {
      this.installTerminalAcceptHandler(this.terminalError)
      return
    }
    if (!acceptStrm) {
      this.api.handleStreamCtr.set(undefined)
      return
    }

    this.api.handleStreamCtr.set(async (channel: PacketStream) => {
      if (this.terminalError) {
        throw this.terminalError
      }

      const messageChannel = new MessageChannel()
      let accepted = false
      try {
        acceptStrm(messageChannel.port1)
        accepted = true
      } finally {
        if (!accepted) {
          messageChannel.port1.close()
          messageChannel.port2.close()
        }
      }

      const duplex = new MessagePortDuplex<Uint8Array>(messageChannel.port2)
      this.activeAcceptedStreams.add(duplex)
      try {
        await pipe(channel, duplex, channel)
      } finally {
        this.activeAcceptedStreams.delete(duplex)
        closeMessagePortDuplex(duplex)
      }
    })
  }

  private fail(err: unknown) {
    if (this.terminalError) {
      return
    }

    const terminalError = castToError(err, 'Go WASM process failed')
    this.terminalError = terminalError
    console.warn('plugin-wasm: Go WASM process exited', terminalError)
    this.closeActiveAcceptedStreams()
    this.installTerminalAcceptHandler(terminalError)
    globalScope.BLDR_PLUGIN_REPORT_RUNTIME_FAILURE?.(terminalError)
  }

  private closeActiveAcceptedStreams() {
    for (const duplex of this.activeAcceptedStreams) {
      closeMessagePortDuplex(duplex)
    }
    this.activeAcceptedStreams.clear()
  }

  private installTerminalAcceptHandler(err: Error) {
    this.api.handleStreamCtr.set(async () => {
      throw err
    })
  }
}

// Main function exported by this module.
export default async function main(
  api: BackendAPI,
  abortSignal?: AbortSignal,
): Promise<void> {
  const generation = new WasmPluginGeneration(api, abortSignal)

  // The Go runtime will call this function to open outgoing streams.
  globalScope.BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME = (
    onMessage,
    onClose,
    onResolve,
    onReject,
  ): void => {
    void (async () => {
      const packetStream = await api.openStream()
      const packetSource = packetStream.source
      const push = pushable<Uint8Array>({ objectMode: true })
      let callbacksClosed = false
      const closeCallbacks = (errMsg?: string) => {
        if (callbacksClosed) {
          return
        }
        callbacksClosed = true
        deferGoCallback(() => {
          callGoCallback(() => {
            onClose(errMsg)
          })
        })
      }
      const deliverMessage = async (msg: Uint8Array): Promise<boolean> => {
        if (callbacksClosed) {
          return false
        }
        const released = await deferGoCallbackResult(() =>
          callGoCallback(() => {
            onMessage(msg)
          }),
        )
        if (released) {
          callbacksClosed = true
          return false
        }
        return true
      }
      queueMicrotask(async () => {
        try {
          for await (const msg of packetSource) {
            if (!(await deliverMessage(msg))) {
              push.end()
              return
            }
          }
          closeCallbacks()
        } catch (err) {
          const e = castToError(err)
          closeCallbacks(e.toString())
        }
      })

      queueMicrotask(() => {
        void packetStream.sink(push).catch((err) => {
          const e = castToError(err)
          closeCallbacks(e.toString())
        })
      })
      return {
        push: (message: Uint8Array) => {
          push.push(message)
        },
        end: () => {
          push.end()
        },
      }
    })().then(
      (sink) => {
        deferGoCallback(() => {
          const released = callGoCallback(() => {
            onResolve(sink)
          })
          if (released) {
            sink.end()
          }
        })
      },
      (err) => {
        deferGoCallback(() => {
          callGoCallback(() => {
            onReject(castToError(err).toString())
          })
        })
      },
    )
  }

  // The Go runtime will call this function to set a callback for incoming streams.
  globalScope.BLDR_PLUGIN_SET_ACCEPT_STREAM = (
    acceptStrm?: (localPort: MessagePort) => void,
  ) => {
    generation.setAcceptStream(acceptStrm)
  }

  // Start the Go plugin, passing the startInfo from the API
  generation.start(api.startInfo)
}

function closeMessagePortDuplex(duplex: MessagePortDuplex<Uint8Array>) {
  try {
    duplex.close()
  } catch {
    // ignored: the port may already be closed by the pipe.
  }
}

function flushGoCallbacks(): void {
  goCallbackScheduled = false
  const callbacks = goCallbackQueue.splice(0)
  for (const callback of callbacks) {
    callback()
  }
  if (goCallbackQueue.length !== 0) {
    scheduleGoCallbackFlush()
  }
}

function scheduleGoCallbackFlush(): void {
  if (goCallbackScheduled) {
    return
  }
  goCallbackScheduled = true
  if (typeof MessageChannel === 'function') {
    goCallbackChannel ??= new MessageChannel()
    goCallbackChannel.port1.onmessage = flushGoCallbacks
    goCallbackChannel.port2.postMessage(undefined)
    return
  }
  setTimeout(flushGoCallbacks, 0)
}

function deferGoCallback(callback: () => void): void {
  goCallbackQueue.push(callback)
  scheduleGoCallbackFlush()
}

function deferGoCallbackResult<T>(callback: () => T): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    deferGoCallback(() => {
      try {
        resolve(callback())
      } catch (err) {
        reject(err)
      }
    })
  })
}

function isReleasedGoCallbackError(err: unknown): boolean {
  if (typeof err === 'string') {
    return err === 'call to released function'
  }
  return err instanceof Error && err.message === 'call to released function'
}

function callGoCallback(cb: () => void): boolean {
  const consoleError = console.error
  let released = false
  console.error = (...args: unknown[]) => {
    // Go may release a callback before this owner invokes it. Filter that
    // known callback edge only inside the invocation, never for the whole worker.
    if (args.some(isReleasedGoCallbackError)) {
      released = true
      return
    }
    consoleError(...args)
  }
  try {
    cb()
  } catch (err) {
    if (isReleasedGoCallbackError(err)) {
      released = true
      return released
    }
    throw err
  } finally {
    console.error = consoleError
  }
  return released
}
