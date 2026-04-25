import type { SharedObjectHealth } from '@s4wave/core/sobject/sobject.pb.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

import { toSharedObjectView } from '../status/shared-object.js'

interface SharedObjectLoadingCardProps {
  health: SharedObjectHealth | null
  onRetry?: () => void
  onCancel?: () => void
  className?: string
}

// SharedObjectLoadingCard renders a SharedObjectHealth snapshot as a
// LoadingCard. Accepts the pre-resolved health so the wrapper is cheap to
// drop into containers that already own the watch.
export function SharedObjectLoadingCard({
  health,
  onRetry,
  onCancel,
  className,
}: SharedObjectLoadingCardProps) {
  return (
    <LoadingCard
      view={toSharedObjectView(health, { onRetry, onCancel })}
      className={className}
    />
  )
}
