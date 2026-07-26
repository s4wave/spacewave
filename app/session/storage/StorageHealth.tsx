import type { ReactNode } from 'react'

import {
  LuArrowRight,
  LuCloud,
  LuDatabase,
  LuHardDrive,
  LuLink,
  LuRefreshCw,
  LuShieldCheck,
  LuTriangleAlert,
} from 'react-icons/lu'

import {
  type StorageHealthView,
  useStorageHealth,
} from '@s4wave/app/session/storage/useStorageHealth.js'
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
          className="border-success/20 bg-success/5 rounded-lg border p-3.5"
          role="status"
        >
          <div className="flex items-start gap-3">
            <div className="bg-success/10 text-success flex size-8 shrink-0 items-center justify-center rounded-md">
              <LuShieldCheck className="size-4" aria-hidden="true" />
            </div>
            <div>
              <h2 className="text-foreground text-sm font-semibold tracking-tight">
                Saving works
              </h2>
              <p className="text-foreground-alt/70 mt-1 text-xs leading-relaxed">
                No durable write failure is reported on this screen. These
                actions help protect your Spaces and create another copy.
              </p>
            </div>
          </div>
        </div>
      )}

      <ScopeSection
        title="This browser"
        description="These readings cover all Spacewave data stored by this browser."
      >
        <HealthRow
          icon={<LuHardDrive className="size-3.5" />}
          label="Browser usage estimate"
          value={originLabel}
          detail={
            health.browserReadFailed
              ? 'The browser did not provide this estimate.'
              : 'An approximate whole-origin reading, not a measure of free disk space.'
          }
          tone="neutral"
        />
        <HealthRow
          icon={<LuShieldCheck className="size-3.5" />}
          label="Automatic cleanup protection"
          value={protectionLabel(health.protectionState)}
          detail="When on, the browser is less likely to remove this copy automatically. It is not a backup."
          tone={health.protectionState === 'protected' ? 'positive' : 'neutral'}
        />
        {mode === 'settings' && health.safariCleanupRisk && (
          <div
            className="border-warning/20 bg-warning/5 mt-3 rounded-lg border p-3.5"
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
                  without interaction. An exported archive or a verified replica
                  protects against losing this browser copy.
                </p>
              </div>
            </div>
          </div>
        )}
      </ScopeSection>

      <ScopeSection
        title="This device's store"
        description="These readings describe Spacewave's local block store on this device."
      >
        <HealthRow
          icon={<LuDatabase className="size-3.5" />}
          label="Physical storage used"
          value={
            health.providerLoading
              ? 'Checking local store'
              : health.providerSupported
                ? `${formatBytes(health.providerBytes)} physical`
                : 'Local store unavailable'
          }
          detail={
            health.providerSupported
              ? 'This is the space currently occupied by the local store, not a count of user Spaces.'
              : 'The local store did not provide a reading.'
          }
          tone="neutral"
        />
        {health.providerSupported && (
          <details className="border-foreground/6 mt-3 border-t pt-3">
            <summary className="text-foreground-alt hover:text-foreground cursor-pointer text-xs font-medium">
              Technical details
            </summary>
            <p className="text-foreground-alt/60 mt-2 text-[0.6rem] leading-relaxed">
              {Number(health.blockCount).toLocaleString()} block entries are
              currently reported by the local store. Entries are a storage
              measurement, not proof of a complete backup.
            </p>
          </details>
        )}
      </ScopeSection>

      <ScopeSection
        title="Sync activity"
        description="This shows transfer work that Spacewave can currently see."
      >
        <HealthRow
          icon={<LuRefreshCw className="size-3.5" />}
          label="Transfer activity"
          value={transferActivityLabel(health.sync)}
          detail={transferActivityDetail(health.sync)}
          tone={health.sync.error ? 'warning' : 'neutral'}
        />
        <details className="border-foreground/6 mt-3 border-t pt-3">
          <summary className="text-foreground-alt hover:text-foreground cursor-pointer text-xs font-medium">
            Technical details
          </summary>
          <div className="text-foreground-alt/60 mt-2 space-y-1 text-[0.6rem] leading-relaxed">
            <p>{health.sync.detailLabel}</p>
            <p>Upload waiting: {health.sync.pendingUploadLabel}</p>
            <p>Download waiting: {health.sync.pendingDownloadLabel}</p>
          </div>
        </details>
      </ScopeSection>

      <section aria-labelledby="storage-solutions-title">
        <div className="mb-2 flex items-center justify-between gap-3">
          <div>
            <h2
              id="storage-solutions-title"
              className="text-foreground text-xs font-medium"
            >
              Protect your Spaces
            </h2>
            <p className="text-foreground-alt/60 mt-0.5 text-xs">
              Choose an action that creates or protects a copy.
            </p>
          </div>
          {mode === 'settings' && onOpenRecovery && (
            <DashboardButton
              icon={<LuShieldCheck className="size-3.5" />}
              onClick={onOpenRecovery}
            >
              Storage recovery
            </DashboardButton>
          )}
        </div>
        <div className="space-y-2">
          <SolutionRow
            icon={<LuShieldCheck className="size-3.5" />}
            title="Request automatic cleanup protection"
            detail="Requesting this protects this browser copy from automatic cleanup. This does not create another copy."
            action="Request protection"
            onAction={() => {
              void health.requestProtection()
            }}
          />
          <SolutionRow
            icon={<LuDatabase className="size-3.5" />}
            title="Export a backup"
            detail="Open an important Space and choose File > Export Space. Keep the archive outside this browser."
          />
          <SolutionRow
            icon={<LuLink className="size-3.5" />}
            title="Link another device"
            detail="Set up another destination for a second copy. Linking alone does not verify that the copy can restore this Space."
            action="Link device"
            onAction={onLinkDevice}
          />
          <SolutionRow
            icon={<LuCloud className="size-3.5" />}
            title="Set up Spacewave Cloud"
            detail="Choose Spacewave Cloud as another destination. Setup alone does not verify a complete copy."
            action="Cloud options"
            onAction={onUseCloud}
          />
          <SolutionRow
            icon={<LuHardDrive className="size-3.5" />}
            title="Make room for future saves"
            detail="If a save fails, remove unrelated downloads, applications, or site data before retrying. Do not clear Spacewave site data as an ordinary fix."
          />
        </div>
      </section>
    </div>
  )
}

