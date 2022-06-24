import type { WebViewStatus } from '../runtime/runtime.pb.js'
import type { Server, Client } from 'starpc'

// WebView implements the web-view with pluggable logic.
export interface WebView {
  // getWebViewUuid returns the web-view unique identifier.
  getWebViewUuid(): string
  // getPermanent checks if the web-view is permanent.
  getPermanent(): boolean
  // getRpcServer returns the Server implementing the WebView rpc.
  getRpcServer(): Promise<Server>
  // remove removes the web view, if !permanent.
  // returns if the web view was removed successfully.
  remove(): Promise<boolean>
}

// WebViewRegistration is returned when registering a web-view.
export interface WebViewRegistration {
  // rpcClient is the RPC client for the WebView.
  readonly rpcClient: Client
  // release indicates that the web view has been shutdown.
  release(): void
}

// buildWebViewStatus builds a status message for a web view.
// if webView is null, indicates the view was deleted.
export function buildWebViewStatus(
  webViewId: string,
  webView?: WebView
): WebViewStatus {
  return {
    id: webViewId,
    deleted: !webView,
    permanent: !!webView?.getPermanent(),
  }
}
