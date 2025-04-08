import { HandleStreamCtr, Client } from 'starpc'
import { PluginHost } from '../plugin/plugin_srpc.pb.js'

// BackendAPI is the API exposed to Bldr plugin backends (running in a WebWorker).
//
// "backend" refers to the plugin code and "frontend" to bundles included in the assets filesystem.
export interface BackendAPI {
  // startInfoB58 is the base58 encoded start information passed during initialization.
  readonly startInfoB58: string

  // client is a connection to the Go WebRuntime
  // This will call => WebRuntime (TypeScript) => rpcstream WebWorkerRpc => Go runtime
  // RPC calls will be run against the plugin host bus with server id web-worker/{web-worker-id}
  readonly client: Client

  // pluginHost is the plugin host RPC service client.
  readonly pluginHost: PluginHost

  // handleStreamCtr allows the plugin module to register a function
  // that will be called to handle incoming streams from the WebRuntime.
  readonly handleStreamCtr: HandleStreamCtr
}

// BackendEntrypointFunc is the default function exported from a plugin backend entrypoint.
export type BackendEntrypointFunc = (api: BackendAPI) => Promise<void>
