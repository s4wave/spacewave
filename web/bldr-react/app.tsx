import React from 'react'
import type { WebDocument as BldrWebDocument } from '../bldr'
import { WebDocument } from './web-document'
import { WebView } from './web-view'

interface IAppProps {
  // webDocument is the external bldr WebDocument handle
  // if unset, constructs a default WebDocument
  webDocument?: BldrWebDocument
  // children is the set of react children components.
  children?: JSX.Element | JSX.Element[]
}

// App contains a WebDocument and a root web view.
export class App extends React.Component<IAppProps> {
  public render() {
    return (
      <WebDocument webDocument={this.props.webDocument || undefined}>
        <WebView isPermanent={true} />
        {this.props.children}
      </WebDocument>
    )
  }
}
