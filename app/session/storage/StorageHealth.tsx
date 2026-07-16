import type { ReactNode } from 'react'

import {
  LuArrowRight,
  LuCloud,
  LuCloudOff,
  LuDatabase,
  LuHardDrive,
  LuLink,
  LuRefreshCw,
  LuShieldCheck,
  LuTriangleAlert,
} from 'react-icons/lu'

import { useStorageHealth } from '@s4wave/app/session/storage/useStorageHealth.js'
import { cn } from '@s4wave/web/style/utils.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'

interface StorageHealthProps {
  mode: 'settings' | 'recovery'
  onOpenRecovery?: () => void
  onLinkDevice: () => void
  onUseCloud: () => void
}

// StorageHealth renders the shared settings and systemic-recovery storage facts.
export function StorageHealth({
  mode,
  onOpenRecovery,
  onLinkDevice,
  onUseCloud,
}: StorageHealthProps) {
  const health = useStorageHealth()
  const originLabel = formatOriginUsage(
    health.originUsageBytes,
    health.originQuotaBytes,
  )

  return (
    <div className="space-y-4" data-testid={`storage-health-${mode}`}>
      {mode === 'recovery' && (
        <div
          className="border-destructive/30 bg-destructive/8 rounded-lg border p-3.5"
          role="alert"
          aria-live="assertive"
        >
          <div className="flex items-start gap-3">
            <div className="bg-destructive/10 text-destructive flex size-8 shrink-0 items-center justify-center rounded-md">
              <LuTriangleAlert className="size-4" aria-hidden="true" />
            </div>
            <div>
              <h2 className="text-foreground text-sm font-semibold tracking-tight">
                Spacewave cannot save reliably
              </h2>
              <p className="text-foreground-alt/70 mt-1 text-xs leading-relaxed">
                Browser storage is exhausted or unavailable for app-critical
                writes. Free storage before continuing work that must be saved.
              </p>
            </div>
          </div>
        </div>
      )}

      <InfoCard>
        <div className="divide-foreground/6 divide-y">
          <HealthRow
            icon={<LuDatabase className="size-3.5" />}
            label="Saved on this browser"
            value={
              health.providerLoading
                ? 'Checking local storage'
                : health.providerSupported
                  ? `${formatBytes(health.providerBytes)} in the local provider`
                  : 'Local provider usage unavailable'
            }
            tone="neutral"
          />
          <HealthRow
            icon={<LuHardDrive className="size-3.5" />}
            label="Approximate browser usage"
            value={originLabel}
            tone="neutral"
          />
          <HealthRow
            icon={<LuShieldCheck className="size-3.5" />}
            label="Protected from browser cleanup"
            value={protectionLabel(health.protectionState)}
            tone={
              health.protectionState === 'protected' ? 'positive' : 'warning'
            }
          />
          <HealthRow
            icon={<LuRefreshCw className="size-3.5" />}
            label="Sync activity"
            value={health.sync.summaryLabel}
            tone={health.sync.error ? 'warning' : 'neutral'}
          />
          <HealthRow
            icon={<LuCloudOff className="size-3.5" />}
            label="Backed up / replicated"
            value={health.replicaLabel}
            tone="warning"
          />
        </div>
      </InfoCard>

      <details className="border-foreground/6 bg-background-card/30 rounded-lg border">
        <summary className="text-foreground-alt hover:text-foreground cursor-pointer px-3.5 py-2.5 text-xs font-medium">
          Why these readings matter
        </summary>
        <div className="border-foreground/6 text-foreground-alt/60 space-y-2 border-t px-3.5 py-3 text-[0.6rem] leading-relaxed">
          <p>
            <strong className="text-foreground-alt/80">
              Saved on this browser:
            </strong>{' '}
            {health.providerSupported
              ? `${Number(health.blockCount).toLocaleString()} stored blocks`
              : 'This browser copy can still be lost through clearing, eviction, profile loss, or device loss.'}
          </p>
          <p>
            <strong className="text-foreground-alt/80">
              Approximate browser usage:
            </strong>{' '}
            {health.browserReadFailed
              ? 'The browser refused the storage-health query.'
              : 'Whole-origin estimates are approximate and may exceed real free disk headroom.'}
          </p>
          <p>
            <strong className="text-foreground-alt/80">
              Protected from browser cleanup:
            </strong>{' '}
            {protectionDetail(health.protectionState)}
          </p>
          <p>
            <strong className="text-foreground-alt/80">Sync activity:</strong>{' '}
            {health.sync.detailLabel}
          </p>
          <p>
            <strong className="text-foreground-alt/80">
              Backed up / replicated:
            </strong>{' '}
            Sync activity, pairing, and an empty queue do not prove that another
            copy can restore this Space.
          </p>
        </div>
      </details>

      {mode === 'settings' && health.safariCleanupRisk && (
        <div
          className="border-warning/20 bg-warning/5 rounded-lg border p-3.5"
          data-testid="safari-storage-risk"
        >
          <div className="flex items-start gap-2.5">
            <LuTriangleAlert
              className="text-warning mt-0.5 size-4 shrink-0"
              aria-hidden="true"
            />
            <div>
              <h3 className="text-foreground text-xs font-medium">
                Safari storage policy
              </h3>
              <p className="text-foreground-alt/70 mt-1 text-xs leading-relaxed">
                Safari may remove this site&apos;s local data after seven days
                of Safari use without interaction. Only a verified replica on
                another device or Spacewave Cloud makes that loss recoverable.
              </p>
            </div>
          </div>
        </div>
      )}

      <section aria-labelledby="storage-solutions-title">
        <div className="mb-2 flex items-center justify-between gap-3">
          <div>
            <h2
              id="storage-solutions-title"
              className="text-foreground text-xs font-medium"
            >
              Storage and recovery options
            </h2>
            <p className="text-foreground-alt/60 mt-0.5 text-xs">
              Use a non-destructive option first.
            </p>
          </div>
          {mode === 'settings' && onOpenRecovery && (
            <DashboardButton
              icon={<LuTriangleAlert className="size-3.5" />}
              onClick={onOpenRecovery}
            >
              Recovery view
            </DashboardButton>
          )}
        </div>

        <div className="space-y-2">
          <SolutionRow
            icon={<LuHardDrive className="size-3.5" />}
            title="Free browser or device storage"
            detail="Remove unrelated downloads, applications, or other site data, then retry the failed operation. Do not clear Spacewave site data unless important Spaces are backed up or exported."
          />
          <SolutionRow
            icon={<LuLink className="size-3.5" />}
            title="Link another device"
            detail="Set up another copy. Spacewave will keep reporting Not yet verified until it can prove the current Space is restorable there."
            action="Link device"
            onAction={onLinkDevice}
          />
          <SolutionRow
            icon={<LuCloud className="size-3.5" />}
            title="Set up Spacewave Cloud"
            detail="Move the session toward a remote copy. Account setup alone is not a backup verification."
            action="Cloud options"
            onAction={onUseCloud}
          />
          <SolutionRow
            icon={<LuDatabase className="size-3.5" />}
            title="Export important Spaces"
            detail="Open each important Space and choose File → Export Space. Keep the archive outside this browser as a last-resort copy."
          />
          <SolutionRow
            icon={<LuCloudOff className="size-3.5" />}
            title="Remove a local Space copy"
            detail="Unavailable until this browser verifies that a current remote replica can restore the Space."
          />
        </div>
      </section>
    </div>
  )
}

