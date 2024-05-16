import {
  MessageStream,
  Mux,
  PacketStream,
  RpcStreamPacket,
  Server,
  createHandler,
  createMux,
} from 'starpc'
import { PluginWorker } from '../../runtime/plugin-worker.js'
import {
  Plugin as SRPCPlugin,
  PluginDefinition,
} from '../../../plugin/plugin_srpc.pb.js'
import { WebPluginBrowserHostClient } from './browser_srpc.pb.js'

// https://github.com/microsoft/TypeScript/issues/14877
declare let self: SharedWorkerGlobalScope | DedicatedWorkerGlobalScope
const global: SharedWorkerGlobalScope | DedicatedWorkerGlobalScope = self

// pluginWorker contains the common worker logic.
const pluginWorker = new PluginWorker(
  global,
  startPlugin,
  // Handle incoming RPC streams for the plugin.
  async (channel: PacketStream) => {
    plugin.rpcServer.handlePacketStream(channel)
  },
)

// webRuntimeClient manages the connection to the WebRuntime.
const webRuntimeClient = pluginWorker.webRuntimeClient

// webRuntimeRpcClient attempts to contact the WebRuntime over any of the WebDocument relays.
// NOTE: this connects to GetWebWorkerHost in web/runtime/remote.go
const webRuntimeRpcClient = webRuntimeClient.rpcClient

// webPluginBrowserHost is the web plugin browser host controller running on the web runtime host bus.
const webPluginBrowserHost = new WebPluginBrowserHostClient(webRuntimeRpcClient)

// Plugin implements the bldr.plugin.Plugin service.
class Plugin implements SRPCPlugin {
  // rpcServer is the rpc server for the mux with the bldr.plugin.Plugin service.
  public readonly rpcServer: Server
  // rpcMux is the rpc mux.
  public readonly rpcMux: Mux

  constructor() {
    const mux = createMux()
    mux.register(createHandler(PluginDefinition, this))
    this.rpcServer = new Server(mux.lookupMethodFunc)
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
    return webPluginBrowserHost.PluginRpc(request, abortSignal)
  }
}

const plugin = new Plugin()

function startPlugin() {
  console.log('start web plugin for browser')
}
