import React from 'react'
import type { Runtime } from '../bldr'
import { AppContainer } from './app-container'
import { WebView } from './web-view'
import { Demo } from './demo'

interface IAppProps {
  // runtime is the external bldr runtime handle
  // if unset, constructs a default Runtime
  runtime?: Runtime
  // children is the set of react children components.
  children?: JSX.Element | JSX.Element[]
}

// App contains a bldr runtime and a web view.
export class App extends React.Component<IAppProps> {
  public render() {
    return (
      <AppContainer runtime={this.props.runtime || undefined}>
        <WebView isWindow={true} />
        <Demo />
        {this.props.children}
      </AppContainer>
    )
  }
}