function HealthRow({
  icon,
  label,
  value,
  tone,
}: {
  icon: ReactNode
  label: string
  value: string
  tone: 'neutral' | 'positive' | 'warning'
}) {
  return (
    <div className="flex items-start gap-3 py-3 first:pt-0 last:pb-0">
      <div
        className={cn(
          'flex size-7 shrink-0 items-center justify-center rounded-md',
          tone === 'positive' && 'bg-success/10 text-success',
          tone === 'warning' && 'bg-warning/10 text-warning',
          tone === 'neutral' && 'bg-foreground/5 text-foreground-alt',
        )}
      >
        {icon}
      </div>
      <div className="min-w-0 flex-1">
        <div className="text-foreground-alt/60 text-xs">{label}</div>
        <div className="text-foreground mt-0.5 text-xs font-medium">
          {value}
        </div>
      </div>
    </div>
  )
}

function SolutionRow({
  icon,
  title,
  detail,
  action,
  onAction,
}: {
  icon: ReactNode
  title: string
  detail: string
  action?: string
  onAction?: () => void
}) {
  return (
    <div className="border-foreground/6 bg-background-card/30 flex items-start gap-3 rounded-lg border p-3.5">
      <div className="bg-foreground/5 text-foreground-alt flex size-7 shrink-0 items-center justify-center rounded-md">
        {icon}
      </div>
      <div className="min-w-0 flex-1">
        <h3 className="text-foreground text-xs font-medium">{title}</h3>
        <p className="text-foreground-alt/60 mt-1 text-xs leading-relaxed">
          {detail}
        </p>
      </div>
      {action && onAction && (
        <DashboardButton
          icon={<LuArrowRight className="size-3.5" />}
          onClick={onAction}
        >
          {action}
        </DashboardButton>
      )}
    </div>
  )
}

function protectionLabel(state: string): string {
  switch (state) {
    case 'protected':
      return 'Protected'
    case 'not-protected':
      return 'Not protected'
    case 'checking':
      return 'Checking browser status'
    default:
      return 'Status unavailable'
  }
}

function protectionDetail(state: string): string {
  if (state === 'protected') {
    return 'The browser reports persistent storage. This reduces automatic-eviction risk but is not a backup.'
  }
  if (state === 'not-protected') {
    return 'The browser has not granted persistent storage. Spacewave requests it quietly after the first successful durable write.'
  }
  return 'Persistence status could not be confirmed. It never changes whether a write is accepted.'
}

function formatOriginUsage(usage: number | null, quota: number | null): string {
  if (usage == null && quota == null) return 'Estimate unavailable'
  if (usage == null) return `Quota about ${formatBytes(quota ?? 0)}`
  if (quota == null || quota <= 0) return `${formatBytes(usage)} used`
  const percent = Math.min(100, Math.round((usage / quota) * 100))
  return `${formatBytes(usage)} of ${formatBytes(quota)} (${percent}%)`
}

function formatBytes(bytes: bigint | number): string {
  const value = typeof bytes === 'bigint' ? Number(bytes) : bytes
  if (!Number.isFinite(value) || value <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const index = Math.min(
    Math.floor(Math.log(value) / Math.log(1024)),
    units.length - 1,
  )
  const amount = value / 1024 ** index
  return `${amount >= 10 || index === 0 ? amount.toFixed(0) : amount.toFixed(1)} ${units[index]}`
}
