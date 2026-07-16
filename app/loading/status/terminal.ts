import type { LoadingView } from '@s4wave/web/ui/loading/types.js'

export type TerminalStatus =
  | { kind: 'connecting' }
  | { kind: 'failed'; detail: string }
  | { kind: 'closed' }

interface TerminalStatusOptions {
  cliSession: boolean
  connectingTitle?: string
  connectingDetail?: string
  onRetry?: () => void
  onBackToSettings?: () => void
}

// terminalStatusToLoadingView maps terminal connection states onto the shared
// loading surface contract while preserving terminal-specific recovery actions.
export function terminalStatusToLoadingView(
  status: TerminalStatus,
  options: TerminalStatusOptions,
): LoadingView {
  const sessionName = options.cliSession ? 'CLI session' : 'Terminal session'
  if (status.kind === 'failed') {
    return {
      state: 'error',
      title: `${sessionName} failed`,
      detail: status.detail,
      onRetry: options.onRetry,
      retryLabel: 'Retry',
      onCancel: options.onBackToSettings,
      cancelLabel: 'Back to Settings',
    }
  }
  if (status.kind === 'closed') {
    return {
      state: 'error',
      title: `${sessionName} ended`,
      detail: 'The command prompt has ended.',
      onRetry: options.onRetry,
      retryLabel: 'Restart',
    }
  }
  return {
    state: 'active',
    title:
      options.connectingTitle ??
      (options.cliSession
        ? 'Connecting to Spacewave CLI…'
        : 'Connecting terminal…'),
    detail:
      options.connectingDetail ??
      (options.cliSession
        ? 'Preparing a session-local command prompt.'
        : 'Connecting and waiting for the remote prompt.'),
  }
}
