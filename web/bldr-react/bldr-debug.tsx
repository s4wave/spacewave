import React from 'react'
import { BldrComponent } from './bldr-component.js'
import { DebugInfo } from './debug-info.js'

// BldrDebug renders information about the BldrContext.
export class BldrDebug extends BldrComponent {
  render() {
    const { webView, webDocument } = this
    const permanent = webView?.getPermanent()
    const uuid = webView?.getUuid()
    const parentUUID = webView?.getParentUuid()
    const webRuntimeId = webDocument?.webRuntimeId
    const webDocumentUuid = webDocument?.webDocumentUuid

    const infoElements = [
      webRuntimeId && <>Runtime ID: {webRuntimeId}</>,
      webDocumentUuid && <>Document ID: {webDocumentUuid}</>,
      uuid && <>WebView ID: {uuid}</>,
      permanent && <>WebView Permanent: {permanent}</>,
      parentUUID && <>Parent WebView ID: {parentUUID}</>,
    ].filter(Boolean)

    return (
      <DebugInfo>
        {infoElements.map((element, index) => (
          <React.Fragment key={index}>
            {element}
            {index < infoElements.length - 1 && <br />}
          </React.Fragment>
        ))}
      </DebugInfo>
    )
  }
}
