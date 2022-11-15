import React, { Suspense } from 'react'
import { Client } from 'starpc'

import { BldrContext, IBldrContext } from './bldr-context.js'
import type {
  WebDocument as BldrWebDocument,
  WebView as BldrWebView,
  WebViewRegistration,
} from '../bldr/index.js'
import { RenderMode, SetRenderModeRequest } from '../view/view.pb.js'
import { WebViewErrorBoundary } from './web-view-error-boundary.js'
import { randomId } from '../bldr/random-id.js'

// RemoveWebViewFunc is a function to remove a web view.
type RemoveWebViewFunc = (view: WebView) => void

// LoadedReactComponentType is the type the loaded component should implement.
type LoadedReactComponentType = React.ComponentType<unknown>

// LoadedReactComponent is a lazy-loaded React component.
type LoadedReactComponent = React.LazyExoticComponent<LoadedReactComponentType>

// LoadedScriptModule is the module loaded from a script.
// type LoadedScriptModule = { default: LoadedReactComponentType }

interface IWebViewProps {
  // uuid is the unique identifier for the web view.
  // if unset, a random id will be generated.
  uuid?: string
  // webDocument overrides the webDocument provided by context.
  webDocument?: BldrWebDocument
  // isPermanent indicates closing this web view will close the window.
  // calls window.close() when removing the web view.
  // if the window cannot be script-closed, marks view as permanent.
  isPermanent?: boolean
  // onRemove is a callback to remove the WebView, if possible.
  // if both isPermanent and onRemove are unset, marks the view as permanent
  onRemove?: RemoveWebViewFunc
}

interface IWebViewState {
  // ready indicates the registration is ready.
  ready?: boolean
  // renderMode is the current rendering mode.
  // defaults to NONE.
  renderMode?: RenderMode
  // reactComponent is the lazy-loaded contents for REACT_COMPONENT.
  reactComponent?: LoadedReactComponent
  // scriptPath is the script path to lazy load.
  scriptPath?: string
}

// WebView represents a portion of the page which the Go webDocument controls.
// It is exposed as a WebView to the Go stack.
export class WebView
  extends React.Component<IWebViewProps, IWebViewState>
  implements BldrWebView
{
  // context is the webDocument context
  declare context: React.ContextType<typeof BldrContext>
  static contextType = BldrContext

  // reg is the web-view registration
  private reg?: WebViewRegistration
  // uuid is the web view unique id.
  private readonly uuid: string
  // childContext is the context for child elements.
  private childContext: IBldrContext

  // loadedScript is the promise with the loaded script module.
  // private loadedScript?: Promise<LoadedScriptModule>
  // _loadedScript resolves the loadedScript promise.
  // private _loadedScript?: (err?: Error, val?: LoadedScriptModule) => void

  constructor(props: IWebViewProps) {
    super(props)
    this.state = { renderMode: RenderMode.RenderMode_NONE }
    this.uuid = props.uuid || randomId()
    this.childContext = {
      webDocument: this.getWebDocument(),
    }
  }

  // webViewHostClient returns the rpcClient for the WebViewHost
  //
  // expects the WebView to have been registered already.
  public get webViewHostClient(): Client {
    // assume the registration is complete
    if (!this.reg) {
      throw new Error('web view is not registered')
    }
    return this.reg.rpcClient
  }

  // getUuid returns the unique id of this web-view.
  public getUuid(): string {
    return this.uuid
  }

  // getParentUuid returns the unique id of this web-view.
  // may be empty
  public getParentUuid(): string | undefined {
    return this.context?.webView?.getUuid() || undefined
  }

  // getWebDocument returns the webDocument this is attached to.
  public getWebDocument(): BldrWebDocument | undefined {
    return this.props.webDocument || this.context?.webDocument || undefined
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
      (!!this.props.isPermanent && this.canCloseWindow())
    )
  }

  // setRenderMode sets the render mode of the view.
  // if wait=true, should wait for op to complete before returning.
  public async setRenderMode(options: SetRenderModeRequest): Promise<void> {
    const renderMode = options.renderMode
    let scriptPath = options.scriptPath?.trim() || ''
    let reactComponent: LoadedReactComponent | undefined = undefined
    let reactComponentPromise: Promise<{ default: unknown }> | undefined =
      undefined
    console.log('set render mode', options)
    switch (options.renderMode) {
      case RenderMode.RenderMode_REACT_COMPONENT:
        if (scriptPath) {
          ;[reactComponent, reactComponentPromise] =
            this._initReactComponent(scriptPath)
        }
        break
      default:
      case RenderMode.RenderMode_NONE:
        // make sure script is unset
        scriptPath = ''
        break
    }

    this.setState({ renderMode, reactComponent, scriptPath })
    if (!options.wait) {
      return
    }

    // wait for the component to load
    if (reactComponentPromise) {
      await reactComponentPromise
    }
    return
  }

  // remove removes the web view, if !permanent.
  // returns if the web view was removed successfully.
  public async remove(): Promise<boolean> {
    if (this.props.onRemove) {
      this.props.onRemove(this)
      return true
    }
    if (this.props.isPermanent && this.canCloseWindow()) {
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
    const webDocument = this.getWebDocument()
    if (webDocument) {
      this.reg = webDocument.registerWebView(this)
      this.setState({ ready: true })
      console.log(
        `WebView: mounted ${this.uuid} to document ${webDocument.webDocumentUuid} runtime ${webDocument.webRuntimeId}`
      )
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
    const parentWebViewId = this.getParentUuid()
    return (
      <BldrContext.Provider value={this.childContext}>
        <>
          WebView ID: {this.uuid} <br />
          {parentWebViewId ? (
            <>
              Parent WebView ID: {parentWebViewId}
              <br />
            </>
          ) : undefined}
          Render Mode: {this.state.renderMode} <br />
          {this.state.ready &&
          this.state.renderMode === 1 &&
          this.state.reactComponent ? (
            <WebViewErrorBoundary>
              <Suspense fallback={<div>Loading...</div>}>
                <this.state.reactComponent />
              </Suspense>
            </WebViewErrorBoundary>
          ) : undefined}
          <br />
        </>
      </BldrContext.Provider>
    )
  }

  // _initReactComponent initializes the promises to load a react component.
  private _initReactComponent(
    scriptPath: string
  ): [LoadedReactComponent, Promise<{ default: LoadedReactComponentType }>] {
    const loadPromise = import(scriptPath)
    return [
      React.lazy(async (): Promise<{ default: LoadedReactComponentType }> => {
        return loadPromise
      }),
      loadPromise,
    ]
  }
}
