import React from 'react'

import { BldrComponent } from './bldr-component.js'
import { DebugInfo } from './debug-info.js'

// BldrDebug renders information about the BldrContext.
export class BldrDebug extends BldrComponent {
  public render() {
    return (
      <DebugInfo>
        Runtime ID: {this.webDocument?.webRuntimeId}
        <br />
        Document ID: {this.webDocument?.webDocumentUuid}
        <br />
        WebView ID: {this.webView?.getUuid()}
        <br />
        WebView Permanent: {this.webView?.getPermanent()}
        <br />
        Parent WebView ID: {this.webView?.getParentUuid()}
      </DebugInfo>
    )
  }
}
