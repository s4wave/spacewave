import { pipe } from 'it-pipe'
import { Pushable, pushable } from 'it-pushable'
import { MessagePortDuplex, PacketStream, castToError } from 'starpc'
import { GoWasmProcess } from '../../runtime/wasm/go-process.js'
import { BackendAPI } from '@aptre/bldr-sdk'

interface Global {
  BLDR_BASE_URL: string,
  BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME?: (
    onMessage: (message: Uint8Array) => void,
    onClose: (errMsg?: string) => void,
  ) => Promise<Pushable<Uint8Array>>
  BLDR_PLUGIN_SET_ACCEPT_STREAM?: (acceptStream: () => MessagePort) => void
}

// globalScope is globalThis but with the bldr globals.
const globalScope: Global = globalThis as any

// baseURL is the base URL to use for paths relative to this module.
const baseURL = import.meta?.url
globalScope.BLDR_BASE_URL = baseURL

// BLDR_PLUGIN_ENTRYPOINT is declared at build time by the plugin compiler.
declare const BLDR_PLUGIN_ENTRYPOINT: string
const pluginEntrypointPath = BLDR_PLUGIN_ENTRYPOINT!

// startGoPlugin starts the go wasm process.
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
          // TODO: How should errors be propagated back here?
          // Consider clearing the acceptStreamCtr if the wasm crashes irrecoverably.
          // api.acceptStreamCtr.set(undefined);
        },
      },
    },
  )

  // start the Go process
  goProcess.start()
}

// Main function exported by this module.
export default async function main(api: BackendAPI): Promise<void> {
  // The Go runtime will call this function to open outgoing streams.
  globalScope.BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME = async (
    onMessage,
    onClose,
  ): Promise<Pushable<Uint8Array>> => {
    const packetStream = await api.webRuntimeClient.openStream()
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
  globalScope.BLDR_PLUGIN_SET_ACCEPT_STREAM = (
    acceptStrm?: () => MessagePort,
  ) => {
    if (!acceptStrm) {
      // Unregister the handler from the shared worker side via the API controller
      api.handleStreamCtr.set(undefined)
      return
    }

    // Create the handler function that converts the MessagePort to a PacketStream
    const handler = async (channel: PacketStream): Promise<void> => {
      const duplex = new MessagePortDuplex<Uint8Array>(acceptStrm())
      await pipe(channel, duplex, channel)
    }

    // Register the handler with the shared worker side via the API controller
    api.handleStreamCtr.set(handler)
  }

  // Start the Go plugin, passing the startInfo from the API
  startGoPlugin(api.startInfoB58)
}
