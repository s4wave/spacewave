import type { WebRuntimeClient } from '../web/bldr/web-runtime-client.js'
import { HandleStreamCtr } from 'starpc'

// BackendAPI is the API exposed to Bldr plugin backends (running in a WebWorker).
//
// "backend" refers to the plugin code and "frontend" to bundles included in the assets filesystem.
export interface BackendAPI {
  // webRuntimeClient provides the connection to the WebRuntime host.
  readonly webRuntimeClient: WebRuntimeClient

  // handleStreamCtr allows the plugin module to register a function
  // that will be called to handle incoming streams from the WebRuntime.
  readonly handleStreamCtr: HandleStreamCtr

  // startInfoB58 is the base58 encoded start information passed during initialization.
  readonly startInfoB58: string
}

// BackendEntrypointFunc is the default function exported from a plugin backend entrypoint.
export type BackendEntrypointFunc = (api: BackendAPI) => Promise<void>
