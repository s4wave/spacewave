import { useLayoutEffect, useMemo } from 'react'
import { useBldrContext } from '@aptre/bldr-react'
import { createMux } from 'starpc'
import type { WebView } from '@aptre/bldr'

import {
  createDebugBridgeHandler,
  DebugBridgeWebViewID,
} from '@s4wave/sdk/debug/debug-service.js'

// DebugBridgeProvider registers a hidden WebView that exposes debug RPCs.
// Renders nothing. Must be inside AppShell (needs useBldrContext).
export function DebugBridgeProvider() {
  const ctx = useBldrContext()
  const doc = ctx?.webDocument

  const webView = useMemo<WebView | null>(() => {
    if (!doc) {
      return null
    }
    const mux = createMux()
    const handler = createDebugBridgeHandler(
      DebugBridgeWebViewID,
      doc.webDocumentUuid,
    )
    mux.register(handler)
    return {
      getUuid: () => DebugBridgeWebViewID,
      getParentUuid: () => undefined,
      getPermanent: () => true,
      lookupMethod: mux.lookupMethod,
      setRenderMode: () => Promise.resolve(),
      setHtmlLinks: () => Promise.resolve(),
      resetView: () => Promise.resolve(),
      remove: () => Promise.resolve(false),
    }
  }, [doc])

  useLayoutEffect(() => {
    if (!doc || !webView) {
      return
    }
    const reg = doc.registerWebView(webView)
    return () => {
      reg.release()
    }
  }, [doc, webView])

  return null
}
