import React from 'react'

import { WebDocument } from '../bldr'

interface IAppContainerProps {
  // children contains optional child DOM of the app container
  children?: React.ReactNode
  // webDocument is the external bldr WebDocument handle.
  // if unset, constructs a default WebDocument.
  webDocument?: WebDocument
}

// WebDocumentContext provides the WebDocument to child components.
//
// default: mark as placeholder
export const WebDocumentContext = React.createContext<WebDocument | null>(null)

// AppContainer is the root bldr application container.
// It provides the runtime to child components and adds debug info.
export class AppContainer extends React.Component<IAppContainerProps> {
  private externalRuntime?: boolean
  private webDocument: WebDocument

  constructor(props: IAppContainerProps) {
    super(props)
    if (props.webDocument) {
      this.externalRuntime = true
      this.webDocument = props.webDocument
    } else {
      this.webDocument = new WebDocument()
    }
    this.state = {}
  }

  // getWebDocument gets and returns the WebDocument instance.
  public getWebDocument(): WebDocument {
    return this.webDocument
  }

  public componentWillUnmount() {
    if (this.webDocument && !this.externalRuntime) {
      this.webDocument.close()
    }
  }

  public render() {
    return (
      <WebDocumentContext.Provider value={this.webDocument}>
        {this.props.children}
      </WebDocumentContext.Provider>
    )
  }
}
