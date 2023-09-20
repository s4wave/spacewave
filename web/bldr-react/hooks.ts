import { DependencyList, useEffect } from 'react'
import { Client } from 'starpc'
import { useBldrContext } from './bldr-context.js'
import { WebDocument as BldrWebDocument } from '../bldr/web-document.js'
import { WebView as BldrWebView } from '../bldr/web-view.js'

// Destructor is the destructor type from React.
type Destructor = () => void

// useWebViewHostClient builds a client and abort signal for the web view host.
export function useWebViewHostClient(
  effect: (
    client: Client,
    abortSignal: AbortSignal,
    webDocument: BldrWebDocument,
    webView: BldrWebView,
  ) => void | Destructor,
  deps?: DependencyList,
) {
  const bldrContext = useBldrContext()
  const webDocument = bldrContext?.webDocument
  const webView = bldrContext?.webView
  let effectDeps: DependencyList = [webDocument, webView]
  if (deps?.length) {
    effectDeps = [...effectDeps, ...deps]
  }
  useEffect(() => {
    if (!webDocument || !webView) {
      return
    }
    const client = webDocument.buildWebViewHostClient(webView.getUuid())
    const cancel = new AbortController()
    const destructor = effect(client, cancel.signal, webDocument, webView)
    return () => {
      cancel.abort()
      if (destructor) {
        destructor()
      }
    }
  }, effectDeps)
}
