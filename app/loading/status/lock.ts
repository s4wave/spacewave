import { SessionLockMode } from '@s4wave/core/session/session.pb.js'
import type { LoadingView } from '@s4wave/web/ui/loading/types.js'

interface LockViewInput {
  mode?: SessionLockMode
  locked: boolean
  unlocking?: boolean
  errorMessage?: string
  onRetry?: () => void
  onCancel?: () => void
}

// toLockView describes the current session lock state. Unlocked sessions
// never render a loading surface; locked sessions show 'active' (awaiting PIN
// / auto-unlock) or 'error' when unlock fails.
export function toLockView(input: LockViewInput): LoadingView {
  const { mode, locked, unlocking, errorMessage, onRetry, onCancel } = input
  if (!locked) {
    return {
      state: 'synced',
      title: 'Session unlocked',
      detail: 'Session key is loaded.',
    }
  }
  if (errorMessage) {
    return {
      state: 'error',
      title: 'Session unlock failed',
      detail: 'Try again or cancel to go back.',
      error: errorMessage,
      onRetry,
      onCancel,
    }
  }
  if (unlocking) {
    return {
      state: 'active',
      title: 'Unlocking session',
      detail:
        mode === SessionLockMode.PIN_ENCRYPTED ?
          'Deriving session key from PIN.'
        : 'Deriving session key.',
      onCancel,
    }
  }
  return {
    state: 'loading',
    title: 'Session locked',
    detail:
      mode === SessionLockMode.PIN_ENCRYPTED ?
        'Enter PIN to unlock the session.'
      : 'Waiting for auto-unlock.',
    onCancel,
  }
}
