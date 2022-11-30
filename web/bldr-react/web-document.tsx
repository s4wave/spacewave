import React from 'react'

import { WebDocument as BldrWebDocument } from '../bldr/web-document.js'
import { BldrContext, IBldrContext } from './bldr-context.js'

interface IWebDocumentProps {
  // children contains optional child DOM of the app container
  children?: React.ReactNode
  // webDocument is the external bldr WebDocument handle.
  // if unset, constructs a default WebDocument.
  webDocument?: BldrWebDocument
}

// WebDocument is the root bldr application container.
// It provides the runtime to child components and adds debug info.
export class WebDocument extends React.Component<IWebDocumentProps> {
  private externalRuntime?: boolean
  private webDocument: BldrWebDocument
  private childContext: IBldrContext

  constructor(props: IWebDocumentProps) {
    super(props)
    if (props.webDocument) {
      this.externalRuntime = true
      this.webDocument = props.webDocument
    } else {
      this.webDocument = new BldrWebDocument()
    }
    this.state = {}
    this.childContext = { webDocument: this.webDocument }
  }

  public componentDidMount() {
    console.log(
      `WebDocument: mounted ${this.webDocument.webDocumentUuid} to WebRuntime ${this.webDocument.webRuntimeId}`
    )
  }

  // getWebDocument gets and returns the WebDocument instance.
  public getWebDocument(): BldrWebDocument {
    return this.webDocument
  }

  public componentWillUnmount() {
    if (this.webDocument && !this.externalRuntime) {
      this.webDocument.close()
    }
  }

  public render() {
    return (
      <BldrContext.Provider value={this.childContext}>
        <div>
          Runtime ID: {this.webDocument?.webRuntimeId}
          <br />
          Document ID: {this.webDocument?.webDocumentUuid}
          <br />
        </div>
        {this.props.children}
      </BldrContext.Provider>
    )
  }
}
