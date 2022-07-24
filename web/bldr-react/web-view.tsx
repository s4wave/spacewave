import React, { Suspense } from 'react'

import type {
  WebDocument,
  WebView as BldrWebView,
  WebViewRegistration,
} from '../bldr/index.js'
import { RenderMode, SetRenderModeRequest } from '../document/view/view.pb.js'
import { WebDocumentContext } from './app-container.js'
import { WebViewErrorBoundary } from './web-view-error-boundary.js'

// RemoveWebViewFunc is a function to remove a web view.
type RemoveWebViewFunc = (view: WebView) => void

// LoadedReactComponentType is the type the loaded component should implement.
type LoadedReactComponentType = React.ComponentType<unknown>

// LoadedReactComponent is a lazy-loaded React component.
type LoadedReactComponent = React.LazyExoticComponent<LoadedReactComponentType>

// LoadedScriptModule is the module loaded from a script.
// type LoadedScriptModule = { default: LoadedReactComponentType }

interface IWebViewProps {
  // webDocument overrides the webDocument provided by context.
  webDocument?: WebDocument
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
  // reactComponent is the lazy-loaded contents for REACT_COMPONENT.
  reactComponent?: LoadedReactComponent
  // scriptPath is the script path to lazy load.
  scriptPath?: string
}

// forceScriptPrefix forces the given prefix on any script path.
const forceScriptPrefix = '/b/'

// WebView represents a portion of the page which the Go webDocument controls.
// It is exposed as a WebView to the Go stack.
export class WebView
  extends React.Component<IWebViewProps, IWebViewState>
  implements BldrWebView
{
  // context is the webDocument context
  declare context: React.ContextType<typeof WebDocumentContext>
  static contextType = WebDocumentContext
  // reg is the web-view registration
  private reg?: WebViewRegistration
  // webViewUuid is the randomly generated uuid.
  private readonly webViewUuid: string

  // loadedScript is the promise with the loaded script module.
  // private loadedScript?: Promise<LoadedScriptModule>
  // _loadedScript resolves the loadedScript promise.
  // private _loadedScript?: (err?: Error, val?: LoadedScriptModule) => void

  constructor(props: IWebViewProps) {
    super(props)
    this.state = { renderMode: RenderMode.RenderMode_NONE }
    this.webViewUuid = Math.random().toString(36).substring(2, 9)
  }

  // getWebViewUuid should return a unique id for this web-view.
  public getWebViewUuid(): string {
    return this.webViewUuid
  }

  // getRuntime returns the webDocument this is attached to.
  public getRuntime(): WebDocument | undefined {
    return this.context || this.props.webDocument || undefined
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
    const renderMode = options.renderMode
    let scriptPath = options.scriptPath?.trim() || ''
    if (scriptPath && !scriptPath.startsWith(forceScriptPrefix)) {
      scriptPath = forceScriptPrefix + scriptPath
    }

    let reactComponent: LoadedReactComponent | undefined = undefined
    let reactComponentPromise: Promise<{ default: unknown }> | undefined =
      undefined
    switch (options.renderMode) {
      case RenderMode.RenderMode_REACT_COMPONENT:
        ;[reactComponent, reactComponentPromise] =
          this._initReactComponent(scriptPath)
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
    const webDocument = this.getRuntime()
    if (webDocument) {
      this.reg = webDocument.registerWebView(this)
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
    return (
      <div>
        <>
          WebView <br />
          ID: {this.webViewUuid} <br />
          Render Mode: {this.state.renderMode} <br />
          {this.state.renderMode === 1 && this.state.reactComponent ? (
            <WebViewErrorBoundary>
              <Suspense fallback={<div>Loading...</div>}>
                <this.state.reactComponent />
              </Suspense>
            </WebViewErrorBoundary>
          ) : undefined}
          <br />
        </>
      </div>
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
