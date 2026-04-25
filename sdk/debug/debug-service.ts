import { createHandler, type MessageStream } from 'starpc'

import {
  EvalJSRequest,
  EvalJSResponse,
  GetPageInfoResponse,
} from './debug.pb.js'
import {
  DebugBridgeServiceDefinition,
  DebugBridgeService,
} from './debug_srpc.pb.js'
import type { RpcStreamPacket } from '@go/github.com/aperturerobotics/starpc/rpcstream/rpcstream.pb.js'

// DebugBridgeWebViewID is the fixed WebView ID for the debug bridge.
export const DebugBridgeWebViewID = 'debug-bridge'

// DebugBridgeServiceImpl implements DebugBridgeService in the page context.
class DebugBridgeServiceImpl implements DebugBridgeService {
  private webViewId: string
  private documentId: string

  constructor(webViewId: string, documentId: string) {
    this.webViewId = webViewId
    this.documentId = documentId
  }

  async EvalJS(request: EvalJSRequest): Promise<EvalJSResponse> {
    const resp = EvalJSResponse.create({})
    const url = request.url
    if (!url) {
      resp.error = 'missing url'
      return resp
    }
    try {
      const mod = (await import(/* @vite-ignore */ url)) as {
        default?: unknown
      }
      const value = mod.default
      resp.result = JSON.stringify(value) ?? String(value)
    } catch (err) {
      resp.error = String(err)
    }
    return resp
  }

  GetPageInfo(): Promise<GetPageInfoResponse> {
    return Promise.resolve(
      GetPageInfoResponse.create({
        url: globalThis.location?.href ?? '',
        title: globalThis.document?.title ?? '',
        webViewId: this.webViewId,
        documentId: this.documentId,
      }),
    )
  }

  PluginRpc(
    _request: MessageStream<RpcStreamPacket>,
    _abortSignal?: AbortSignal,
  ): MessageStream<RpcStreamPacket> {
    throw new Error('PluginRpc not implemented in page context')
  }
}

// createDebugBridgeHandler creates a starpc Handler for the debug bridge service.
export function createDebugBridgeHandler(
  webViewId: string,
  documentId: string,
) {
  const impl = new DebugBridgeServiceImpl(webViewId, documentId)
  return createHandler(DebugBridgeServiceDefinition, impl)
}
