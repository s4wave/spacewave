import { pipe } from 'it-pipe'
import {
  OpenStreamCtr,
  PacketStream,
} from 'starpc'
import { PluginWorker } from '../../runtime/plugin-worker.js'

// https://github.com/microsoft/TypeScript/issues/14877
declare let self: SharedWorkerGlobalScope | DedicatedWorkerGlobalScope
const global: SharedWorkerGlobalScope | DedicatedWorkerGlobalScope =
  self

// TODO set the shim config via global variable

// goOpenStreamCtr contains the function to open a stream with the Go program.
const goOpenStreamCtr = new OpenStreamCtr(undefined)

// pluginWorker contains the common worker logic.
new PluginWorker(
  global,
  startPlugin,
  // Hadle incoming RPC streams for the plugin.
  async (channel: PacketStream) => {
    const goStream = await goOpenStreamCtr.openStreamFunc()
    return pipe(channel, goStream, channel)
  },
)

function startPlugin(startInfoB58: string) {
  console.log("TODO start web plugin for browser", startInfoB58)
}
