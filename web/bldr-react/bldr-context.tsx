import React from 'react'
import type { WebDocument as BldrWebDocument } from '../bldr/web-document.js'
import type { WebView as BldrWebView } from '../bldr/web-view.js'

// IBldrContext is the context containing the current WebDocument and WebView.
export interface IBldrContext {
  // webDocument is the parent web document.
  webDocument?: BldrWebDocument
  // webView is the parent WebView.
  webView?: BldrWebView
}

// BldrContext provides the IBldrContext to child components.
export const BldrContext = React.createContext<IBldrContext | null>(null)
