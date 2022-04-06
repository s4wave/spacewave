import React from 'react'
import type {
  Runtime,
  WebView as BldrWebView,
  WebViewRegistration,
} from '../bldr'
import { RuntimeContext } from './app-container'

interface IWebViewProps {
  // runtime overrides the runtime provided by context.
  runtime?: Runtime
}

// WebView represents a portion of the page which the Go runtime controls.
// It is exposed as a WebView to the Go stack.
export class WebView
  extends React.Component<IWebViewProps>
  implements BldrWebView
{
  // contextType is the context type to use for react context
  static contextType = RuntimeContext
  // reg is the web-view registration
  private reg?: WebViewRegistration
  // webViewUuid is the randomly generated uuid.
  private webViewUuid: string

  constructor(props: IWebViewProps) {
    super(props)
    this.webViewUuid = Math.random().toString(36).substr(2, 9)
  }

  // getWebViewUuid should return a unique id for this web-view.
  public getWebViewUuid(): string {
    return this.webViewUuid
  }

  public componentDidMount() {
    const runtime = this.context || this.props.runtime
    if (runtime) {
      this.reg = runtime.registerWebView(this)
    }
  }

  public componentWillUnmount() {
    if (this.reg) {
      this.reg.release()
      delete this.reg
    }
  }

  public render() {
    return <div>WebView {this.webViewUuid}</div>
  }
}
