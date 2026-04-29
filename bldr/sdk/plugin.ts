import {
  HandleStreamCtr,
  Client,
  OpenStreamFunc,
  RpcStreamPacket,
} from 'starpc'
import { MessageType } from '@aptre/protobuf-es-lite'
import {
  ExecControllerRequest,
  ExecControllerResponse,
} from '@go/github.com/aperturerobotics/controllerbus/controller/exec/exec.pb.js'
import { PluginHost } from '../plugin/plugin_srpc.pb.js'
import {
  GetPluginInfoRequest,
  GetPluginInfoResponse,
  LoadPluginRequest,
  LoadPluginResponse,
  PluginStartInfo,
} from '../plugin/plugin.pb.js'
import type { Client as ResourceClient } from './resource/client.js'

// BackendAPI is the API exposed to Bldr plugin backends (running in a WebWorker).
//
// "backend" refers to the plugin code and "frontend" to bundles included in the assets filesystem.
export interface BackendAPI {
  // startInfo is the start information passed during initialization.
  readonly startInfo: PluginStartInfo

  // openStream opens a stream to the remote rpc server, also used by Client.
  readonly openStream: OpenStreamFunc

  // client is a connection to the Go WebRuntime
  // This will call => WebRuntime (TypeScript) => rpcstream WebWorkerRpc => Go runtime
  // RPC calls will be run against the plugin host bus with server id web-worker/{web-worker-id}
  readonly client: Client

  // pluginHost is the plugin host RPC service client.
  readonly pluginHost: PluginHost

  // handleStreamCtr allows the plugin module to register a function
  // that will be called to handle incoming streams from the WebRuntime.
  readonly handleStreamCtr: HandleStreamCtr

  // resourceClient provides access to the plugin's resource tree.
  // Lazily connects on first accessRootResource() call.
  readonly resourceClient: ResourceClient

  // buildPluginOpenStream builds an OpenStreamFunc for RPCs to a remote plugin.
  buildPluginOpenStream(pluginID: string, instanceKey?: string): OpenStreamFunc

  // protos contains the protobuf objects used by the BackendAPI.
  readonly protos: {
    PluginStartInfo: MessageType<PluginStartInfo>

    GetPluginInfoRequest: MessageType<GetPluginInfoRequest>
    GetPluginInfoResponse: MessageType<GetPluginInfoResponse>

    ExecControllerRequest: MessageType<ExecControllerRequest>
    ExecControllerResponse: MessageType<ExecControllerResponse>

    LoadPluginRequest: MessageType<LoadPluginRequest>
    LoadPluginResponse: MessageType<LoadPluginResponse>

    RpcStreamPacket: MessageType<RpcStreamPacket>
  }

  // HTTP prefix constants
  readonly constants: {
    // BldrHttpPrefix is the route prefix for bldr-controlled URL space.
    // /b/
    readonly BLDR_HTTP_PREFIX: '/b/'

    // PluginDistHttpPrefix is the route prefix to use for plugin dist.
    // /b/pd/{plugin-id}
    readonly PLUGIN_DIST_HTTP_PREFIX: '/b/pd/'

    // PluginAssetsHttpPrefix is the route prefix to use for plugin assets.
    // /b/pa/{plugin-id}
    readonly PLUGIN_ASSETS_HTTP_PREFIX: '/b/pa/'

    // PluginWebPkgHttpPrefix is the public URL path prefix for web packages.
    // /b/pkg/
    readonly PLUGIN_WEB_PKG_HTTP_PREFIX: '/b/pkg/'

    // PluginHttpPrefix is the route prefix for plugin-controlled URL space.
    // /p/{pluginId}/
    readonly PLUGIN_HTTP_PREFIX: '/p/'
  }

  // HTTP path utility functions
  readonly utils: {
    // pluginHttpPath adds the plugin http prefix to the given path.
    pluginHttpPath(pluginId: string, ...httpPaths: string[]): string

    // pluginDistHttpPath adds the plugin distribution file prefix to the given path.
    pluginDistHttpPath(pluginId: string, httpPath: string): string

    // pluginAssetHttpPath adds the plugin asset file prefix to the given path.
    pluginAssetHttpPath(pluginId: string, httpPath: string): string
  }
}

// BackendEntrypointFunc is the default function exported from a plugin backend entrypoint.
export type BackendEntrypointFunc = (
  api: BackendAPI,
  abortSignal: AbortSignal,
) => Promise<void>
