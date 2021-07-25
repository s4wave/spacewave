import React from 'react'

import { AppContainer, Runtime } from './bldr'

interface IAppProps {
  // runtime is the external bldr runtime handle
  // if unset, constructs a default Runtime
  runtime?: Runtime
}

// App is the root entrypoint of the sandbox.
export class App extends React.Component<IAppProps> {
  private externalRuntime?: boolean
  private runtime: Runtime

  constructor(props: IAppProps) {
    super(props)
    if (props.runtime) {
      this.externalRuntime = true
      this.runtime = props.runtime
    } else {
      this.runtime = new Runtime()
    }
  }

  public componentWillUnmount() {
    if (this.runtime && !this.externalRuntime) {
      this.runtime.dispose()
    }
  }

  public render() {
    return (
      <div className="app">
        <AppContainer runtime={this.runtime}>
          <h2>Hello</h2>
        </AppContainer>
      </div>
    )
  }
}
