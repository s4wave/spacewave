import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

import { useSessionSyncStatus } from '@s4wave/app/session/SessionSyncStatusContext.js'

import { toSessionSyncView } from '../status/session-sync.js'

interface SessionSyncLoadingCardProps {
  onRetry?: () => void
  className?: string
}

// SessionSyncLoadingCard renders the session sync status as a LoadingCard
// using the provider-owned sync watch. Use this anywhere the legacy "Loading
// session..." placeholder or bespoke sync-status card appeared.
export function SessionSyncLoadingCard({
  onRetry,
  className,
}: SessionSyncLoadingCardProps) {
  const status = useSessionSyncStatus()
  return (
    <LoadingCard
      view={toSessionSyncView(status, { onRetry })}
      className={className}
    />
  )
}
