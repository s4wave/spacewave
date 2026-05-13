import { pipe } from 'it-pipe'
import { Pushable, pushable } from 'it-pushable'
import { MessagePortDuplex, PacketStream, castToError } from 'starpc'
import { GoWasmProcess } from '../../runtime/wasm/go-process.js'
import { BackendAPI } from '@aptre/bldr-sdk'
import { PluginStartInfo } from '../../../plugin/plugin.pb.js'

interface Global {
  BLDR_BASE_URL: string
  BLDR_PLUGIN_REPORT_RUNTIME_FAILURE?: (err: unknown) => void
  BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME?: (
    onMessage: (message: Uint8Array) => void,
    onClose: (errMsg?: string) => void,
  ) => Promise<Pushable<Uint8Array>>
  BLDR_PLUGIN_SET_ACCEPT_STREAM?: (acceptStream: () => MessagePort) => void
}

// globalScope is globalThis but with the bldr globals.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const globalScope: Global = globalThis as any

// baseURL is the base URL to use for paths relative to this module.
const baseURL = import.meta?.url
globalScope.BLDR_BASE_URL = baseURL

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

  public setAcceptStream(acceptStrm?: () => MessagePort) {
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

      const duplex = new MessagePortDuplex<Uint8Array>(acceptStrm())
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
  globalScope.BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME = async (
    onMessage,
    onClose,
  ): Promise<Pushable<Uint8Array>> => {
    const packetStream = await api.openStream()
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
