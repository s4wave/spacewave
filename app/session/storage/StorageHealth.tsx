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
} from '@s4wave/web/images/icons.js'

import {
  type StorageHealthView,
  useStorageHealth,
} from '@s4wave/app/session/storage/useStorageHealth.js'

import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  fonts as themeFonts,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles4 = stylex.create({
  rule1: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-warning']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-warning']} 5%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule3: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-warning']} 10%, transparent)`,
    color: themeColors['--color-warning'],
  },
  rule4: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
  },
  rule5: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule6: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-success']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-success']} 5%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule7: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-success']} 10%, transparent)`,
    color: themeColors['--color-success'],
  },
  rule8: {
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-warning']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-warning']} 5%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule9: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
  rule10: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule11: {
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule12: {
    cursor: 'pointer',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    paddingBlock: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 3.5)`,
    },
  },
  rule13: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule14: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule15: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule16: { marginTop: `calc(${themeLayout['--spacing']} * 2)` },
  rule17: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 7)`,
    height: `calc(${themeLayout['--spacing']} * 7)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
  },
  rule18: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-success']} 10%, transparent)`,
    color: themeColors['--color-success'],
  },
  rule19: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-warning']} 10%, transparent)`,
    color: themeColors['--color-warning'],
  },
  rule20: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: themeColors['--color-foreground-alt'],
  },
  rule21: { minWidth: 0, flex: 1 },
  rule22: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule23: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule24: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule25: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule26: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 7)`,
    height: `calc(${themeLayout['--spacing']} * 7)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: themeColors['--color-foreground-alt'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule3: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
    color: themeColors['--color-warning'],
  },
})

interface StorageHealthProps {
  mode: 'settings' | 'recovery'
  storageIncident?: boolean
  onOpenRecovery?: () => void
  onLinkDevice: () => void
  onUseCloud: () => void
}

// StorageHealth renders the shared settings and systemic-recovery storage facts.
export function StorageHealth({
  mode,
  storageIncident = false,
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
    <div data-testid={`storage-health-${mode}`}>
      {mode === 'recovery' &&
        (storageIncident ? (
          <div
            sx={[nativeChildSpacingNext.s4, stylexStyles2.rule1]}
            role="alert"
          >
            <div sx={stylexStyles2.rule2}>
              <div sx={stylexStyles2.rule3}>
                <LuTriangleAlert sx={stylexStyles.rule1} aria-hidden="true" />
              </div>
              <div>
                <h2 sx={stylexStyles2.rule4}>A recent save needs attention</h2>
                <p sx={stylexStyles2.rule5}>
                  Spacewave reported a durable write failure. Free unrelated
                  storage before retrying the save.
                </p>
              </div>
            </div>
          </div>
        ) : (
          <div
            sx={[nativeChildSpacingNext.s4, stylexStyles2.rule6]}
            role="status"
          >
            <div sx={stylexStyles2.rule2}>
              <div sx={stylexStyles2.rule7}>
                <LuShieldCheck sx={stylexStyles.rule1} aria-hidden="true" />
              </div>
              <div>
                <h2 sx={stylexStyles2.rule4}>Saving works</h2>
                <p sx={stylexStyles2.rule5}>
                  No durable write failure is reported on this screen. These
                  actions help protect your Spaces and create another copy.
                </p>
              </div>
            </div>
          </div>
        ))}

      <ScopeSection
        sx={nativeChildSpacingNext.s4}
        title="This browser"
        description="These readings cover all Spacewave data stored by this browser."
      >
        <HealthRow
          icon={<LuHardDrive sx={stylexStyles.rule2} />}
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
          icon={<LuShieldCheck sx={stylexStyles.rule2} />}
          label="Automatic cleanup protection"
          value={protectionLabel(health.protectionState)}
          detail="When on, the browser is less likely to remove this copy automatically. It is not a backup."
          tone={health.protectionState === 'protected' ? 'positive' : 'neutral'}
        />
        {mode === 'settings' && health.safariCleanupRisk && (
          <div sx={stylexStyles2.rule8} data-testid="safari-storage-risk">
            <div sx={stylexStyles2.rule9}>
              <LuTriangleAlert sx={stylexStyles.rule3} aria-hidden="true" />
              <div>
                <h3 sx={stylexStyles2.rule10}>Safari storage policy</h3>
                <p sx={stylexStyles2.rule5}>
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
        sx={nativeChildSpacingNext.s4}
        title="This device's store"
        description="These readings describe Spacewave's local block store on this device."
      >
        <HealthRow
          icon={<LuDatabase sx={stylexStyles.rule2} />}
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
          <details sx={stylexStyles2.rule11}>
            <summary sx={stylexStyles2.rule12}>Technical details</summary>
            <p sx={stylexStyles2.rule13}>
              {Number(health.blockCount).toLocaleString()} block entries are
              currently reported by the local store. Entries are a storage
              measurement, not proof of a complete backup.
            </p>
          </details>
        )}
      </ScopeSection>

      <ScopeSection
        sx={nativeChildSpacingNext.s4}
        title="Sync activity"
        description="This shows transfer work that Spacewave can currently see."
      >
        <HealthRow
          icon={<LuRefreshCw sx={stylexStyles.rule2} />}
          label="Transfer activity"
          value={transferActivityLabel(health.sync)}
          detail={transferActivityDetail(health.sync)}
          tone={health.sync.error ? 'warning' : 'neutral'}
        />
        <details sx={stylexStyles2.rule11}>
          <summary sx={stylexStyles2.rule12}>Technical details</summary>
          <div sx={stylexStyles3.rule1}>
            <p sx={nativeChildSpacing.s1}>{health.sync.detailLabel}</p>
            <p sx={nativeChildSpacing.s1}>
              Upload waiting: {health.sync.pendingUploadLabel}
            </p>
            <p sx={nativeChildSpacing.s1}>
              Download waiting: {health.sync.pendingDownloadLabel}
            </p>
          </div>
        </details>
      </ScopeSection>

      <section
        sx={nativeChildSpacingNext.s4}
        aria-labelledby="storage-solutions-title"
      >
        <div sx={stylexStyles2.rule14}>
          <div>
            <h2 id="storage-solutions-title" sx={stylexStyles2.rule10}>
              Protect your Spaces
            </h2>
            <p sx={stylexStyles2.rule15}>
              Choose an action that creates or protects a copy.
            </p>
          </div>
          {mode === 'settings' && onOpenRecovery && (
            <DashboardButton
              icon={<LuShieldCheck sx={stylexStyles.rule2} />}
              onClick={onOpenRecovery}
            >
              Storage recovery
            </DashboardButton>
          )}
        </div>
        <div>
          <SolutionRow
            sx={nativeChildSpacingNext.s2}
            icon={<LuShieldCheck sx={stylexStyles.rule2} />}
            title="Request automatic cleanup protection"
            detail="Requesting this protects this browser copy from automatic cleanup. This does not create another copy."
            action="Request protection"
            onAction={() => {
              void health.requestProtection()
            }}
          />
          <SolutionRow
            sx={nativeChildSpacingNext.s2}
            icon={<LuDatabase sx={stylexStyles.rule2} />}
            title="Export a backup"
            detail="Open an important Space and choose File > Export Space. Keep the archive outside this browser."
          />
          <SolutionRow
            sx={nativeChildSpacingNext.s2}
            icon={<LuLink sx={stylexStyles.rule2} />}
            title="Link another device"
            detail="Set up another destination for a second copy. Linking alone does not verify that the copy can restore this Space."
            action="Link device"
            onAction={onLinkDevice}
          />
          <SolutionRow
            sx={nativeChildSpacingNext.s2}
            icon={<LuCloud sx={stylexStyles.rule2} />}
            title="Set up Spacewave Cloud"
            detail="Choose Spacewave Cloud as another destination. Setup alone does not verify a complete copy."
            action="Cloud options"
            onAction={onUseCloud}
          />
          <SolutionRow
            sx={nativeChildSpacingNext.s2}
            icon={<LuHardDrive sx={stylexStyles.rule2} />}
            title="Make room for future saves"
            detail="If a save fails, remove unrelated downloads, applications, or site data before retrying. Do not clear Spacewave site data as an ordinary fix."
          />
        </div>
      </section>
    </div>
  )
}

