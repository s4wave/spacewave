// shared-worker.ts is the unified SharedWorker entry point for all plugin types.
//
// It parses URL parameters to determine:
// - s: script path (the plugin script to run)
// - t: worker type ('native' or 'quickjs', defaults to 'native')
//
// For native plugins, it imports the script directly and calls main(api, signal).
// For quickjs plugins, it imports quickjs-runner and calls main(api, signal, scriptPath).

import { HandleStreamCtr, HandleStreamFunc } from 'starpc'

import { PluginWorker } from '../runtime/plugin-worker.js'
import { BackendApiImpl } from '../../sdk/impl/backend-api.js'
import { PluginStartInfo } from '../../plugin/plugin.pb.js'

declare let self: SharedWorkerGlobalScope

// handleIncomingStreamCtr is the container for the plugin handle stream func.
const handleIncomingStreamCtr = new HandleStreamCtr()

// handleIncomingStream waits for a handler to be registered in handleIncomingStreamCtr.
const handleIncomingStream: HandleStreamFunc =
  handleIncomingStreamCtr.handleStreamFunc

// parseUrlParams parses the URL hash parameters.
// Format: #s=<scriptPath>&t=<workerType>
function parseUrlParams(): { scriptPath: string; workerType: string } {
  const url = new URL(self.location.href)
  const hash = url.hash

  if (!hash || !hash.startsWith('#')) {
    throw new Error('shared-worker: Missing hash parameters in URL.')
  }

  // Parse hash as query string (remove leading #)
  const params = new URLSearchParams(hash.substring(1))

  const scriptPath = params.get('s')
  if (!scriptPath) {
    throw new Error('shared-worker: Missing script path (s) in URL hash.')
  }

  const workerType = params.get('t') ?? 'native'

  return { scriptPath: decodeURIComponent(scriptPath), workerType }
}

// startPluginCallback is called when the first WebDocument connects and sends initialization data.
const startPluginCallback = async (startInfo: PluginStartInfo) => {
  const { scriptPath, workerType } = parseUrlParams()

  // Construct the WebRuntimeHost client.
  // This will call => WebRuntime (TypeScript) => rpcstream WebWorkerRpc => Go runtime
  const openStream = pluginWorker.webRuntimeClient.openStream.bind(
    pluginWorker.webRuntimeClient,
  )

  // Construct the backend api
  const backendAPI = new BackendApiImpl(
    startInfo,
    openStream,
    handleIncomingStreamCtr,
  )

  // Build abort signal
  const abortController = new AbortController()
  const abortSignal = abortController.signal

  if (workerType === 'quickjs') {
    // QuickJS plugin: import quickjs-runner and run the script in QuickJS VM
    console.log('shared-worker: starting QuickJS plugin:', scriptPath)
    const quickjsRunner =
      await import('../runtime/quickjs/plugin-host-quickjs.js')
    await quickjsRunner.default(backendAPI, abortSignal, scriptPath)
  } else {
    // Native plugin: dynamically import the script and call its main function
    console.log('shared-worker: starting native plugin:', scriptPath)
    const pluginModule = await import(scriptPath)
    if (typeof pluginModule.default !== 'function') {
      throw new Error(
        `shared-worker: Imported module "${scriptPath}" does not have a default export function.`,
      )
    }
    await pluginModule.default(backendAPI, abortSignal)
  }
}

// Initialize the PluginWorker.
const pluginWorker = new PluginWorker(
  self,
  startPluginCallback,
  handleIncomingStream,
)

// Note: the pluginWorker registers the onconnect callback on "self".