function ScopeSection({
  title,
  description,
  children,
}: {
  title: string
  description: string
  children: ReactNode
}) {
  return (
    <section aria-labelledby={`${title}-title`}>
      <h2 id={`${title}-title`} className="text-foreground text-xs font-medium">
        {title}
      </h2>
      <p className="text-foreground-alt/60 mt-0.5 text-xs">{description}</p>
      <div className="mt-2">
        <InfoCard>
          <div>{children}</div>
        </InfoCard>
      </div>
    </section>
  )
}

function HealthRow({
  icon,
  label,
  value,
  detail,
  tone,
}: {
  icon: ReactNode
  label: string
  value: string
  detail: string
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
        <p className="text-foreground-alt/60 mt-1 text-xs leading-relaxed">
          {detail}
        </p>
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
      return 'On'
    case 'not-protected':
      return 'Not on'
    case 'checking':
      return 'Checking'
    default:
      return 'Unavailable'
  }
}

function transferActivityLabel(sync: StorageHealthView['sync']): string {
  if (sync.error) return 'Needs attention'
  if (sync.loading) return 'Checking'
  if (sync.active) return sync.summaryLabel
  return 'Idle'
}

function transferActivityDetail(sync: StorageHealthView['sync']): string {
  if (sync.error) {
    return sync.lastError || 'Spacewave reported a transfer problem.'
  }
  if (sync.loading) return 'Waiting for the transfer status.'
  if (sync.active) return sync.detailLabel
  return 'No transfer is currently reported. This does not verify another copy.'
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
