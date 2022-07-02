import React from 'react'
import { Server, Mux, createMux, createHandler } from 'starpc'

import type {
  Runtime,
  WebView as BldrWebView,
  WebViewRegistration,
} from '../bldr/index.js'
import { RuntimeContext } from './app-container.js'
import {
  EchoMsg,
  WebViewRenderer,
  WebViewRendererDefinition,
} from '../runtime/view/view.pb.js'

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
  implements BldrWebView, WebViewRenderer
{
  // context is the runtime context
  declare context: React.ContextType<typeof RuntimeContext>
  static contextType = RuntimeContext
  // reg is the web-view registration
  private reg?: WebViewRegistration
  // webViewUuid is the randomly generated uuid.
  private readonly webViewUuid: string
  // mux is the RPC mux for the server.
  private readonly mux: Mux
  // server is the RPC Server callable by the Go runtime.
  private readonly server: Server

  constructor(props: IWebViewProps) {
    super(props)
    this.webViewUuid = Math.random().toString(36).substring(2, 9)
    this.mux = createMux()
    const renderer: WebViewRenderer = this
    this.mux.register(createHandler(WebViewRendererDefinition, renderer))
    this.server = new Server(this.mux)
  }

  // TODO: remove
  public async Echo(request: EchoMsg): Promise<EchoMsg> {
    return request
  }

  // getWebViewUuid should return a unique id for this web-view.
  public getWebViewUuid(): string {
    return this.webViewUuid
  }

  // getRuntime returns the runtime this is attached to.
  public getRuntime(): Runtime | undefined {
    return this.context || this.props.runtime || undefined
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

  // getRpcServer returns the Server implementing the WebView rpc.
  public async getRpcServer(): Promise<Server> {
    return this.server
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

  public async componentDidMount() {
    const runtime = this.getRuntime()
    if (runtime) {
      this.reg = runtime.registerWebView(this)
      // see: this.reg.webViewHost
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
