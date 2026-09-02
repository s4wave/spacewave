import type { PacketStream } from 'starpc'
import type { BackendAPI, BackendEntrypointLifecycle } from '@aptre/bldr-sdk'
import { PluginStartInfo } from '../../../plugin/plugin.pb.js'
import {
  castToError,
  packetStreamToOpenStreamCallbacks,
  type OpenStreamFunc,
} from './open-stream-contract.js'

// GoScriptPluginMain is the plugin's main entrypoint function.
export type GoScriptPluginMain = () => void | Promise<void>
// GoScriptPluginMainLoader loads the plugin's main entrypoint function.
export type GoScriptPluginMainLoader = () => Promise<GoScriptPluginMain>

// The outgoing stream global is shared with the browser runtime
// (plugin-goscript.ts): the Go side invokes it with the same four callbacks.
// The accepted-stream global is Workers-specific: instead of a MessagePort,
// each accepted stream is delivered as an openStreamFunc with the shared
// callback contract (see open-stream-contract.ts).
declare global {
  var BLDR_PLUGIN_SET_ACCEPT_STREAM_WORKERS:
    | ((acceptStream?: (openStreamFunc: OpenStreamFunc) => void) => void)
    | undefined
}

const globalScope = globalThis
const baseURL = import.meta?.url
globalScope.BLDR_BASE_URL = baseURL

class GoScriptCloudflarePluginGeneration {
  private readonly activeAcceptedStreams = new Set<(err?: Error) => void>()
  private readonly startup = Promise.withResolvers<void>()
  private readonly done = Promise.withResolvers<void>()
  private terminalError?: Error

  public constructor(private readonly api: BackendAPI) {
    void this.startup.promise.catch(() => {})
    void this.done.promise.catch(() => {})
  }

  public start(
    startInfo: PluginStartInfo,
    loadPluginMain: GoScriptPluginMainLoader,
  ): BackendEntrypointLifecycle {
    const pluginStartInfoJsonB64 = btoa(PluginStartInfo.toJsonString(startInfo))
    globalScope.BLDR_PLUGIN_START_INFO = pluginStartInfoJsonB64

    void Promise.resolve()
      .then(() => loadPluginMain())
      .then((pluginMain) => pluginMain())
      .then(
        () => this.fail(new Error('GoScript plugin process exited')),
        (err) => this.fail(err),
      )

    return {
      startup: this.startup.promise,
      done: this.done.promise,
    }
  }

  public markReady() {
    if (this.terminalError) {
      return
    }
    this.startup.resolve()
  }

  public setAcceptStream(
    acceptStream?: (openStreamFunc: OpenStreamFunc) => void,
  ) {
    if (this.terminalError) {
      this.installTerminalAcceptHandler(this.terminalError)
      return
    }
    if (!acceptStream) {
      this.api.handleStreamCtr.set(undefined)
      return
    }

    this.api.handleStreamCtr.set(async (channel: PacketStream) => {
      if (this.terminalError) {
        throw this.terminalError
      }
      await this.acceptStream(channel, acceptStream)
    })
    this.markReady()
  }

  // acceptStream delivers one incoming channel to the Go plugin. The channel
  // is wrapped as an openStreamFunc; the handler resolves when the stream
  // closes in either direction.
  private async acceptStream(
    channel: PacketStream,
    acceptStreamFn: (openStreamFunc: OpenStreamFunc) => void,
  ): Promise<void> {
    const bridge = packetStreamToOpenStreamCallbacks(channel)
    this.activeAcceptedStreams.add(bridge.close)
    try {
      await new Promise<void>((resolve) => {
        acceptStreamFn((onMessage, onClose, onResolve, onReject) => {
          bridge.open(
            onMessage,
            (errMsg) => {
              resolve()
              onClose(errMsg)
            },
            onResolve,
            (errMsg) => {
              resolve()
              onReject(errMsg)
            },
          )
        })
      })
    } finally {
      this.activeAcceptedStreams.delete(bridge.close)
      bridge.close()
    }
  }

  private fail(err: unknown) {
    if (this.terminalError) {
      return
    }

    const terminalError = castToError(err, 'GoScript plugin process failed')
    this.terminalError = terminalError
    this.startup.reject(terminalError)
    this.done.reject(terminalError)
    console.warn(
      'plugin-goscript-cloudflare: GoScript plugin process exited',
      terminalError,
    )
    this.closeActiveAcceptedStreams(terminalError)
    this.installTerminalAcceptHandler(terminalError)
  }

  private closeActiveAcceptedStreams(err: Error) {
    for (const closeStream of this.activeAcceptedStreams) {
      closeStream(err)
    }
    this.activeAcceptedStreams.clear()
  }

  private installTerminalAcceptHandler(err: Error) {
    this.api.handleStreamCtr.set(async () => {
      throw err
    })
  }
}

export default function main(
  api: BackendAPI,
  loadPluginMain: GoScriptPluginMainLoader,
): BackendEntrypointLifecycle {
  const generation = new GoScriptCloudflarePluginGeneration(api)

  // Outgoing streams: map api.openStream() onto the callback contract the Go
  // side consumes through NewPushableOpenStream.
  globalScope.BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME = (
    onMessage,
    onClose,
    onResolve,
    onReject,
  ): void => {
    api.openStream().then(
      (channel) =>
        packetStreamToOpenStreamCallbacks(channel).open(
          onMessage,
          onClose,
          onResolve,
          onReject,
        ),
      (err) => {
        onReject(castToError(err).toString())
      },
    )
  }

  globalScope.BLDR_PLUGIN_MARK_READY = () => {
    generation.markReady()
  }

  globalScope.BLDR_PLUGIN_SET_ACCEPT_STREAM_WORKERS = (
    acceptStream?: (openStreamFunc: OpenStreamFunc) => void,
  ) => {
    generation.setAcceptStream(acceptStream)
  }

  return generation.start(api.startInfo, loadPluginMain)
}
