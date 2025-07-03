import {
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
} from '../../plugin/plugin.pb.js'

// BackendAPIImpl implements the interface provided to the plugin module.
export class BackendApiImpl implements BackendAPI {
  // startInfoB58 is the base58 encoded start information passed during initialization.
  public readonly startInfoB58: string
  // openStream is the open stream func for client
  public readonly openStream: OpenStreamFunc
  // client is a connection to the Go WebRuntime via. WebWorkerRpc rpcstream.
  public readonly client: Client
  // pluginHost is the plugin host RPC service client.
  readonly pluginHost: PluginHost
  // handleStreamCtr allows the plugin module to register a function
  // that will be called to handle incoming streams from the WebRuntime.
  public readonly handleStreamCtr: HandleStreamCtr

  // protos contains the protobuf objects used by the BackendAPI.
  public readonly protos = {
    GetPluginInfoRequest: GetPluginInfoRequest,
    GetPluginInfoResponse: GetPluginInfoResponse,

    ExecControllerRequest: ExecControllerRequest,
    ExecControllerResponse: ExecControllerResponse,

    LoadPluginRequest: LoadPluginRequest,
    LoadPluginResponse: LoadPluginResponse,

    RpcStreamPacket: RpcStreamPacket,
  } as const

  constructor(
    startInfoB58: string,
    openStream: OpenStreamFunc,
    handleStreamCtr: HandleStreamCtr,
  ) {
    this.startInfoB58 = startInfoB58
    this.openStream = openStream
    this.client = new Client(openStream)
    this.handleStreamCtr = handleStreamCtr
    this.pluginHost = new PluginHostClient(this.client)
  }
}
