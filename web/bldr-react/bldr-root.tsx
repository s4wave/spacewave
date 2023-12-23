import React from 'react'
import type { WebDocument as BldrWebDocument } from '@aptre/bldr'
import { WebDocument } from './web-document.js'
import { WebView } from './web-view.js'

interface IBldrRootProps {
  // webDocument is the external bldr WebDocument handle
  // if unset, constructs a default WebDocument
  webDocument?: BldrWebDocument
  // children is the set of react children components.
  children?: JSX.Element | JSX.Element[]
}

// BldrRoot contains a WebDocument and a root web view.
export class BldrRoot extends React.Component<IBldrRootProps> {
  public render() {
    return (
      <WebDocument webDocument={this.props.webDocument || undefined}>
        <WebView isPermanent={true} />
        {this.props.children}
      </WebDocument>
    )
  }
}
