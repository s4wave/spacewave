import React from 'react'

export interface IWebViewErrorBoundaryProps {
  // children are children elements
  children?: React.ReactNode
  // retryDelay is the delay in ms before retrying after a recoverable error.
  retryDelay?: number
}

interface IWebViewErrorBoundaryState {
  // caughtError is an error caught by the boundary.
  caughtError?: Error
}

// isRecoverableError checks if the error is a transient error that can be recovered by retrying.
function isRecoverableError(error: Error): boolean {
  const message = error.message || ''
  return (
    message.includes('Failed to fetch dynamically imported module') ||
    message.includes('error loading dynamically imported module') ||
    message.includes('Importing a module script failed')
  )
}

// WebViewErrorBoundary represents a portion of the page which the Go runtime controls.
// It is exposed as a WebViewErrorBoundary to the Go stack.
export class WebViewErrorBoundary extends React.Component<
  IWebViewErrorBoundaryProps,
  IWebViewErrorBoundaryState
> {
  private retryTimeout: ReturnType<typeof setTimeout> | null = null

  constructor(props: IWebViewErrorBoundaryProps) {
    super(props)
    this.state = {}
  }

  public static getDerivedStateFromError(error: Error) {
    return { caughtError: error }
  }

  public componentDidCatch(error: Error, errorInfo: React.ErrorInfo) {
    console.error('web-view error', error, errorInfo)

    if (isRecoverableError(error)) {
      console.log('web-view: recoverable error detected, will retry...')
      this.scheduleRetry()
    }
  }

  public componentWillUnmount() {
    if (this.retryTimeout) {
      clearTimeout(this.retryTimeout)
      this.retryTimeout = null
    }
  }

  private scheduleRetry() {
    if (this.retryTimeout) {
      return
    }
    const delay = this.props.retryDelay ?? 1000
    this.retryTimeout = setTimeout(() => {
      this.retryTimeout = null
      console.log('web-view: retrying after recoverable error...')
      this.setState({ caughtError: undefined })
    }, delay)
  }

  private handleReload = () => {
    window.location.reload()
  }

  public render() {
    const { caughtError } = this.state
    if (caughtError && !isRecoverableError(caughtError)) {
      return (
        <>
          Error: {caughtError.message}
          <br />
          <button onClick={this.handleReload}>Reload page</button>
        </>
      )
    }
    return <>{this.props.children}</>
  }
}
