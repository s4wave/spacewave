import {
  DependencyList,
  RefObject,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react'
import { Client } from 'starpc'
import { useBldrContext } from './bldr-context.js'

import {
  WebDocument as BldrWebDocument,
  WebView as BldrWebView,
  RetryOpts,
  retryWithAbort,
} from '@aptre/bldr'

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

// useLatestRef returns a ref that contains the latest version of the value.
//
// changed is called when the value is changed from the initial value.
export function useLatestRef<T>(
  value: T,
  changed?: (value: T) => void,
): RefObject<T> {
  const ref = useRef(value)
  const changedRef = useRef(changed)
  useEffect(() => {
    changedRef.current = changed
  }, [changed])
  useEffect(() => {
    if (ref.current !== value) {
      ref.current = value
      if (changedRef.current) {
        changedRef.current(value)
      }
    }
  }, [value])
  return ref
}

// isUint8ArrayEqual checks if two Uint8Array are equal.
export function isUint8ArrayEqual(v1: Uint8Array | null, v2: Uint8Array | null) {
  // Check if they are equal by js reference.
  if (v1 === v2) {
    return true
  }

  // Check if both arrays are null
  if (v1 === null && v2 === null) {
    return true;
  }

  // Check if only one of the arrays is null
  if (v1 === null || v2 === null) {
    return false;
  }

  // Check if the arrays are the same length
  if (v1.length !== v2.length) {
    return false;
  }

  // Compare each element
  for (let i = 0; i < v1.length; i++) {
    if (v1[i] !== v2[i]) {
      return false;
    }
  }

  // Arrays are equal
  return true;
}

// useMemoUint8Array memoizes a uint8array.
export function useMemoUint8Array(value: Uint8Array | null): Uint8Array | null {
  const [memoValue, setMemoValue] = useState(() => value)
  const memoEquiv = isUint8ArrayEqual(value, memoValue)
  useEffect(() => {
    if (!memoEquiv) {
      setMemoValue(value)
    }
  }, [memoEquiv, value])
  return memoEquiv ? memoValue : value
}
