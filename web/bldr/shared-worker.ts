import { Client, HandleStreamCtr, HandleStreamFunc } from 'starpc'
import { PluginWorker } from '../runtime/plugin-worker.js'
import { BackendAPI } from '../../sdk/plugin.js'
import { PluginHost, PluginHostClient } from '../../plugin/plugin_srpc.pb.js'

declare let self: SharedWorkerGlobalScope

// BackendAPI interface provided to the plugin module.
class BackendApiImpl implements BackendAPI {
  // startInfoB58 is the base58 encoded start information passed during initialization.
  public readonly startInfoB58: string
  // client is a connection to the Go WebRuntime via. WebWorkerRpc rpcstream.
  public readonly client: Client
  // pluginHost is the plugin host RPC service client.
  readonly pluginHost: PluginHost
  // handleStreamCtr allows the plugin module to register a function
  // that will be called to handle incoming streams from the WebRuntime.
  public readonly handleStreamCtr: HandleStreamCtr

  constructor(
    startInfoB58: string,
    client: Client,
    handleStreamCtr: HandleStreamCtr,
  ) {
    this.startInfoB58 = startInfoB58
    this.client = client
    this.handleStreamCtr = handleStreamCtr
    this.pluginHost = new PluginHostClient(client)
  }
}

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
  const webWorkerHostClient = new Client(
    pluginWorker.webRuntimeClient.openStream.bind(pluginWorker.webRuntimeClient),
  )

  // Construct the backend api
  const backendAPI = new BackendApiImpl(
    startInfoB58,
    webWorkerHostClient,
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
