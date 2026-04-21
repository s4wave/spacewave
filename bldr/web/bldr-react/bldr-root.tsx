import React from 'react'
import type {
  WebDocument as BldrWebDocument,
  WebDocumentOptions,
} from '@aptre/bldr'
import { WebDocument } from './web-document.js'
import { WebView } from './WebView.js'

export interface IBldrRootProps {
  // webDocument is the external bldr WebDocument handle
  // if unset, constructs a default WebDocument
  webDocument?: BldrWebDocument
  // children is the set of react children components.
  children?: React.ReactNode
  // webDocumentOpts are options to pass to WebDocument.
  // ignored if webDocument is set
  webDocumentOpts?: WebDocumentOptions
  // disableRootWebView disables the default root web view.
  disableRootWebView?: boolean
}

// BldrRoot contains a WebDocument and a root web view.
export const BldrRoot: React.FC<IBldrRootProps> = React.memo((props) => {
  return (
    <WebDocument
      webDocument={props.webDocument || undefined}
      webDocumentOpts={props.webDocumentOpts}
    >
      {!props.disableRootWebView ?
        <WebView isPermanent={true} />
      : undefined}
      {props.children}
    </WebDocument>
  )
})

BldrRoot.displayName = 'BldrRoot'
