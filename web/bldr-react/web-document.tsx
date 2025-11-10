import React from 'react'

import { WebDocument as BldrWebDocument, WebDocumentOptions } from '@aptre/bldr'
import { BldrContext, IBldrContext } from './bldr-context.js'
import { DebugInfo } from './DebugInfo.js'

interface IWebDocumentProps {
  // children contains optional child DOM of the app container
  children?: React.ReactNode
  // webDocument is the external bldr WebDocument handle.
  // if unset, constructs a default WebDocument.
  webDocument?: BldrWebDocument
  // webDocumentOpts are options to pass to WebDocument.
  // ignored if webDocument is set
  webDocumentOpts?: WebDocumentOptions
  // showDebugInfo shows debug information about the WebDocument.
  showDebugInfo?: boolean
}

// WebDocument is the root bldr application container.
// It provides the runtime to child components and adds debug info.
export class WebDocument extends React.Component<IWebDocumentProps> {
  public readonly webDocument: BldrWebDocument
  private externalRuntime?: boolean
  private childContext: IBldrContext

  constructor(props: IWebDocumentProps) {
    super(props)
    if (props.webDocument) {
      this.externalRuntime = true
      this.webDocument = props.webDocument
    } else {
      let opts = props.webDocumentOpts || {}

      if (!opts.watchVisibility) {
        opts = {
          ...opts,
          watchVisibility: (cb: (hidden: boolean) => void) => {
            const handler = () => cb(document.hidden)
            handler()
            document.addEventListener('visibilitychange', handler)
            return () =>
              document.removeEventListener('visibilitychange', handler)
          },
        }
      }

      this.webDocument = new BldrWebDocument(opts)
    }
    this.state = {}
    this.childContext = { webDocument: this.webDocument }
  }

  public componentDidMount() {
    console.log(
      `WebDocument: mounted ${this.webDocument.webDocumentUuid} to WebRuntime ${this.webDocument.webRuntimeId}`,
    )
  }

  public componentWillUnmount() {
    if (!this.externalRuntime) {
      this.webDocument.close()
    }
  }

  public render() {
    return (
      <BldrContext.Provider value={this.childContext}>
        {this.props.showDebugInfo ?
          <DebugInfo>
            Runtime ID: {this.webDocument?.webRuntimeId}
            <br />
            Document ID: {this.webDocument?.webDocumentUuid}
            <br />
          </DebugInfo>
        : undefined}
        {this.props.children}
      </BldrContext.Provider>
    )
  }
}
