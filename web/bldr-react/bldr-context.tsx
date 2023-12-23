import React, { useContext } from 'react'
import type {
  WebDocument as BldrWebDocument,
  WebView as BldrWebView,
} from '@aptre/bldr'

// IBldrContext is the context containing the current WebDocument and WebView.
export interface IBldrContext {
  // webDocument is the parent web document.
  webDocument?: BldrWebDocument
  // webView is the parent WebView.
  webView?: BldrWebView
}

// BldrContext provides the IBldrContext to child components.
export const BldrContext = React.createContext<IBldrContext | null>(null)

// useBldrContext returns the current BldrContext.
export function useBldrContext() {
  return useContext(BldrContext)
}
