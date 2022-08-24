import type { WebViewStatus } from '../document/document.pb.js'
import {
  WebViewHostClientImpl,
  SetRenderModeRequest,
  SetRenderModeResponse,
} from '../document/view/view.pb.js'
import type { Client, InvokeFn } from 'starpc'

// WebView implements the web-view with pluggable logic.
export interface WebView {
  // getWebViewUuid returns the web-view unique identifier.
  getWebViewUuid(): string
  // getPermanent checks if the web-view is permanent.
  getPermanent(): boolean
  // lookupMethod looks up the given WebView RPC method.
  lookupMethod?(serviceID: string, methodID: string): Promise<InvokeFn | null>
  // setRenderMode sets the render mode of the view.
  // if wait=true, should wait for op to complete before returning.
  setRenderMode(
    options: SetRenderModeRequest
  ): Promise<SetRenderModeResponse | void>
  // remove removes the web view, if !permanent.
  // returns if the web view was removed successfully.
  remove(): Promise<boolean>
}

// WebViewRegistration is returned when registering a web-view.
export interface WebViewRegistration {
  // rpcClient is the RPC client for the WebViewHost.
  readonly rpcClient: Client
  // webViewHost is the service attached to the rpcClient.
  readonly webViewHost: WebViewHostClientImpl
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
