import React from 'react'
import { Client } from 'starpc'

import { AbortComponent } from './abort-component.js'
import { BldrContext } from './bldr-context.js'
import {
  WebDocument as BldrWebDocument,
  WebView as BldrWebView,
} from '../bldr/index.js'

// BldrComponent extends React.PureComponent with the bldr context and an abort controller.
export class BldrComponent<P = {}, S = {}, SS = any> extends AbortComponent<
  P,
  S,
  SS
> {
  // context is the webDocument context
  declare context: React.ContextType<typeof BldrContext>
  static contextType = BldrContext

  constructor(props: P) {
    super(props)
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
}
