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
  private readonly activeStreams = new Set<(err?: Error) => void>()
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
    const pluginStartInfoJsonB64 = btoa(
      String.fromCharCode(
        ...new TextEncoder().encode(PluginStartInfo.toJsonString(startInfo)),
      ),
    )
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
    const completed = Promise.withResolvers<void>()
    const close = (err?: Error) => {
      bridge.close(err)
      completed.resolve()
    }
    this.activeStreams.add(close)
    try {
      acceptStreamFn((onMessage, onClose, onResolve, onReject) => {
        bridge.open(
          onMessage,
          (errMsg) => {
            completed.resolve()
            onClose(errMsg)
          },
          onResolve,
          (errMsg) => {
            completed.resolve()
            onReject(errMsg)
          },
        )
      })
      await completed.promise
    } finally {
      this.activeStreams.delete(close)
      close()
    }
  }

  // openStream keeps outgoing channels within this generation's lifetime.
  public openStream: OpenStreamFunc = (
    onMessage,
    onClose,
    onResolve,
    onReject,
  ) => {
    if (this.terminalError) {
      onReject(this.terminalError.toString())
      return
    }
    void this.api.openStream().then(
      (channel) => {
        if (this.terminalError) {
          channel.abort(this.terminalError)
          onReject(this.terminalError.toString())
          return
        }
        const bridge = packetStreamToOpenStreamCallbacks(channel)
        this.activeStreams.add(bridge.close)
        bridge.open(
          onMessage,
          (err) => {
            this.activeStreams.delete(bridge.close)
            onClose(err)
          },
          onResolve,
          onReject,
        )
      },
      (err) => onReject(castToError(err).toString()),
    )
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
    this.closeActiveStreams(terminalError)
    this.installTerminalAcceptHandler(terminalError)
  }

  private closeActiveStreams(err: Error) {
    for (const closeStream of this.activeStreams) {
      closeStream(err)
    }
    this.activeStreams.clear()
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

  globalScope.BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME = (...args) => {
    generation.openStream(...args)
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
