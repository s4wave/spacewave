import {
  buildRpcStreamOpenStream,
  Client,
  HandleStreamCtr,
  OpenStreamFunc,
  RpcStreamPacket,
} from 'starpc'
import {
  ExecControllerRequest,
  ExecControllerResponse,
} from '@go/github.com/aperturerobotics/controllerbus/controller/exec/exec.pb.js'
import { BackendAPI } from '../plugin.js'
import { PluginHost, PluginHostClient } from '../../plugin/plugin_srpc.pb.js'
import {
  GetPluginInfoRequest,
  GetPluginInfoResponse,
  LoadPluginRequest,
  LoadPluginResponse,
  PluginStartInfo,
} from '../../plugin/plugin.pb.js'

// BackendAPIImpl implements the interface provided to the plugin module.
export class BackendApiImpl implements BackendAPI {
  // startInfo is the start information passed during initialization.
  public readonly startInfo: PluginStartInfo
  // openStream is the open stream func for client
  public readonly openStream: OpenStreamFunc
  // client is a connection to the Go WebRuntime via. WebWorkerRpc rpcstream.
  public readonly client: Client
  // pluginHost is the plugin host RPC service client.
  public readonly pluginHost: PluginHost
  // handleStreamCtr allows the plugin module to register a function
  // that will be called to handle incoming streams from the WebRuntime.
  public readonly handleStreamCtr: HandleStreamCtr

  // protos contains the protobuf objects used by the BackendAPI.
  public readonly protos = {
    PluginStartInfo: PluginStartInfo,

    GetPluginInfoRequest: GetPluginInfoRequest,
    GetPluginInfoResponse: GetPluginInfoResponse,

    ExecControllerRequest: ExecControllerRequest,
    ExecControllerResponse: ExecControllerResponse,

    LoadPluginRequest: LoadPluginRequest,
    LoadPluginResponse: LoadPluginResponse,

    RpcStreamPacket: RpcStreamPacket,
  } as const

  // HTTP prefix constants
  public readonly constants = {
    BLDR_HTTP_PREFIX: '/b/',
    PLUGIN_DIST_HTTP_PREFIX: '/b/pd/',
    PLUGIN_ASSETS_HTTP_PREFIX: '/b/pa/',
    PLUGIN_WEB_PKG_HTTP_PREFIX: '/b/pkg/',
    PLUGIN_HTTP_PREFIX: '/p/',
  } as const

  // HTTP path utility functions
  public readonly utils = {
    // pluginHttpPath adds the plugin http prefix to the given path.
    pluginHttpPath: (pluginId: string, ...httpPaths: string[]): string => {
      let result = this.constants.PLUGIN_HTTP_PREFIX + pluginId
      if (httpPaths.length === 0 || !httpPaths[0].startsWith('/')) {
        result += '/'
      }
      for (const httpPath of httpPaths) {
        result += httpPath
      }
      return result
    },

    // pluginDistHttpPath adds the plugin distribution file prefix to the given path.
    pluginDistHttpPath: (pluginId: string, httpPath: string): string => {
      let result = this.constants.PLUGIN_DIST_HTTP_PREFIX + pluginId
      if (!httpPath.startsWith('/')) {
        result += '/'
      }
      result += httpPath
      return result
    },

    // pluginAssetHttpPath adds the plugin asset file prefix to the given path.
    pluginAssetHttpPath: (pluginId: string, httpPath: string): string => {
      let result = this.constants.PLUGIN_ASSETS_HTTP_PREFIX + pluginId
      if (!httpPath.startsWith('/')) {
        result += '/'
      }
      result += httpPath
      return result
    },
  } as const

  constructor(
    startInfo: PluginStartInfo,
    openStream: OpenStreamFunc,
    handleStreamCtr: HandleStreamCtr,
  ) {
    this.startInfo = startInfo
    this.openStream = openStream
    this.client = new Client(openStream)
    this.handleStreamCtr = handleStreamCtr
    this.pluginHost = new PluginHostClient(this.client)
  }

  // buildPluginOpenStream builds an OpenStreamFunc for RPCs to a remote plugin.
  public buildPluginOpenStream(pluginID: string): OpenStreamFunc {
    return buildRpcStreamOpenStream(pluginID, this.pluginHost.PluginRpc)
  }
}
