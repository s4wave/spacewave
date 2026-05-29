import runGoScriptPlugin from '../../../../web/runtime/goscript/plugin-goscript.js'
import { PluginStartInfo } from '../../../../plugin/plugin.pb.js'

declare const self: DedicatedWorkerGlobalScope

type GoPushableSink = {
  push: (message: Uint8Array) => void
  end: () => void
}

declare global {
  var BLDR_PLUGIN_START_INFO: string | undefined
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

export default async function main(api: Parameters<typeof runGoScriptPlugin>[0]) {
  await runGoScriptPlugin(api, pluginMain)
}

async function pluginMain(): Promise<void> {
  const startInfo = readStartInfo()
  self.postMessage({
    type: 'start-info',
    instanceId: startInfo.instanceId,
    pluginId: startInfo.pluginId,
    instanceKey: startInfo.instanceKey,
  })

  installAcceptStream()
  self.postMessage({ type: 'accept-ready' })

  await provePluginToHostStream()
  await new Promise<void>(() => {})
}

function readStartInfo(): PluginStartInfo {
  const encoded = globalThis.BLDR_PLUGIN_START_INFO
  if (!encoded) {
    throw new Error('missing BLDR_PLUGIN_START_INFO')
  }
  return PluginStartInfo.fromJsonString(atob(encoded))
}

function installAcceptStream(): void {
  const setAcceptStream = globalThis.BLDR_PLUGIN_SET_ACCEPT_STREAM
  if (!setAcceptStream) {
    throw new Error('missing BLDR_PLUGIN_SET_ACCEPT_STREAM')
  }
  setAcceptStream((port: MessagePort) => {
    port.onmessage = (ev: MessageEvent<Uint8Array>) => {
      const msg = ev.data
      if (msg?.[0] === 21) {
        port.postMessage(new Uint8Array([22]))
      }
    }
    port.start()
  })
}

async function provePluginToHostStream(): Promise<void> {
  const openStream = globalThis.BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME
  if (!openStream) {
    throw new Error('missing BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME')
  }

  const received = new Promise<void>((resolve, reject) => {
    let streamSink: GoPushableSink | undefined
    openStream(
      (message) => {
        if (message[0] === 12) {
          streamSink?.end()
          self.postMessage({ type: 'plugin-to-host-ok' })
          resolve()
          return
        }
        reject(new Error(`unexpected plugin-to-host response ${message[0]}`))
      },
      (errMsg) => {
        if (errMsg) {
          reject(new Error(errMsg))
        }
      },
      (sink) => {
        streamSink = sink
        sink.push(new Uint8Array([11]))
      },
      (errMsg) => reject(new Error(errMsg)),
    )
  })

  await received
}
