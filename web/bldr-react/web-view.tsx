import React, { Suspense } from 'react'
import { Client } from 'starpc'

import { BldrContext, IBldrContext } from './bldr-context.js'
import type {
  WebDocument as BldrWebDocument,
  WebView as BldrWebView,
  WebViewRegistration,
} from '../bldr/index.js'
import {
  RenderMode,
  SetRenderModeRequest,
  SetHtmlLinksRequest,
  HtmlLink,
} from '../view/view.pb.js'
import { WebViewErrorBoundary } from './web-view-error-boundary.js'
import { randomId } from '../bldr/random-id.js'
import { FunctionComponentContainer } from './web-view-function.js'

// RemoveWebViewFunc is a function to remove a web view.
type RemoveWebViewFunc = (view: WebView) => void

// LoadedReactComponentType is the type the loaded component should implement.
type LoadedReactComponentType = React.ComponentType<unknown>

// LoadedReactComponent is a lazy-loaded React component.
type LoadedReactComponent = React.LazyExoticComponent<LoadedReactComponentType>

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
  // showDebugInfo shows debug information about the WebView.
  showDebugInfo?: boolean
}

interface IWebViewHtmlLink {
  id: string
  link: HtmlLink
}

interface IWebViewState {
  // ready indicates the registration is ready.
  ready?: boolean
  // renderMode is the current rendering mode.
  // defaults to NONE.
  renderMode?: RenderMode

  // scriptPath is the script path to lazy load.
  scriptPath?: string

  // props is the binary props field.
  props?: Uint8Array

  // reactProps are props to pass to the component (an Object).
  reactProps?: unknown

  // reactComponent is the lazy-loaded contents for REACT_COMPONENT.
  reactComponent?: LoadedReactComponent

  // htmlLinks is the set of html link components.
  htmlLinks: IWebViewHtmlLink[]
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

  constructor(props: IWebViewProps) {
    super(props)
    this.state = { renderMode: RenderMode.RenderMode_NONE, htmlLinks: [] }
    this.uuid = props.uuid || randomId()
    this.childContext = { webView: this }
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
    let componentPromise: Promise<{ default: unknown }> | undefined = undefined
    const props = options.props
    let reactProps: unknown | undefined = undefined
    console.log('set render mode', options)
    switch (options.renderMode) {
      case RenderMode.RenderMode_REACT_COMPONENT:
        if (props.length) {
          const propsTxt = new TextDecoder().decode(props)
          try {
            reactProps = JSON.parse(propsTxt)
          } catch (err) {
            console.error('ignoring invalid json props', propsTxt)
            reactProps = undefined
          }
        }
        if (scriptPath) {
          ;[reactComponent, componentPromise] =
            this._initReactComponent(scriptPath)
        }
        break
      case RenderMode.RenderMode_FUNCTION:
        break
      default:
      case RenderMode.RenderMode_NONE:
        // make sure script is unset
        scriptPath = ''
        break
    }

    this.setState({
      renderMode,
      reactComponent,
      scriptPath,
      reactProps,
      props,
    })

    if (!options.wait) {
      return
    }

    // wait for the component to load
    if (componentPromise) {
      await componentPromise
    }

    return
  }

  // setHtmlLinks sets or updates the list of HTML links.
  public async setHtmlLinks(options: SetHtmlLinksRequest): Promise<void> {
    console.log('set html links', options)
    let links = (!options.clear && this.state.htmlLinks) || []
    const removeLink = (id: string) => {
      for (let i = 0; i < links.length; i++) {
        if (links[i].id === id) {
          links.splice(i, 1)
          break
        }
      }
    }
    for (const removeID of options.remove) {
      removeLink(removeID)
    }
    for (const addID of Object.keys(options.setLinks)) {
      removeLink(addID)
      links.push({
        id: addID,
        link: options.setLinks[addID],
      })
    }
    this.setState({ htmlLinks: links })
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
    this.childContext.webDocument = webDocument
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
          {this.props.showDebugInfo ? (
            <>
              WebView ID: {this.uuid} <br />
              {parentWebViewId ? (
                <>
                  Parent WebView ID: {parentWebViewId}
                  <br />
                </>
              ) : undefined}
              Ready: {this.state.ready ? 'true' : 'false'}
              <br />
              Render Mode: {this.state.renderMode}
              <br />
              {this.state.scriptPath ? (
                <>
                  Script Path: {this.state.scriptPath}
                  <br />
                </>
              ) : undefined}
            </>
          ) : undefined}
          {this.state.ready
            ? this.state.htmlLinks.map((ilink) => {
                return (
                  <link
                    key={ilink.id}
                    rel={ilink.link.rel}
                    href={ilink.link.href}
                  />
                )
              })
            : undefined}
          {this.state.ready &&
          this.state.renderMode === 1 &&
          this.state.reactComponent ? (
            <WebViewErrorBoundary>
              <Suspense fallback={<div>Loading...</div>}>
                <this.state.reactComponent
                  {...(typeof this.state.reactProps === 'object'
                    ? this.state.reactProps
                    : {})}
                />
              </Suspense>
            </WebViewErrorBoundary>
          ) : undefined}
          {this.state.ready &&
          this.state.renderMode === 2 &&
          this.state.scriptPath ? (
            <FunctionComponentContainer
              key={this.state.scriptPath}
              scriptPath={this.state.scriptPath}
              componentProps={this.state.props}
            />
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
