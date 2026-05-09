import React from 'react'
import { BldrComponent } from './bldr-component.js'
import { DebugInfo } from './DebugInfo.js'

// BldrDebug renders information about the BldrContext.
export class BldrDebug extends BldrComponent {
  render() {
    const { webView, webDocument } = this
    const permanent = webView?.getPermanent()
    const uuid = webView?.getUuid()
    const parentUUID = webView?.getParentUuid()
    const webRuntimeId = webDocument?.webRuntimeId
    const webDocumentUuid = webDocument?.webDocumentUuid

    const infoElements: { key: string; element: React.ReactNode }[] = []
    if (webRuntimeId) {
      infoElements.push({
        key: 'runtime',
        element: <>Runtime ID: {webRuntimeId}</>,
      })
    }
    if (webDocumentUuid) {
      infoElements.push({
        key: 'document',
        element: <>Document ID: {webDocumentUuid}</>,
      })
    }
    if (uuid) {
      infoElements.push({ key: 'webview', element: <>WebView ID: {uuid}</> })
    }
    if (permanent) {
      infoElements.push({
        key: 'permanent',
        element: <>WebView Permanent: {permanent}</>,
      })
    }
    if (parentUUID) {
      infoElements.push({
        key: 'parent',
        element: <>Parent WebView ID: {parentUUID}</>,
      })
    }

    return (
      <DebugInfo>
        {infoElements.map((info, index) => (
          <React.Fragment key={info.key}>
            {info.element}
            {index < infoElements.length - 1 && <br />}
          </React.Fragment>
        ))}
      </DebugInfo>
    )
  }
}
