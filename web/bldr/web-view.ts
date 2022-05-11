import { WebViewStatus } from '../runtime/runtime'

// WebView implements the web-view with pluggable logic.
export interface WebView {
  // webViewUuid returns the web-view unique identifier.
  webViewUuid: string
  // isPermanent checks if the web view can be closed.
  isPermanent: boolean
}

// WebViewRegistration is returned when registering a web-view.
export interface WebViewRegistration {
  // release indicates that the web view has been shutdown.
  release(): void
}

// buildWebViewStatus builds a status message for a web view.
// if webView is null, indicates the view was deleted.
export function buildWebViewStatus(
  webViewId: string,
  webView: WebView
): WebViewStatus {
  return {
    id: webViewId,
    deleted: !webView,
    permanent: webView?.isPermanent || false,
  }
}
