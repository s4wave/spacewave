import React from 'react'
import type {
  Runtime,
  WebView as BldrWebView,
  WebViewRegistration,
} from '../bldr'
import { RuntimeContext } from './app-container'

// RemoveWebViewFunc is a function to remove a web view.
type RemoveWebViewFunc = (view: WebView) => void

interface IWebViewProps {
  // runtime overrides the runtime provided by context.
  runtime?: Runtime
  // isWindow indicates closing this web view will close the window.
  // calls window.close() when removing the web view.
  // if the window cannot be script-closed, marks view as permanent.
  isWindow?: boolean
  // onRemove is a callback to remove the WebView, if possible.
  // if both isWindow and onRemove are unset, marks the view as permanent
  onRemove?: RemoveWebViewFunc
}

// WebView represents a portion of the page which the Go runtime controls.
// It is exposed as a WebView to the Go stack.
export class WebView
  extends React.Component<IWebViewProps>
  implements BldrWebView
{
  // context is the runtime context
  declare context: React.ContextType<typeof RuntimeContext>
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

  // getRuntime returns the runtime this is attached to.
  public getRuntime(): Runtime | undefined {
    const runtime = this.context || this.props.runtime
    return runtime && runtime.registerWebView ? runtime : undefined
  }

  // getPermanent checks if the web-view is permanent.
  public getPermanent(): boolean {
    return !this.getRemovable()
  }

  // getRemovable checks if it's possible to remove this web view.
  public getRemovable(): boolean {
    return (
      // removable by callback
      !!this.props.onRemove ||
      // removable by window.close
      (!!this.props.isWindow && this.canCloseWindow())
    )
  }

  // remove removes the web view, if !permanent.
  // returns if the web view was removed successfully.
  public async remove(): Promise<boolean> {
    if (this.props.onRemove) {
      this.props.onRemove(this)
      return true
    }
    if (this.props.isWindow && this.canCloseWindow()) {
      window.close()
      return true
    }
    return false
  }

  // canCloseWindow checks if window.close will (probably) work.
  // https://stackoverflow.com/a/50593730
  public canCloseWindow(): boolean {
    return window.opener != null || window.history.length == 1
  }

  public componentDidMount() {
    const runtime = this.getRuntime()
    if (runtime) {
      this.reg = runtime.registerWebView(this)
    } else {
      console.error('Runtime is empty in WebView.')
    }
  }

  public componentWillUnmount() {
    if (this.reg) {
      this.reg.release()
      delete this.reg
    }
  }

  public render() {
    return <span>WebView {this.webViewUuid}</span>
  }
}
