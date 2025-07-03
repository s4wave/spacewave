import { HandleStreamCtr, HandleStreamFunc } from 'starpc'
import { PluginWorker } from '../runtime/plugin-worker.js'
import { BackendApiImpl } from '../../sdk/impl/backend-api.js'

declare let self: SharedWorkerGlobalScope

// handleIncomingStreamCtr is the container for the plugin handle stream func.
const handleIncomingStreamCtr = new HandleStreamCtr()

// handleIncomingStream waits for a handler to be registered in handleIncomingStreamCtr.
const handleIncomingStream: HandleStreamFunc =
  handleIncomingStreamCtr.handleStreamFunc

// Function passed to PluginWorker, called when the first WebDocument connects
// and sends initialization data.
const startPluginCallback = async (startInfoB58: string) => {
  // Parse the script path from the worker's URL hash.
  const url = new URL(self.location.href)
  let scriptPath: string | null = null
  if (url.hash && url.hash.startsWith('#s=')) {
    scriptPath = decodeURIComponent(url.hash.substring(3)) // Remove '#s=' prefix
  }
  if (!scriptPath) {
    throw new Error('shared-worker: Missing script hash parameter in URL.')
  }

  // Dynamically import the specified plugin module using the path from the 's' parameter.
  const pluginModule = await import(scriptPath)
  if (typeof pluginModule.default !== 'function') {
    throw new Error(
      `shared-worker: Imported module "${scriptPath}" does not have a default export function.`,
    )
  }

  // Construct the WebRuntimeHost client.
  // This will call => WebRuntime (TypeScript) => rpcstream WebWorkerRpc => Go runtime
  const openStream = pluginWorker.webRuntimeClient.openStream.bind(
    pluginWorker.webRuntimeClient,
  )

  // Construct the backend api
  const backendAPI = new BackendApiImpl(
    startInfoB58,
    openStream,
    handleIncomingStreamCtr,
  )

  // Call the imported module's main function, passing the API implementation.
  await pluginModule.default(backendAPI)
}

// Initialize the PluginWorker.
const pluginWorker = new PluginWorker(
  self,
  startPluginCallback,
  handleIncomingStream,
)

// Note: the pluginWorker registers the onconnect callback on "self".
