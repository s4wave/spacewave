import { pipe } from 'it-pipe'
import { Pushable, pushable } from 'it-pushable'
import {
  MessagePortDuplex,
  OpenStreamCtr,
  PacketStream,
  castToError,
} from 'starpc'
import { GoWasmProcess } from '../../runtime/wasm/go-process.js'
import { PluginWorker } from '../plugin-worker.js'

// https://github.com/microsoft/TypeScript/issues/14877
declare let self: SharedWorkerGlobalScope | DedicatedWorkerGlobalScope
interface Global {
  BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME?: (
    onMessage: (message: Uint8Array) => void,
    onClose: (errMsg?: string) => void,
  ) => Promise<Pushable<Uint8Array>>
  BLDR_PLUGIN_SET_ACCEPT_STREAM?: (acceptStream: () => MessagePort) => void
}
const global: Global & (SharedWorkerGlobalScope | DedicatedWorkerGlobalScope) =
  self

// baseURL is the base URL to use for paths.
const baseURL = import.meta?.url

// BLDR_PLUGIN_ENTRYPOINT is declared at build time by the plugin compiler.
declare const BLDR_PLUGIN_ENTRYPOINT: string
const pluginEntrypointPath = BLDR_PLUGIN_ENTRYPOINT!

// goOpenStreamCtr contains the function to open a stream with the Go program.
const goOpenStreamCtr = new OpenStreamCtr(undefined)

// pluginWorker contains the common worker logic.
const pluginWorker = new PluginWorker(
  global,
  startGoPlugin,
  // Handle incoming RPC streams for the plugin.
  async (channel: PacketStream) => {
    const goStream = await goOpenStreamCtr.openStreamFunc()
    return pipe(channel, goStream, channel)
  },
)

// The Go runtime will call this function to open outgoing streams.
global.BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME = async (
  onMessage,
  onClose,
): Promise<Pushable<Uint8Array>> => {
  const packetStream = await pluginWorker.webRuntimeClient.openStream()
  const packetSource = packetStream.source
  queueMicrotask(async () => {
    try {
      for await (const msg of packetSource) {
        onMessage(msg)
      }
      onClose()
    } catch (err) {
      const e = castToError(err)
      onClose(e.toString())
    }
  })

  const push = pushable<Uint8Array>({ objectMode: true })
  queueMicrotask(() => packetStream.sink(push))
  return push
}

// The Go runtime will call this function to set a callback for incoming streams.
global.BLDR_PLUGIN_SET_ACCEPT_STREAM = (acceptStrm?: () => MessagePort) => {
  if (!acceptStrm) {
    goOpenStreamCtr.set(undefined)
    return
  }
  goOpenStreamCtr.set(async (): Promise<PacketStream> => {
    return new MessagePortDuplex<Uint8Array>(acceptStrm())
  })
}

function startGoPlugin(startInfoB58: string) {
  // construct the go wasm process
  const goProcess = new GoWasmProcess(
    new URL(pluginEntrypointPath, baseURL).toString(),
    {
      argv: ['plugin.wasm'],
      env: {
        BLDR_PLUGIN_START_INFO: startInfoB58,
      },
      retryOpts: {
        errorCb: (err) => {
          console.warn('plugin-wasm: error executing wasm', err)
          goOpenStreamCtr.set(undefined)
          // TODO notify error to the plugin host & ask if we should retry or terminate
          // webRuntimeClient.notifyError(...)
          self.close() // terminate the shared worker
        },
      },
    },
  )

  // start the Go process
  goProcess.start()
}
