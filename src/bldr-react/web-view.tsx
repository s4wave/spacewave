import React from 'react'

import { Runtime } from '../bldr'
import { RuntimeContext } from './app-container'

interface IWebViewProps {
  // runtime overrides the runtime provided by context.
  runtime?: Runtime
}

// WebView represents a portion of the page which the Go runtime controls.
// It is exposed as a WebView to the Go stack.
export class WebView extends React.Component<IWebViewProps> {
  static contextType = RuntimeContext
  private runtime: Runtime

  constructor(props: IWebViewProps) {
    super(props)
    this.runtime = this.context
    if (this.props.runtime) {
      this.runtime = this.props.runtime
    }
  }

  public render() {
    return <div>Runtime: {this.runtime}</div>
  }
}
