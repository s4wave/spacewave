import { DependencyList, useEffect, useState } from 'react'
import { Client } from 'starpc'
import { useBldrContext } from './bldr-context.js'
import { WebDocument as BldrWebDocument } from '../bldr/web-document.js'
import { WebView as BldrWebView } from '../bldr/web-view.js'

// Destructor is the destructor type from React.
type Destructor = () => void

// useAbortController initializes an AbortController instance and returns it. A
// new AbortController is created whenever the dependencies change, and the old one
// is aborted. It also ensures that the AbortController is aborted when the
// component using the hook unmounts.
export function useAbortController(deps?: DependencyList): AbortController {
  const [abortController, setAbortController] = useState<AbortController>(
    () => new AbortController(),
  )

  // watch the dependencies / component being removed.
  useEffect(() => {
    let abort = abortController
    setAbortController((prevAbortController) => {
      if (!prevAbortController.signal.aborted) {
        return prevAbortController
      }

      return (abort = new AbortController())
    })

    return () => abort.abort()
  }, deps)

  return abortController
}

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

// useWebViewHostClientImpl builds a client implementation and abort signal for the web view host.
export function useWebViewHostClientImpl<T>(
  ctor: (c: Client) => T,
  effect: (
    impl: T,
    abortSignal: AbortSignal,
    webDocument: BldrWebDocument,
    webView: BldrWebView,
    client: Client,
  ) => void | Destructor,
  deps?: DependencyList,
) {
  useWebViewHostClient(
    (
      client: Client,
      abortSignal: AbortSignal,
      webDocument: BldrWebDocument,
      webView: BldrWebView,
    ) => {
      return effect(ctor(client), abortSignal, webDocument, webView, client)
    },
    deps,
  )
}
