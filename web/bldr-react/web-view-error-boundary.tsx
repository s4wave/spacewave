import React from 'react'

export interface IWebViewErrorBoundaryProps {
  // children are children elements
  children?: React.ReactNode
}

interface IWebViewErrorBoundaryState {
  // caughtError is an error caught by the boundary.
  caughtError?: Error
}

// WebViewErrorBoundary represents a portion of the page which the Go runtime controls.
// It is exposed as a WebViewErrorBoundary to the Go stack.
export class WebViewErrorBoundary extends React.Component<
  IWebViewErrorBoundaryProps,
  IWebViewErrorBoundaryState
> {
  constructor(props: IWebViewErrorBoundaryProps) {
    super(props)
    this.state = {}
  }

  public static getDerivedStateFromError(error: Error) {
    return { caughtError: error }
  }

  public componentDidCatch(error: Error, errorInfo: React.ErrorInfo) {
    console.error('web-view error', error, errorInfo)
  }

  public render() {
    return (
      <>
        {this.state.caughtError ?
          <>
            Error: {this.state.caughtError.message}
            <br />
          </>
        : this.props.children}
      </>
    )
  }
}
