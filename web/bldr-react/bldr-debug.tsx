import React from 'react'

import { BldrContext } from './bldr-context.js'
import type { WebDocument as BldrWebDocument } from '../bldr/web-document.js'
import type { WebView as BldrWebView } from '../bldr/web-view.js'

// BldrDebug renders information about the BldrContext.
export class BldrDebug extends React.Component {
  // context is the webDocument context
  declare context: React.ContextType<typeof BldrContext>
  static contextType = BldrContext

  // webDocument gets and returns the WebDocument instance.
  get webDocument(): BldrWebDocument | undefined {
    return this.context?.webDocument
  }

  // webView gets and returns the WebView instance.
  get webView(): BldrWebView | undefined {
    return this.context?.webView
  }

  public render() {
    return (
      <div>
        Runtime ID: {this.webDocument?.webRuntimeId}
        <br />
        Document ID: {this.webDocument?.webDocumentUuid}
        <br />
        WebView ID: {this.webView?.getUuid()}
        <br />
        WebView Permanent: {this.webView?.getPermanent()}
        <br />
        Parent WebView ID: {this.webView?.getParentUuid()}
        <br />
      </div>
    )
  }
}
