import React from 'react'
import type { WebDocument } from '../bldr'
import { AppContainer } from './app-container'
import { WebView } from './web-view'

interface IAppProps {
  // webDocument is the external bldr WebDocument handle
  // if unset, constructs a default WebDocument
  webDocument?: WebDocument
  // children is the set of react children components.
  children?: JSX.Element | JSX.Element[]
}

// App contains a bldr runtime and a web view.
export class App extends React.Component<IAppProps> {
  public render() {
    return (
      <AppContainer webDocument={this.props.webDocument || undefined}>
        <WebView isWindow={true} />
        {this.props.children}
      </AppContainer>
    )
  }
}
