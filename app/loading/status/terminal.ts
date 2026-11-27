import type { LoadingView } from '@s4wave/web/ui/loading/types.js'

export type TerminalStatus =
  | { kind: 'connecting' }
  | { kind: 'failed'; detail: string }
  | { kind: 'closed' }

// TerminalSessionKind declares what a terminal attaches to: the Spacewave CLI,
// a terminal object's remote prompt, or a virtual machine's console.
export type TerminalSessionKind = 'cli' | 'terminal' | 'console'

// terminalSessionCopy is the status copy of each terminal session kind.
const terminalSessionCopy: Record<
  TerminalSessionKind,
  {
    name: string
    connectingTitle: string
    connectingDetail: string
    closedDetail: string
  }
> = {
  cli: {
    name: 'CLI session',
    connectingTitle: 'Connecting to Spacewave CLI…',
    connectingDetail: 'Preparing a session-local command prompt.',
    closedDetail: 'The command prompt has ended.',
  },
  terminal: {
    name: 'Terminal session',
    connectingTitle: 'Connecting terminal…',
    connectingDetail: 'Connecting and waiting for the remote prompt.',
    closedDetail: 'The command prompt has ended.',
  },
  console: {
    name: 'Console',
    connectingTitle: 'Attaching to the console…',
    connectingDetail: 'Waiting for the virtual machine to write to it.',
    closedDetail: 'The console has ended.',
  },
}

interface TerminalStatusOptions {
  session: TerminalSessionKind
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
  const copy = terminalSessionCopy[options.session]
  if (status.kind === 'failed') {
    return {
      state: 'error',
      title: `${copy.name} failed`,
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
      title: `${copy.name} ended`,
      detail: copy.closedDetail,
      onRetry: options.onRetry,
      retryLabel: 'Restart',
    }
  }
  return {
    state: 'active',
    title: options.connectingTitle ?? copy.connectingTitle,
    detail: options.connectingDetail ?? copy.connectingDetail,
  }
}
