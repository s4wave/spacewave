import React from 'react'

export interface IWebViewErrorBoundaryProps {
  // children are children elements
  children?: React.ReactNode
}

interface IWebViewErrorBoundaryState {
  // caughtError is an error caught by the boundary.
  caughtError?: Error
  // retryAttempt is the current retry attempt number (0 = first failure).
  retryAttempt: number
  // countdown is the remaining seconds before auto-retry.
  countdown: number
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

// backoffSeconds returns the backoff delay in seconds for the given attempt.
// 2s, 4s, 8s, 16s, 16s, ...
function backoffSeconds(attempt: number): number {
  return Math.min(2 ** (attempt + 1), 16)
}

// truncateModuleUrl extracts a readable module name from the full URL.
function truncateModuleUrl(message: string): string {
  const match = message.match(
    /(?:Failed to fetch|error loading|Importing a module script failed).*?(\/b\/.*\.mjs)/i,
  )
  if (match) {
    return match[1]
  }
  return message
}

const containerStyle: React.CSSProperties = {
  display: 'flex',
  flexDirection: 'column',
  alignItems: 'center',
  justifyContent: 'center',
  padding: '24px',
  gap: '12px',
  fontFamily: 'system-ui, sans-serif',
  fontSize: '14px',
  color: '#a0a0a0',
}

const errorTextStyle: React.CSSProperties = {
  fontFamily: 'monospace',
  fontSize: '12px',
  color: '#808080',
  wordBreak: 'break-all',
  textAlign: 'center',
  maxWidth: '480px',
}

const buttonRowStyle: React.CSSProperties = {
  display: 'flex',
  gap: '8px',
}

const buttonStyle: React.CSSProperties = {
  padding: '6px 16px',
  border: '1px solid #404040',
  borderRadius: '4px',
  background: 'transparent',
  color: '#c0c0c0',
  cursor: 'pointer',
  fontSize: '13px',
}

// WebViewErrorBoundary catches errors from dynamically imported plugin modules.
// Recoverable errors (stale module hash, network glitch) trigger exponential
// backoff retries with a visible countdown and user controls.
export class WebViewErrorBoundary extends React.Component<
  IWebViewErrorBoundaryProps,
  IWebViewErrorBoundaryState
> {
  private countdownInterval: ReturnType<typeof setInterval> | null = null

  constructor(props: IWebViewErrorBoundaryProps) {
    super(props)
    this.state = { retryAttempt: 0, countdown: 0 }
  }

  public static getDerivedStateFromError(error: Error) {
    return { caughtError: error }
  }

  public componentDidCatch(error: Error, errorInfo: React.ErrorInfo) {
    console.error('web-view error', error, errorInfo)

    if (isRecoverableError(error)) {
      const delay = backoffSeconds(this.state.retryAttempt)
      console.log(
        `web-view: recoverable error detected, retrying in ${delay}s (attempt ${this.state.retryAttempt + 1})`,
      )
      this.startCountdown(delay)
    }
  }

  public componentWillUnmount() {
    this.clearCountdown()
  }

  private clearCountdown() {
    if (this.countdownInterval) {
      clearInterval(this.countdownInterval)
      this.countdownInterval = null
    }
  }

  private startCountdown(seconds: number) {
    this.clearCountdown()
    this.setState({ countdown: seconds })
    this.countdownInterval = setInterval(() => {
      this.setState((prev) => {
        const next = prev.countdown - 1
        if (next <= 0) {
          this.clearCountdown()
          return {
            ...prev,
            countdown: 0,
            caughtError: undefined,
            retryAttempt: prev.retryAttempt + 1,
          }
        }
        return { ...prev, countdown: next }
      })
    }, 1000)
  }

  private handleRetryNow = () => {
    this.clearCountdown()
    this.setState((prev) => ({
      caughtError: undefined,
      countdown: 0,
      retryAttempt: prev.retryAttempt + 1,
    }))
  }

  private handleReload = () => {
    window.location.reload()
  }

  public render() {
    const { caughtError, countdown } = this.state
    if (!caughtError) {
      return <>{this.props.children}</>
    }

    const recoverable = isRecoverableError(caughtError)
    const modulePath = truncateModuleUrl(caughtError.message)

    return (
      <div style={containerStyle}>
        <div>Failed to load module</div>
        <div style={errorTextStyle}>{modulePath}</div>
        {recoverable && countdown > 0 && (
          <div>Retrying in {countdown}s...</div>
        )}
        <div style={buttonRowStyle}>
          {recoverable && (
            <button style={buttonStyle} onClick={this.handleRetryNow}>
              Retry now
            </button>
          )}
          <button style={buttonStyle} onClick={this.handleReload}>
            Reload page
          </button>
        </div>
      </div>
    )
  }
}
