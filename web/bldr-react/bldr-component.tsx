import React from 'react'
import { Client } from 'starpc'

import { BldrContext } from '@bldr/web/bldr-react'
import {
  WebDocument as BldrWebDocument,
  WebView as BldrWebView,
} from '@bldr/web/bldr'

// BldrComponent extends React.Component with the bldr context and an abort controller.
export class BldrComponent<P = {}, S = {}, SS = any> extends React.Component<
  P,
  S,
  SS
> {
  // context is the webDocument context
  declare context: React.ContextType<typeof BldrContext>
  static contextType = BldrContext
  // closeController is aborted when the component is unmounted.
  private closeController: AbortController

  constructor(props: P) {
    super(props)
    this.closeController = new AbortController()
  }

  // webDocument exposes the web document from context.
  get webDocument(): BldrWebDocument {
    return this.context!.webDocument!
  }

  // webView exposes the web view from context.
  get webView(): BldrWebView {
    return this.context!.webView!
  }

  // buildWebViewHostClient builds a client for the WebViewHost mux.
  public buildWebViewHostClient(): Client {
    return this.webDocument.buildWebViewHostClient(this.webView.getUuid())
  }

  public componentWillUnmount() {
    this.closeController.abort()
  }
}
