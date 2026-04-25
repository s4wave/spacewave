import {
  LuCircleAlert,
  LuCircleCheck,
  LuCloud,
  LuHardDrive,
} from 'react-icons/lu'

import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { cn } from '@s4wave/web/style/utils.js'
import {
  type SessionSyncStatusView,
  useSessionSyncStatus,
} from '@s4wave/app/session/SessionSyncStatusContext.js'

// SessionSyncStatusSummary renders a compact sync status card for SessionDetails.
export function SessionSyncStatusSummary() {
  const status = useSessionSyncStatus()
  return (
    <InfoCard>
      <div
        className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between"
        data-testid="session-sync-status-summary"
      >
        <div className="flex min-w-0 items-start gap-3">
          <div
            className={cn(
              'flex h-8 w-8 shrink-0 items-center justify-center rounded-md',
              status.error && 'bg-destructive/10 text-destructive',
              status.active && 'bg-brand/10 text-brand',
              status.visualState === 'synced' && 'bg-foreground/5 text-brand',
              status.loading && 'bg-foreground/5 text-foreground-alt',
            )}
          >
            <SessionSyncStatusSummaryIcon status={status} />
          </div>
          <div className="min-w-0">
            <div className="text-foreground text-sm font-semibold tracking-tight">
              {status.summaryLabel}
            </div>
            <div className="text-foreground-alt/60 mt-0.5 text-xs">
              {status.detailLabel}
            </div>
          </div>
        </div>
        <div className="grid grid-cols-2 gap-2 md:w-52">
          <SummaryPill label="Up" value={status.uploadRateLabel} />
          <SummaryPill label="Down" value={status.downloadRateLabel} />
        </div>
      </div>
    </InfoCard>
  )
}

function SessionSyncStatusSummaryIcon({
  status,
}: {
  status: SessionSyncStatusView
}) {
  if (status.loading || status.active) {
    return <Spinner />
  }
  if (status.error) {
    return <LuCircleAlert className="h-4 w-4" aria-hidden="true" />
  }
  const Icon = status.local ? LuHardDrive : LuCloud
  return (
    <span
      className="relative flex h-4 w-4 items-center justify-center"
      aria-hidden="true"
    >
      <Icon className="h-3.5 w-3.5" />
      <LuCircleCheck className="text-brand absolute -right-1 -bottom-1 h-2.5 w-2.5" />
    </span>
  )
}

function SummaryPill({ label, value }: { label: string; value: string }) {
  return (
    <div className="border-foreground/6 bg-foreground/5 rounded-md border px-2 py-1">
      <div className="text-foreground-alt/50 text-[0.55rem] font-medium tracking-widest uppercase">
        {label}
      </div>
      <div className="text-foreground text-xs font-semibold">{value}</div>
    </div>
  )
}
