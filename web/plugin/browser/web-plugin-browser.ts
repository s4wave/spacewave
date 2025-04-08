import {
  MessageStream,
  Mux,
  PacketStream,
  RpcStreamPacket,
  Server,
  createHandler,
  createMux,
} from 'starpc'
// Import types generated from protobuf definitions.
import type { BackendAPI } from '@aptre/bldr-sdk'
import {
  Plugin as SRPCPlugin,
  PluginDefinition,
} from '../../../plugin/plugin_srpc.pb.js'
import { WebPluginBrowserHostClient } from './browser_srpc.pb.js'

// Plugin implements the bldr.plugin.Plugin service.
class Plugin implements SRPCPlugin {
  // rpcServer is the rpc server for the mux with the bldr.plugin.Plugin service.
  public readonly rpcServer: Server
  // rpcMux is the rpc mux.
  public readonly rpcMux: Mux
  // webPluginBrowserHost is the client for the host service.
  private readonly webPluginBrowserHost: WebPluginBrowserHostClient

  constructor(webPluginBrowserHost: WebPluginBrowserHostClient) {
    this.webPluginBrowserHost = webPluginBrowserHost
    const mux = createMux()
    mux.register(createHandler(PluginDefinition, this))
    this.rpcServer = new Server(mux.lookupMethod)
    this.rpcMux = mux
  }

  /**
   * PluginRpc handles an RPC call from a remote plugin.
   * Component ID: remote plugin id
   */
  public PluginRpc(
    request: MessageStream<RpcStreamPacket>,
    abortSignal?: AbortSignal,
  ): MessageStream<RpcStreamPacket> {
    console.log('WebPluginBrowser: forwarding incoming stream to host')
    // Forward the RPC call to the host using the stored client.
    return this.webPluginBrowserHost.PluginRpc(request, abortSignal)
  }
}

/**
 * Main execution function for the browser plugin entrypoint.
 * Initializes the connection to the host and sets up RPC handling.
 */
export default async function main(backendAPI: BackendAPI) {
  console.log('Starting web plugin for browser...')

  // webRuntimeClient manages the connection to the WebRuntime.
  const webRuntimeClient = backendAPI.webRuntimeClient
  // webRuntimeRpcClient attempts to contact the WebRuntime over any of the WebDocument relays.
  // NOTE: this connects to GetWebWorkerHost in web/runtime/remote.go
  const webRuntimeRpcClient = webRuntimeClient.rpcClient

  // Initialize the client for the host service.
  const webPluginBrowserHost = new WebPluginBrowserHostClient(
    webRuntimeRpcClient,
  )

  // plugin is the instance of the Plugin service implementation.
  const plugin = new Plugin(webPluginBrowserHost)

  // Handle incoming RPC streams for the plugin by routing them to the plugin's RPC server.
  backendAPI.handleStreamCtr.set(async (channel: PacketStream) => {
    plugin.rpcServer.handlePacketStream(channel)
  })

  console.log('Web plugin for browser started.')
}
