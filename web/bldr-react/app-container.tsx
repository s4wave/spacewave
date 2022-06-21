import React from 'react'

import { Runtime } from '../bldr'

interface IAppContainerProps {
  // children contains optional child DOM of the app container
  children?: React.ReactNode
  // runtime is the external bldr runtime handle
  // if unset, constructs a default Runtime
  runtime?: Runtime
}

interface IAppContainerState {
  // runtimeReady indicates the runtime is ready to use.
  runtimeReady: boolean
}

// RuntimeContext provides the app runtime to child components.
//
// default: mark as placeholder
export const RuntimeContext = React.createContext<Runtime | null>(null)

// Listener contains information about an event listener.
interface Listener {
  eventName: string
  cb: EventListenerOrEventListenerObject
}

// AppContainer is the root bldr application container.
// It provides the runtime to child components and adds debug info.
export class AppContainer extends React.Component<
  IAppContainerProps,
  IAppContainerState
> {
  private externalRuntime?: boolean
  private runtime: Runtime
  private listeners: Listener[] = []

  constructor(props: IAppContainerProps) {
    super(props)
    if (props.runtime) {
      this.externalRuntime = true
      this.runtime = props.runtime
    } else {
      this.runtime = new Runtime()
    }
    this.state = { runtimeReady: this.runtime.isReady }
  }

  // getRuntime gets and returns the runtime instance.
  public getRuntime(): Runtime {
    return this.runtime
  }

  public componentDidMount() {
    this.addRuntimeListener('ready', this.onRuntimeReady.bind(this))
    this.addRuntimeListener('unready', this.onRuntimeUnready.bind(this))
    if (this.runtime.isReady !== this.state.runtimeReady) {
      this.onRuntimeReady()
    }
  }

  public componentWillUnmount() {
    for (const listener of this.listeners) {
      this.runtime.removeEventListener(listener.eventName, listener.cb)
    }
    this.listeners.length = 0
    if (this.runtime && !this.externalRuntime) {
      this.runtime.close()
    }
  }

  // addRuntimeListener adds a runtime event listener.
  private addRuntimeListener(eventName: string, cb: () => void) {
    this.listeners.push({ eventName, cb })
    this.runtime.addEventListener(eventName, cb)
  }

  // onRuntimeReady is called when the runtime becomes ready.
  private onRuntimeReady() {
    this.setState({ runtimeReady: true })
  }

  // onRuntimeUnready is called when the runtime becomes not-ready.
  private onRuntimeUnready() {
    this.setState({ runtimeReady: false })
  }

  public render() {
    // TODO: implement loading spinner
    let appChildren: React.ReactNode | undefined
    if (this.state.runtimeReady) {
      appChildren = this.props.children
    }
    return (
      <RuntimeContext.Provider value={this.runtime}>
        {appChildren}
      </RuntimeContext.Provider>
    )
  }
}
