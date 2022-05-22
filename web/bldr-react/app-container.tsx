import React from 'react'

import { Runtime } from '../bldr'

interface IAppContainerProps {
  // children contains optional child DOM of the app container
  children?: React.ReactNode
  // runtime is the external bldr runtime handle
  // if unset, constructs a default Runtime
  runtime?: Runtime
}

// RuntimeContext provides the app runtime to child components.
//
// default: mark as placeholder
export const RuntimeContext = React.createContext<Runtime | null>(null)

// AppContainer is the root bldr application container.
// It provides the runtime to child components and adds debug info.
export class AppContainer extends React.Component<IAppContainerProps> {
  private externalRuntime?: boolean
  private runtime: Runtime

  constructor(props: IAppContainerProps) {
    super(props)
    if (props.runtime) {
      this.externalRuntime = true
      this.runtime = props.runtime
    } else {
      this.runtime = new Runtime()
    }
  }

  // getRuntime gets and returns the runtime instance.
  public getRuntime(): Runtime {
    return this.runtime
  }

  public componentWillUnmount() {
    if (this.runtime && !this.externalRuntime) {
      this.runtime.dispose()
    }
  }

  public render() {
    return (
      <RuntimeContext.Provider value={this.runtime}>
        {this.props.children}
      </RuntimeContext.Provider>
    )
  }
}