function ScopeSection({
  sx,
  title,
  description,
  children,
}: {
  title: string
  description: string
  children: ReactNode
} & { sx?: stylex.StyleXStyles; className?: never }) {
  return (
    <section sx={sx} aria-labelledby={`${title}-title`}>
      <h2 id={`${title}-title`} sx={stylexStyles2.rule10}>
        {title}
      </h2>
      <p sx={stylexStyles2.rule15}>{description}</p>
      <div sx={stylexStyles2.rule16}>
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
    <div sx={[stylexStyles4.rule1, edgeSpacing.s3]}>
      <div
        sx={[
          stylexStyles2.rule17,
          tone === 'positive' && stylexStyles2.rule18,
          tone === 'warning' && stylexStyles2.rule19,
          tone === 'neutral' && stylexStyles2.rule20,
        ]}
      >
        {icon}
      </div>
      <div sx={stylexStyles2.rule21}>
        <div sx={stylexStyles2.rule22}>{label}</div>
        <div sx={stylexStyles2.rule23}>{value}</div>
        <p sx={stylexStyles2.rule24}>{detail}</p>
      </div>
    </div>
  )
}

function SolutionRow({
  sx,
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
} & { sx?: stylex.StyleXStyles; className?: never }) {
  return (
    <div sx={[stylexStyles2.rule25, sx]}>
      <div sx={stylexStyles2.rule26}>{icon}</div>
      <div sx={stylexStyles2.rule21}>
        <h3 sx={stylexStyles2.rule10}>{title}</h3>
        <p sx={stylexStyles2.rule24}>{detail}</p>
      </div>
      {action && onAction && (
        <DashboardButton
          icon={<LuArrowRight sx={stylexStyles.rule2} />}
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

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s1: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
  s4: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

// edgeSpacing keeps first and last row padding at the row owner.
const edgeSpacing = stylex.create({
  s3: {
    paddingTop: { default: 'calc(var(--spacing) * 3)', ':first-child': 0 },
    paddingBottom: { default: 'calc(var(--spacing) * 3)', ':last-child': 0 },
  },
})
