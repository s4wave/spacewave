import React, {
  Activity,
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from 'react'
import type {
  WebDocument as BldrWebDocument,
  WebView as BldrWebView,
  WebViewRegistration,
} from '@aptre/bldr'
import {
  randomId,
  RenderMode,
  SetRenderModeRequest,
  SetRenderModeResponse,
  SetHtmlLinksRequest,
  SetHtmlLinksResponse,
  HtmlLink,
} from '@aptre/bldr'

import { BldrContext, IBldrContext, useBldrContext } from './bldr-context.js'
import { FunctionComponentContainer } from './web-view-function.js'
import { ReactComponentContainer } from './web-view-react.js'
import { DebugInfo } from './DebugInfo.js'
import { useLatestRef } from './hooks.js'

// RemoveWebViewFunc is a function to remove a web view.
type RemoveWebViewFunc = (view: BldrWebView) => void

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
  // loading is rendered when the web view is not ready yet (loading).
  loading?: React.ReactNode
}

interface IWebViewHtmlLink {
  id: string
  link: HtmlLink
  // loaded indicates this stylesheet has fired its onload event.
  loaded?: boolean
}

interface IWebViewState {
  // ready indicates the registration is ready.
  ready?: boolean
  // renderMode is the current rendering mode.
  // defaults to NONE.
  renderMode?: RenderMode
  // refreshNonce is the number of times we have refreshed.
  refreshNonce: number
  // scriptPath is the script path to lazy load.
  scriptPath?: string
  // props is the binary props field.
  props?: Uint8Array
  // htmlLinks is the set of html link components.
  htmlLinks: IWebViewHtmlLink[]
  // reg is the web view registration
  reg?: WebViewRegistration
  // cssLoaded indicates all stylesheet links have finished loading.
  cssLoaded?: boolean
}

// canCloseWindow checks if window.close will (probably) work.
// https://stackoverflow.com/a/50593730
export function canCloseWindow() {
  return window.opener != null || window.history.length == 1
}

// useMemoManual signals to the React Compiler we want this to be manually memoized.
// see: https://github.com/facebook/react/issues/34172#issuecomment-3367496138
const useMemoManual = useMemo

interface IStylesheetLinkProps {
  id: string
  href: string
  onLoad: (id: string) => void
}

// StylesheetLink renders a stylesheet link and handles the load event.
// It checks if the stylesheet is already loaded from cache on mount.
function StylesheetLink({ id, href, onLoad }: IStylesheetLinkProps) {
  const ref = useRef<HTMLLinkElement>(null)

  useEffect(() => {
    const link = ref.current
    if (!link) {
      return
    }
    // Check if the stylesheet is already loaded (cached).
    // When a stylesheet is cached, onload may fire before we attach the handler.
    // The sheet property is set once the stylesheet is fully loaded and parsed.
    if (link.sheet) {
      onLoad(id)
    }
  }, [id, href, onLoad])

  const handleLoad = useCallback(() => {
    onLoad(id)
  }, [id, onLoad])

  return (
    <link
      ref={ref}
      href={href}
      rel="stylesheet"
      onLoad={handleLoad}
      onError={handleLoad}
    />
  )
}

