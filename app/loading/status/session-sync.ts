import type { LoadingView } from '@s4wave/web/ui/loading/types.js'

import type { SessionSyncStatusView } from '@s4wave/app/session/SessionSyncStatusContext.js'

// toSessionSyncView projects the rich SessionSyncStatusView onto a LoadingView
// suitable for the LoadingCard primitive. The shape of the existing view is
// preserved; only fields relevant to a generic loading surface are surfaced.
export function toSessionSyncView(
  status: SessionSyncStatusView,
  options: { onRetry?: () => void } = {},
): LoadingView {
  const { onRetry } = options
  const base: LoadingView = {
    state: status.visualState,
    title: status.summaryLabel,
    detail: status.detailLabel,
    rate: {
      up: status.uploadRateLabel,
      down: status.downloadRateLabel,
    },
    lastActivity: status.lastActivityLabel,
  }
  if (status.error) {
    return {
      ...base,
      state: 'error',
      error: status.lastError || status.detailLabel,
      onRetry,
    }
  }
  return base
}
