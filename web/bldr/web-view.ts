import type { Client, InvokeFn } from 'starpc'

import type { WebViewStatus } from '../document/document.pb.js'
import {
  SetRenderModeRequest,
  SetRenderModeResponse,
  SetHtmlLinksRequest,
  SetHtmlLinksResponse,
} from '../view/view.pb.js'
import { WebViewHostClient } from '../view/view_srpc.pb.js'
import { Message } from '@aptre/protobuf-es-lite'

// WebView implements the web-view with pluggable logic.
export interface WebView {
  // getUuid returns the web-view unique identifier.
  getUuid(): string
  // getParentUuid returns the parent web-view unique identifier.
  // may be empty
  getParentUuid(): string | undefined
  // getPermanent checks if the web-view is permanent.
  getPermanent(): boolean
  // lookupMethod looks up the given WebView RPC method.
  lookupMethod?(serviceID: string, methodID: string): Promise<InvokeFn | null>
  // setRenderMode sets the render mode of the view.
  // if wait=true, should wait for op to complete before returning.
  setRenderMode(
    options: Message<SetRenderModeRequest>,
  ): Promise<Message<SetRenderModeResponse> | void>
  // setHtmlLinks sets or updates the list of HTML links.
  setHtmlLinks(
    options: SetHtmlLinksRequest,
  ): Promise<SetHtmlLinksResponse | void>
  // resetView resets the web view to the initial state.
  resetView(): Promise<void>
  // remove removes the web view, if !permanent.
  // returns if the web view was removed successfully.
  remove(): Promise<boolean>
}

// WebViewRegistration is returned when registering a web-view.
export interface WebViewRegistration {
  // rpcClient is the RPC client for the WebViewHost.
  readonly rpcClient: Client
  // webViewHost is the service attached to the rpcClient.
  readonly webViewHost: WebViewHostClient
  // release indicates that the web view has been shutdown.
  release(): void
}

// buildWebViewStatus builds a status message for a web view.
// if webView is null, indicates the view was deleted.
export function buildWebViewStatus(
  webViewId: string,
  webView?: WebView,
): WebViewStatus {
  return {
    id: webViewId,
    deleted: !webView,
    parentId: webView?.getParentUuid() || '',
    permanent: !!webView?.getPermanent(),
  }
}
