/* eslint-disable */
import { RpcStreamPacket } from '@go/github.com/aperturerobotics/starpc/rpcstream/rpcstream.pb.js'

export const protobufPackage = 'bldr.web.plugin.browser'

/**
 * WebPluginBrowserHost proxies requests to the web plugin to the plugin host.
 * accessed by web/plugin/browser/web-plugin-browser.ts
 */
export interface WebPluginBrowserHost {
  /**
   * PluginRpc handles an incoming RPC call from a remote plugin for the web plugin.
   * Id: remote plugin id
   */
  PluginRpc(
    request: AsyncIterable<RpcStreamPacket>,
    abortSignal?: AbortSignal,
  ): AsyncIterable<RpcStreamPacket>
}

export const WebPluginBrowserHostServiceName =
  'bldr.web.plugin.browser.WebPluginBrowserHost'
export class WebPluginBrowserHostClientImpl implements WebPluginBrowserHost {
  private readonly rpc: Rpc
  private readonly service: string
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || WebPluginBrowserHostServiceName
    this.rpc = rpc
    this.PluginRpc = this.PluginRpc.bind(this)
  }
  PluginRpc(
    request: AsyncIterable<RpcStreamPacket>,
    abortSignal?: AbortSignal,
  ): AsyncIterable<RpcStreamPacket> {
    const data = RpcStreamPacket.encodeTransform(request)
    const result = this.rpc.bidirectionalStreamingRequest(
      this.service,
      'PluginRpc',
      data,
      abortSignal || undefined,
    )
    return RpcStreamPacket.decodeTransform(result)
  }
}

/**
 * WebPluginBrowserHost proxies requests to the web plugin to the plugin host.
 * accessed by web/plugin/browser/web-plugin-browser.ts
 */
export type WebPluginBrowserHostDefinition =
  typeof WebPluginBrowserHostDefinition
export const WebPluginBrowserHostDefinition = {
  name: 'WebPluginBrowserHost',
  fullName: 'bldr.web.plugin.browser.WebPluginBrowserHost',
  methods: {
    /**
     * PluginRpc handles an incoming RPC call from a remote plugin for the web plugin.
     * Id: remote plugin id
     */
    pluginRpc: {
      name: 'PluginRpc',
      requestType: RpcStreamPacket,
      requestStream: true,
      responseType: RpcStreamPacket,
      responseStream: true,
      options: {},
    },
  },
} as const

interface Rpc {
  request(
    service: string,
    method: string,
    data: Uint8Array,
    abortSignal?: AbortSignal,
  ): Promise<Uint8Array>
  clientStreamingRequest(
    service: string,
    method: string,
    data: AsyncIterable<Uint8Array>,
    abortSignal?: AbortSignal,
  ): Promise<Uint8Array>
  serverStreamingRequest(
    service: string,
    method: string,
    data: Uint8Array,
    abortSignal?: AbortSignal,
  ): AsyncIterable<Uint8Array>
  bidirectionalStreamingRequest(
    service: string,
    method: string,
    data: AsyncIterable<Uint8Array>,
    abortSignal?: AbortSignal,
  ): AsyncIterable<Uint8Array>
}
