import { DependencyList, useEffect, useMemo, useState } from 'react'
import { Client } from 'starpc'
import { useBldrContext } from './bldr-context.js'
import { WebDocument as BldrWebDocument } from '../bldr/web-document.js'
import { WebView as BldrWebView } from '../bldr/web-view.js'
import { RetryOpts, retryWithAbort } from '../bldr/retry.js'

// Destructor is the destructor type from React.
export type Destructor = () => void

// WebViewHostClientEffect is the callback function type for useWebViewHostClientImpl.
export type WebViewHostClientEffect = (
  client: Client,
  abortSignal: AbortSignal,
  webDocument: BldrWebDocument,
  webView: BldrWebView,
) => void | Destructor

// useWebViewHostClient builds a client and abort signal for the web view host.
export function useWebViewHostClient(
  effect: WebViewHostClientEffect,
  deps?: DependencyList,
) {
  const bldrContext = useBldrContext()
  const webDocument = bldrContext?.webDocument
  const webView = bldrContext?.webView
  useEffect(
    () => {
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
    },
    [webDocument, webView, ...(deps ?? [])], // eslint-disable-line
  )
}

// WebViewHostClientImplEffect is the callback function type for useWebViewHostClientImpl.
export type WebViewHostClientImplEffect<T> = (
  impl: T,
  abortSignal: AbortSignal,
  webDocument: BldrWebDocument,
  webView: BldrWebView,
  client: Client,
) => void | Destructor

// useWebViewHostClientImpl builds a client implementation and abort signal for the web view host.
export function useWebViewHostClientImpl<T>(
  ctor: (c: Client) => T,
  effect: WebViewHostClientImplEffect<T>,
  deps?: DependencyList,
) {
  useWebViewHostClient((client, abortSignal, webDocument, webView) => {
    return effect(ctor(client), abortSignal, webDocument, webView, client)
  }, deps)
}

// createWebViewHostClientImplEffect creates a useEffect function which calls useWebViewHostClientImpl.
export function createWebViewHostClientImplEffect<T>(
  ctor: (c: Client) => T,
): (effect: WebViewHostClientImplEffect<T>, deps?: DependencyList) => void {
  return (effect: WebViewHostClientImplEffect<T>, deps?: DependencyList) => {
    useWebViewHostClientImpl<T>(ctor, effect, deps)
  }
}

// createWebViewHostClientImplState creates a useState function for calling a rpc service impl.
export function createWebViewHostClientImplState<T>(
  ctor: (c: Client) => T,
): (deps?: DependencyList) => T | undefined {
  return (deps?: DependencyList) => {
    const [impl, setImpl] = useState<T | undefined>(undefined)
    useWebViewHostClient((client) => {
      setImpl(ctor(client))
      return () => setImpl(undefined)
    }, deps)
    return impl
  }
}

// useAbortSignal returns an AbortSignal which is canceled when the deps change.
export function useAbortSignal(deps: DependencyList = []): AbortSignal {
  const abortController = useMemo(
    () => new AbortController(),
    [...deps], // eslint-disable-line
  )
  useEffect(() => {
    return () => abortController.abort()
  }, [abortController])
  return abortController.signal
}

// useAbortSignalEffect wraps an effect with an abort signal.
export function useAbortSignalEffect(
  effect: (signal: AbortSignal) => void | (() => void),
  deps?: DependencyList,
) {
  useEffect(
    () => {
      const controller = new AbortController()
      const signal = controller.signal
      const teardown = effect(signal)

      return () => {
        controller.abort()
        if (teardown) {
          teardown()
        }
      }
    },
    [...(deps ?? [])], // eslint-disable-line
  )
}

// useRetryWithAbort calls the function with an abort signal and retries on error.
//
// will be aborted when the component is unmounted or deps change.
export function useRetryWithAbort(
  cb: (abortSignal: AbortSignal) => Promise<void>,
  opts?: RetryOpts,
  deps?: DependencyList,
) {
  useAbortSignalEffect((signal) => {
    retryWithAbort(signal, cb, opts)
  }, deps)
}
