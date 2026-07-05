import { pipe } from 'it-pipe'
import { pushable } from 'it-pushable'
import type { PacketStream } from 'starpc'
import { BackendAPI } from '@aptre/bldr-sdk'
import { PluginStartInfo } from '../../../plugin/plugin.pb.js'

type GoPushableSink = {
  push: (message: Uint8Array) => void
  end: () => void
}

export type GoScriptPluginMain = () => void | Promise<void>
export type GoScriptPluginMainLoader = () => Promise<GoScriptPluginMain>

declare global {
  var BLDR_BASE_URL: string
  var BLDR_PLUGIN_START_INFO: string | undefined
  var BLDR_PLUGIN_REPORT_RUNTIME_FAILURE: ((err: unknown) => void) | undefined
  var BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME:
    | ((
        onMessage: (message: Uint8Array) => void,
        onClose: (errMsg?: string) => void,
        onResolve: (sink: GoPushableSink) => void,
        onReject: (errMsg: string) => void,
      ) => void)
    | undefined
  var BLDR_PLUGIN_SET_ACCEPT_STREAM:
    | ((acceptStream?: (localPort: MessagePort) => void) => void)
    | undefined
}

const globalScope = globalThis
const baseURL = import.meta?.url
globalScope.BLDR_BASE_URL = baseURL

class GoScriptPluginGeneration {
  private readonly activeAcceptedStreams = new Set<BrowserMessagePortDuplex>()
  private terminalError?: Error

  public constructor(private readonly api: BackendAPI) {}

  public start(
    startInfo: PluginStartInfo,
    loadPluginMain: GoScriptPluginMainLoader,
  ) {
    const pluginStartInfoJsonB64 = btoa(PluginStartInfo.toJsonString(startInfo))
    globalScope.BLDR_PLUGIN_START_INFO = pluginStartInfoJsonB64

    void Promise.resolve()
      .then(() => loadPluginMain())
      .then((pluginMain) => pluginMain())
      .then(
        () => this.fail(new Error('GoScript plugin process exited')),
        (err) => this.fail(err),
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

      const duplex = new BrowserMessagePortDuplex(messageChannel.port2)
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

    const terminalError = castToError(err, 'GoScript plugin process failed')
    this.terminalError = terminalError
    console.warn(
      'plugin-goscript: GoScript plugin process exited',
      terminalError,
    )
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

export default async function main(
  api: BackendAPI,
  loadPluginMain: GoScriptPluginMainLoader,
): Promise<void> {
  const generation = new GoScriptPluginGeneration(api)

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
        onClose(errMsg)
      }
      const deliverMessage = async (msg: Uint8Array): Promise<boolean> => {
        if (callbacksClosed) {
          return false
        }
        onMessage(msg)
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
          closeCallbacks(castToError(err).toString())
        }
      })

      queueMicrotask(() => {
        void packetStream.sink(push).catch((err) => {
          closeCallbacks(castToError(err).toString())
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
        onResolve(sink)
      },
      (err) => {
        onReject(castToError(err).toString())
      },
    )
  }

  globalScope.BLDR_PLUGIN_SET_ACCEPT_STREAM = (
    acceptStrm?: (localPort: MessagePort) => void,
  ) => {
    generation.setAcceptStream(acceptStrm)
  }

  generation.start(api.startInfo, loadPluginMain)
}

class BrowserMessagePortDuplex {
  public readonly source: AsyncIterable<Uint8Array>
  private readonly outbound: ReturnType<typeof pushable<Uint8Array>>

  public constructor(private readonly port: MessagePort) {
    this.outbound = pushable<Uint8Array>({ objectMode: true })
    this.source = this.outbound
    this.port.onmessage = (ev: MessageEvent<Uint8Array | null>) => {
      if (ev.data === null) {
        this.close()
        return
      }
      this.outbound.push(copyUint8Array(ev.data))
    }
    this.port.onmessageerror = () => {
      this.close()
    }
    this.port.start()
  }

  public async sink(source: AsyncIterable<Uint8Array>): Promise<void> {
    try {
      for await (const message of source) {
        this.port.postMessage(copyUint8Array(message))
      }
    } finally {
      this.close()
    }
  }

  public close(): void {
    this.outbound.end()
    this.port.close()
  }
}

function closeMessagePortDuplex(duplex: BrowserMessagePortDuplex) {
  try {
    duplex.close()
  } catch {
    // ignored: the port may already be closed by the pipe.
  }
}

function castToError(err: unknown, fallback = 'unknown error'): Error {
  if (err instanceof Error) {
    return err
  }
  if (typeof err === 'string') {
    return new Error(err)
  }
  return new Error(fallback)
}

function copyUint8Array(bytes: Uint8Array): Uint8Array<ArrayBuffer> {
  const copy = new Uint8Array(bytes.byteLength)
  copy.set(bytes)
  return copy
}
