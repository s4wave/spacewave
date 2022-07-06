import React from 'react'

import type {
  Runtime,
  WebView as BldrWebView,
  WebViewRegistration,
} from '../bldr/index.js'
import { RuntimeContext } from './app-container.js'
import {
  WebViewRenderer,
  WebViewRendererDefinition,
  RenderMode,
  SetRenderModeRequest,
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

interface IWebViewState {
  // renderMode is the current rendering mode.
  // defaults to NONE.
  renderMode?: RenderMode
}

// WebView represents a portion of the page which the Go runtime controls.
// It is exposed as a WebView to the Go stack.
export class WebView
  extends React.Component<IWebViewProps, IWebViewState>
  implements BldrWebView
{
  // context is the runtime context
  declare context: React.ContextType<typeof RuntimeContext>
  static contextType = RuntimeContext
  // reg is the web-view registration
  private reg?: WebViewRegistration
  // webViewUuid is the randomly generated uuid.
  private readonly webViewUuid: string

  constructor(props: IWebViewProps) {
    super(props)
    this.state = { renderMode: RenderMode.RenderMode_NONE }
    this.webViewUuid = Math.random().toString(36).substring(2, 9)
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

  // setRenderMode sets the render mode of the view.
  // if wait=true, should wait for op to complete before returning.
  public async setRenderMode(options: SetRenderModeRequest): Promise<void> {
    this.setState({ renderMode: options.renderMode })

    switch (options.renderMode) {
      case RenderMode.RenderMode_REACT_COMPONENT:
      // TODO: load the script
      default:
      case RenderMode.RenderMode_NONE:
        // TODO: unload the script
        break
    }
    if (!options.wait) {
      return
    }

    // TODO: wait for script to be loaded
    return
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
