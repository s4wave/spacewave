import { SessionLockMode } from '@s4wave/core/session/session.pb.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

import { toLockView } from '../status/lock.js'

interface SessionLockLoadingOverlayProps {
  mode?: SessionLockMode
  locked: boolean
  unlocking?: boolean
  errorMessage?: string
  onRetry?: () => void
  onCancel?: () => void
  className?: string
}

// SessionLockLoadingOverlay renders session lock state as a LoadingCard for
// the session-unlock overlay. Unlocked sessions render a 'synced' card.
export function SessionLockLoadingOverlay({
  mode,
  locked,
  unlocking,
  errorMessage,
  onRetry,
  onCancel,
  className,
}: SessionLockLoadingOverlayProps) {
  return (
    <LoadingCard
      view={toLockView({
        mode,
        locked,
        unlocking,
        errorMessage,
        onRetry,
        onCancel,
      })}
      className={className}
    />
  )
}
