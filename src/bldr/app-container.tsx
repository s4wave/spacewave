import React from 'react'

import type { Runtime } from './runtime'

interface IAppContainerProps {
  // runtime is the app runtime
  runtime: Runtime
}

// RuntimeContext provides the app runtime to child components.
export const RuntimeContext = React.createContext<Runtime | null>(null)

// AppContainer is the root bldr application container.
// It provides the runtime to child components and adds debug info.
export class AppContainer extends React.Component<IAppContainerProps> {
  public render() {
    return (
      <div className="bldr-app">
        <RuntimeContext.Provider value={this.props.runtime}>
          {this.props.children}
        </RuntimeContext.Provider>
      </div>
    )
  }
}