// WebView represents a portion of the page which the Go webDocument controls.
// It is exposed as a WebView to the Go stack.
export const WebView: React.FC<IWebViewProps> = (props) => {
  const bldrContext = useBldrContext()
  const bldrWebDocument =
    props.webDocument || bldrContext?.webDocument || undefined

  // uuid is the web view uuid
  const uuid = useMemo(() => props.uuid || randomId(), [props.uuid])

  // parentUuid is the parent web view uuid
  const parentUuid = bldrContext?.webView?.getUuid() || undefined

  // parentUuidRef is the current parent uuid ref.
  const parentUuidRef = useLatestRef(parentUuid)

  // removable marks if this is removable or not
  const removable = useMemo(
    () =>
      // removable by callback
      !!props.onRemove ||
      // removable by window.close
      (!!props.isPermanent && canCloseWindow()),
    [props.onRemove, props.isPermanent],
  )

  // removableRef is a ref to the latest removable value
  const removableRef = useLatestRef(removable)

  // webViewState contains the current web view state.
  const [webViewState, setWebViewState] = useState<IWebViewState>(() => ({
    renderMode: RenderMode.RenderMode_NONE,
    htmlLinks: [],
    refreshNonce: 0,
  }))
  const [isComponentReady, setIsComponentReady] = useState(false)

  // TODO: hack: improve this
  /* eslint-disable */
  useEffect(() => {
    setIsComponentReady(false)
  }, [webViewState.scriptPath, webViewState.refreshNonce])
  /* eslint-enable */

  // onRemoveRef is a ref to the latest onRemove callback
  const onRemoveRef = useLatestRef(props.onRemove)

  const bldrWebViewRef = useRef<BldrWebView | null>(null)
  const bldrWebView: BldrWebView = useMemoManual(
    () => ({
      // getUuid returns the web-view unique identifier.
      getUuid(): string {
        return uuid
      },
      // getParentUuid returns the parent web-view unique identifier.
      // may be empty
      getParentUuid(): string | undefined {
        return parentUuidRef.current ?? undefined
      },
      // getPermanent checks if the web-view is permanent.
      getPermanent(): boolean {
        return !removableRef.current
      },
      // setRenderMode sets the render mode of the view.
      // if wait=true, should wait for op to complete before returning.
      async setRenderMode(
        options: SetRenderModeRequest,
      ): Promise<SetRenderModeResponse | void> {
        console.log(`WebView: set render mode: ${uuid}`, options)
        setWebViewState((prev) => ({
          ...prev,
          renderMode: options.renderMode,
          refreshNonce:
            options.refresh ? prev.refreshNonce + 1 : prev.refreshNonce,
          scriptPath:
            (options.renderMode !== RenderMode.RenderMode_NONE &&
              options.scriptPath?.trim()) ||
            undefined,
          props: options.props,
        }))
      },
      // setHtmlLinks sets or updates the list of HTML links.
      async setHtmlLinks(
        options: SetHtmlLinksRequest,
      ): Promise<SetHtmlLinksResponse | void> {
        console.log(`WebView: set html links: ${uuid}`, options)
        setWebViewState((prev) => {
          const links: IWebViewHtmlLink[] = [
            ...((!options.clear && prev.htmlLinks) || []),
          ]
          const removeLink = (id: string) => {
            for (let i = 0; i < links.length; i++) {
              if (links[i].id === id) {
                links.splice(i, 1)
                break
              }
            }
          }
          for (const removeID of options.remove ?? []) {
            removeLink(removeID)
          }
          if (options.setLinks) {
            for (const addID of Object.keys(options.setLinks)) {
              removeLink(addID)
              const link = options.setLinks[addID]
              if (link) {
                links.push({ id: addID, link, loaded: false })
              }
            }
          }
          // Check if all stylesheets are loaded (or if there are none)
          const hasUnloadedStylesheets = links.some(
            (l) => l.link.rel === 'stylesheet' && !l.loaded,
          )
          return {
            ...prev,
            htmlLinks: links,
            cssLoaded: !hasUnloadedStylesheets,
          }
        })
      },
      // resetView resets the web view to the initial state.
      async resetView(): Promise<void> {
        setWebViewState((prev) => {
          const next = { ...prev }
          next.refreshNonce++
          if (next.htmlLinks.length) {
            next.htmlLinks = []
          }
          if (next.renderMode != null) {
            next.renderMode = RenderMode.RenderMode_NONE
          }
          delete next.scriptPath
          return next
        })
      },
      // remove removes the web view, if !permanent.
      // returns if the web view was removed successfully.
      async remove(): Promise<boolean> {
        if (!removableRef.current) {
          return false
        }
        if (onRemoveRef.current && bldrWebViewRef.current) {
          onRemoveRef.current(bldrWebViewRef.current)
          return true
        }
        if (canCloseWindow()) {
          window.close()
          return true
        }
        return false
      },
    }),
    [uuid, removableRef, parentUuidRef, onRemoveRef, bldrWebViewRef],
  )

  useEffect(() => {
    bldrWebViewRef.current = bldrWebView
  }, [bldrWebView])

  const childContext = useMemo<IBldrContext>(
    () => ({
      webView: bldrWebView,
      webDocument: bldrWebDocument,
    }),
    [bldrWebView, bldrWebDocument],
  )

  // onLinkLoad marks a stylesheet as loaded and updates cssLoaded state.
  const onLinkLoad = useCallback((linkId: string) => {
    setWebViewState((prev) => {
      const links = prev.htmlLinks.map((link) =>
        link.id === linkId ? { ...link, loaded: true } : link,
      )
      const hasUnloadedStylesheets = links.some(
        (l) => l.link.rel === 'stylesheet' && !l.loaded,
      )
      return { ...prev, htmlLinks: links, cssLoaded: !hasUnloadedStylesheets }
    })
  }, [])

  /* eslint-disable */
  useLayoutEffect(() => {
    let nextReg: WebViewRegistration | null = null
    if (bldrWebDocument) {
      nextReg = bldrWebDocument.registerWebView(bldrWebView)
      setWebViewState((prev) => ({
        ...prev,
        ready: true,
        reg: nextReg ?? undefined,
      }))
      console.log(
        `WebView: mounted ${uuid} to document ${bldrWebDocument.webDocumentUuid} runtime ${bldrWebDocument.webRuntimeId}`,
      )
      // see: this.reg.webViewHost
    } else {
      console.error('Runtime is empty in WebView.')
    }

    return () => {
      if (nextReg) {
        nextReg.release()
        setWebViewState((prev) => ({ ...prev, ready: false, reg: undefined }))
      }
    }
  }, [uuid, bldrWebDocument, bldrWebView])
  /* eslint-enable */

  return (
    <BldrContext.Provider value={childContext}>
      {props.showDebugInfo ?
        <DebugInfo>
          WebView ID: {uuid} <br />
          {parentUuid ?
            <>
              Parent WebView ID: {parentUuid}
              <br />
            </>
          : undefined}
          Ready: {webViewState.ready ? 'true' : 'false'}
          <br />
          Render Mode: {webViewState.renderMode}
          <br />
          CSS Loaded: {webViewState.cssLoaded ? 'true' : 'false'}
          <br />
          {webViewState.scriptPath ?
            <>
              Script Path: {webViewState.scriptPath}
              <br />
            </>
          : undefined}
        </DebugInfo>
      : undefined}
      {/* Show loading while CSS is loading or component not ready */}
      {(
        (!webViewState.ready || !webViewState.cssLoaded || !isComponentReady) &&
        props.loading
      ) ?
        props.loading
      : null}
      {/* Render stylesheets immediately when ready with onload tracking */}
      {webViewState.ready ?
        webViewState.htmlLinks
          .filter(
            (ilink) => ilink.link.rel === 'stylesheet' && !!ilink.link.href,
          )
          .map((ilink) => (
            <StylesheetLink
              key={`${webViewState.refreshNonce} -> ${ilink.id}`}
              id={ilink.id}
              href={ilink.link.href!}
              onLoad={onLinkLoad}
            />
          ))
      : undefined}
      {/* Render non-stylesheet links immediately */}
      {webViewState.ready ?
        webViewState.htmlLinks
          .filter(
            (ilink) => ilink.link.rel !== 'stylesheet' && !!ilink.link.href,
          )
          .map((ilink) => (
            <link key={ilink.id} href={ilink.link.href} rel={ilink.link.rel} />
          ))
      : undefined}
      {/* Render component inside Activity - hidden until CSS loads */}
      {webViewState.ready &&
        webViewState.renderMode === RenderMode.RenderMode_REACT_COMPONENT &&
        !!webViewState.scriptPath && (
          <Activity mode={webViewState.cssLoaded ? 'visible' : 'hidden'}>
            <ReactComponentContainer
              key={`${webViewState.refreshNonce} -> ${webViewState.scriptPath}`}
              scriptPath={webViewState.scriptPath}
              componentProps={webViewState.props}
              onReady={() => setIsComponentReady(true)}
            />
          </Activity>
        )}
      {(
        webViewState.ready &&
        webViewState.renderMode === RenderMode.RenderMode_FUNCTION &&
        webViewState.scriptPath
      ) ?
        <Activity mode={webViewState.cssLoaded ? 'visible' : 'hidden'}>
          <FunctionComponentContainer
            key={`${webViewState.refreshNonce} -> ${webViewState.scriptPath}`}
            scriptPath={webViewState.scriptPath}
            componentProps={webViewState.props}
            onReady={() => setIsComponentReady(true)}
          />
        </Activity>
      : undefined}
    </BldrContext.Provider>
  )
}
