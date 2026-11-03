import type { IconType } from '@s4wave/web/images/icons.js'
import { useAppEnvironment } from '@s4wave/web/sdk/app/environment.js'
import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type ComponentProps,
  type ReactNode,
} from 'react'
import {
  DebugInfo,
  DebugInfoProvider,
  useWatchStateRpc,
} from '@aptre/bldr-react'
import {
  SharedObjectHealthCommonReason,
  SharedObjectHealthLayer,
  SharedObjectHealthRemediationHint,
  SharedObjectHealthStatus,
  type SharedObjectHealth,
} from '@s4wave/core/sobject/sobject.pb.js'
import { AccountEscalationIntentKind } from '@s4wave/sdk/account/account.pb.js'
import {
  LuArrowRight,
  LuCircleAlert,
  LuRefreshCw,
  LuRotateCcw,
  LuShieldAlert,
  LuTriangleAlert,
} from '@s4wave/web/images/icons.js'

import { ORG_ROLE_OWNER } from '@s4wave/app/org/org-constants.js'
import { SpaceMountingScreen } from '@s4wave/app/space/SpaceMountingScreen.js'
import { spaceMountStageFromHealth } from '@s4wave/app/space/spaceMountStage.js'
import { isStorageQuotaError } from '@s4wave/app/session/storage/storage-error.js'
import {
  useNavigate,
  useParams,
  useParentPaths,
} from '@s4wave/web/router/router.js'
import {
  SessionContext,
  SharedObjectContext,
  SharedObjectBodyContext,
  useSessionNavigate,
  useSessionIndex,
} from '@s4wave/web/contexts/contexts.js'
import { SpacewaveOrgListContext } from '@s4wave/web/contexts/SpacewaveOrgListContext.js'
import {
  useResourceValue,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import type { OrganizationInfo } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import {
  MountSharedObjectResponse,
  WatchSharedObjectHealthRequest,
  WatchSharedObjectHealthResponse,
  WatchResourcesListRequest,
  WatchResourcesListResponse,
} from '@s4wave/sdk/session/session.pb.js'
import type { Session } from '@s4wave/sdk/session/session.js'
import { SharedObjectBodyContainer } from '@s4wave/app/sobject/SharedObjectBodyContainer.js'
import { SharedObjectSyncNotice } from '@s4wave/app/sobject/SharedObjectSyncNotice.js'
import { ErrorState } from '@s4wave/web/ui/ErrorState.js'
import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'
import { BackButton } from '@s4wave/web/ui/BackButton.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { useMountAccount } from '@s4wave/web/hooks/useMountAccount.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'

import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@s4wave/web/ui/tooltip.js'

import { SessionFrame } from './SessionFrame.js'
import { AccountDashboardStateProvider } from './dashboard/AccountDashboardStateContext.js'
import { AuthConfirmDialog } from './dashboard/AuthConfirmDialog.js'
import { useSessionSelfEnrollmentStatus } from './SessionSelfEnrollmentStatusContext.js'
import {
  buildSharedObjectFallbackHealth,
  getSharedObjectRouteHealth,
} from './sharedObjectHealthFallback.js'
import {
  clearQuickstartRouteAwaitingResourcesList,
  isQuickstartRouteAwaitingResourcesList,
  retainQuickstartRoute,
} from '@s4wave/app/quickstart/session-handoff.js'
import {
  useSharedObjectBodyMount,
  useSharedObjectMount,
} from '@s4wave/app/hooks/useSharedMounts.js'

import * as stylex from '@stylexjs/stylex'

import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../web/style/tokens.stylex.js'

const stylexStyles7 = stylex.create({
  rule1: { marginTop: `calc(${themeLayout['--spacing']} * 5)` },
})

const stylexStyles6 = stylex.create({
  rule1: {
    padding: `calc(${themeLayout['--spacing']} * 5)`,
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
  },
  rule2: {
    borderRadius: '3.40282e38px',
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 12)`,
    width: `calc(${themeLayout['--spacing']} * 12)`,
  },
  rule3: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-widest)',
    WebkitUserSelect: 'none',
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: '.55rem',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-widest)',
    lineHeight: 1.5,
    textTransform: 'uppercase',
    userSelect: 'none',
  },
})

const stylexStyles5 = stylex.create({
  rule1: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
  },
  rule2: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
  },
  rule3: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
  },
  rule4: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule5: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-warning']} 20%, transparent)`,
  },
  rule6: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-warning']} 5%, transparent)`,
  },
  rule7: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-warning']} 10%, transparent)`,
  },
  rule8: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-warning']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-warning']} 10%, transparent)`,
    color: themeColors['--color-warning'],
  },
  rule9: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
  },
  rule10: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
  },
  rule11: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
  },
  rule12: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
    color: themeColors['--color-destructive'],
  },
})

const stylexStyles4 = stylex.create({
  rule1: { display: 'inline-flex' },
  rule2: {
    overflow: 'auto',
    paddingBlock: `calc(${themeLayout['--spacing']} * 12)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'flex-start',
    display: 'flex',
    justifyContent: 'center',
    position: 'relative',
    height: '100%',
    width: '100%',
  },
  rule3: {
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    display: 'flex',
    flexDirection: 'column',
    maxWidth: 'var(--container-xl)',
    width: '100%',
  },
  rule4: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
    textAlign: 'center',
  },
  rule5: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
    fontSize: 'var(--text-base)',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-tight)',
    lineHeight: 'var(--tw-leading, var(--text-base--line-height))',
  },
  rule6: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    maxWidth: 'var(--container-sm)',
  },
  rule7: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
  },
  rule8: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule9: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
  },
  rule10: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    marginTop: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule11: {
    '--tw-leading': 'var(--leading-relaxed)',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 80%, transparent)`,
    fontSize: '.7rem',
    lineHeight: 'var(--leading-relaxed)',
    overflowWrap: 'break-word',
    whiteSpace: 'pre-wrap',
    marginTop: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
  rule12: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    flexWrap: 'wrap',
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule13: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 55%, transparent)`,
    fontSize: '.7rem',
    lineHeight: 1.5,
    marginTop: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
  rule14: {
    color: themeColors['--color-destructive'],
    fontSize: '.7rem',
    lineHeight: 1.5,
    marginTop: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
  rule15: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule16: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    WebkitUserSelect: 'none',
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
  },
  rule17: {
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textDecorationLine: 'underline',
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    color: themeColors['--color-destructive'],
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

const stylexStyles = stylex.create({
  rule1: { maxWidth: 'var(--container-xs)' },
})

interface SharedObjectMutationPermission {
  canMutate: boolean
  disabledReason: string
}

type SharedObjectRemediationAction = 'repair' | 'reinitialize' | null

// credentialRequiredMessage reports a repair that needs an unlocked entity key.
const credentialRequiredMessage =
  'Shared object recovery requires entity credentials'

function getHealthSummary(health: SharedObjectHealth): {
  badge: string
  title: string
  description: string
  hint: string
} {
  // Distinguish opening the space from loading its body contents.
  if (health.status === SharedObjectHealthStatus.LOADING) {
    if (health.layer === SharedObjectHealthLayer.BODY) {
      return {
        badge: 'Loading',
        title: 'Loading space',
        description: 'Almost ready. Loading the space contents.',
        hint: '',
      }
    }
    return {
      badge: 'Loading',
      title: 'Loading space',
      description: 'Mounting the space.',
      hint: '',
    }
  }

  // Explain when the requested shared object no longer exists.
  if (health.commonReason === SharedObjectHealthCommonReason.NOT_FOUND) {
    return {
      badge: 'Closed',
      title: 'Shared object not found',
      description:
        'This shared object is no longer available from the current account or provider.',
      hint: 'Ask the owner for an updated link or confirm the object still exists.',
    }
  }

  // Explain when the session no longer has access to this object.
  if (health.commonReason === SharedObjectHealthCommonReason.ACCESS_REVOKED) {
    return {
      badge: 'Closed',
      title: 'Access revoked',
      description:
        'The current session is no longer allowed to read this shared object.',
      hint: 'Request access again or confirm the correct account is open.',
    }
  }

  // Explain why failed initial-state verification closed the mount.
  if (
    health.commonReason ===
    SharedObjectHealthCommonReason.INITIAL_STATE_REJECTED
  ) {
    return {
      badge: 'Closed',
      title: 'Initial state rejected',
      description:
        'The shared object state failed verification, so Alpha closed the mount instead of retrying indefinitely.',
      hint: 'The owner needs to repair or republish the shared object state.',
    }
  }

  // Keep provider blocks distinct from missing source data.
  if (health.commonReason === SharedObjectHealthCommonReason.RESOURCE_BLOCKED) {
    return {
      badge: 'Closed',
      title: 'Content unavailable',
      description: 'The provider blocked access to this shared object.',
      hint: 'Retry after the block is lifted.',
    }
  }

  // Explain the missing block and when replication may still recover it.
  if (health.commonReason === SharedObjectHealthCommonReason.BLOCK_NOT_FOUND) {
    return {
      badge: 'Closed',
      title: 'Required block missing',
      description:
        'A block required to mount this shared object could not be found.',
      hint: 'Retry if the data may still be syncing; otherwise the source data needs repair.',
    }
  }

  // Require repair of an unreadable transform configuration.
  if (
    health.commonReason ===
    SharedObjectHealthCommonReason.TRANSFORM_CONFIG_DECODE_FAILED
  ) {
    return {
      badge: 'Closed',
      title: 'Transform configuration invalid',
      description:
        'Alpha could not decode the transform configuration needed to read this content.',
      hint: 'The shared object data needs repair before it can be opened.',
    }
  }

  // Explain why unreadable body metadata needs repair or a compatible viewer.
  if (
    health.commonReason ===
    SharedObjectHealthCommonReason.BODY_CONFIG_DECODE_FAILED
  ) {
    return {
      badge: 'Closed',
      title: 'Body configuration invalid',
      description:
        'The shared object body metadata could not be decoded into a supported view.',
      hint: 'The body metadata needs repair or a compatible viewer.',
    }
  }

  // Keep partial availability distinct from a closed mount.
  if (health.status === SharedObjectHealthStatus.DEGRADED) {
    return {
      badge: 'Degraded',
      title: 'Shared object degraded',
      description:
        'The shared object is partially available, but Alpha detected a recoverable problem.',
      hint: '',
    }
  }

  // Describe the failed layer when no more specific recovery reason is available.
  return {
    badge: 'Closed',
    title:
      health.layer === SharedObjectHealthLayer.BODY
        ? 'Shared object body failed'
        : 'Shared object unavailable',
    description:
      health.layer === SharedObjectHealthLayer.BODY
        ? 'The shared object opened, but the body content could not be mounted.'
        : 'Alpha could not mount this shared object.',
    hint: '',
  }
}

function getSharedObjectMutationPermission(
  sharedObjectId: string,
  resourcesList: WatchResourcesListResponse | null,
  organizations: OrganizationInfo[],
  organizationsLoading: boolean,
): SharedObjectMutationPermission {
  // Use organization ownership when deciding whether the account may change this object.
  const org = organizations.find(
    (org) => !!org.id && (org.spaceIds?.includes(sharedObjectId) ?? false),
  )
  if (org) {
    if (org.role === ORG_ROLE_OWNER) {
      return { canMutate: true, disabledReason: '' }
    }
    return {
      canMutate: false,
      disabledReason:
        'Only organization owners can repair or reinitialize this shared object.',
    }
  }

  // Allow mutation of a shared object this account created directly.
  const spaceEntry = resourcesList?.spacesList?.find(
    (entry) => entry.entry?.ref?.providerResourceRef?.id === sharedObjectId,
  )
  if (spaceEntry?.entry?.source === 'created') {
    return { canMutate: true, disabledReason: '' }
  }

  // Wait for ownership evidence before offering a shared object repair.
  if (organizationsLoading || !resourcesList) {
    return {
      canMutate: false,
      disabledReason:
        'Alpha is still checking whether this account can repair this shared object.',
    }
  }

  return {
    canMutate: false,
    disabledReason:
      'Only the shared object owner can repair or reinitialize this shared object.',
  }
}

function getSharedObjectHealthTone(
  isLoading: boolean,
  isDegraded: boolean,
): {
  cardBorder: stylex.StyleXStyles
  cardBackground: stylex.StyleXStyles
  iconWrap: stylex.StyleXStyles
  iconColor: stylex.StyleXStyles
  badgeTone: stylex.StyleXStyles
  Icon: IconType
} {
  if (isLoading) {
    return {
      cardBorder: stylexStyles5.rule1,
      cardBackground: stylexStyles5.rule2,
      iconWrap: stylexStyles5.rule3,
      iconColor: healthIconStyles['--color-foreground'],
      badgeTone: stylexStyles5.rule4,
      Icon: LuCircleAlert,
    }
  }
  if (isDegraded) {
    return {
      cardBorder: stylexStyles5.rule5,
      cardBackground: stylexStyles5.rule6,
      iconWrap: stylexStyles5.rule7,
      iconColor: healthIconStyles['--color-warning'],
      badgeTone: stylexStyles5.rule8,
      Icon: LuTriangleAlert,
    }
  }
  return {
    cardBorder: stylexStyles5.rule9,
    cardBackground: stylexStyles5.rule10,
    iconWrap: stylexStyles5.rule11,
    iconColor: healthIconStyles['--color-destructive'],
    badgeTone: stylexStyles5.rule12,
    Icon: LuShieldAlert,
  }
}

function RemediationActionButton({
  icon,
  label,
  onClick,
  disabledReason,
  disabled = false,
  active,
  variant,
  sx,
}: {
  icon: ReactNode
  label: string
  onClick: () => void
  disabledReason: string
  disabled?: boolean
  active?: boolean
  variant?: 'active' | 'destructive'
  sx?: stylex.StyleXStyles
}) {
  const button = (
    <DashboardButton
      icon={icon}
      onClick={onClick}
      disabled={disabled || !!disabledReason}
      variant={variant ?? (active ? 'active' : undefined)}
      sx={sx}
    >
      {label}
    </DashboardButton>
  )
  if (!disabledReason) {
    return button
  }
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span sx={stylexStyles4.rule1}>{button}</span>
      </TooltipTrigger>
      <TooltipContent side="top" sx={stylexStyles.rule1}>
        {disabledReason}
      </TooltipContent>
    </Tooltip>
  )
}

function SharedObjectHealthCard({
  health,
  onRetry,
  onRepair,
  onReinitialize,
  onBack,
  mutationPermission,
  mutationPending,
  mutationError,
}: {
  health: SharedObjectHealth
  onRetry: () => void
  onRepair: () => void
  onReinitialize: () => void
  onBack: () => void
  mutationPermission: SharedObjectMutationPermission
  mutationPending: boolean
  mutationError: string
}) {
  // Resolve the health summary and recovery flow for this mount state.
  const summary = getHealthSummary(health)
  const isLoading = health.status === SharedObjectHealthStatus.LOADING
  const isDegraded = health.status === SharedObjectHealthStatus.DEGRADED
  const flow = useRemediationFlow(onRepair, onReinitialize)

  // Keep mounting on the progress screen until health leaves loading.
  if (isLoading) {
    return (
      <SpaceMountingScreen
        stage={spaceMountStageFromHealth(health)}
        detail={summary.description}
        onBack={onBack}
        onRetry={onRetry}
      />
    )
  }

  // Describe a failed or degraded mount with its recovery controls.
  const tone = getSharedObjectHealthTone(isLoading, isDegraded)

  // Layer label appears in error/degraded states. The body vs shared object
  // distinction is internal and never surfaces on the loading screen.
  const layerLabel =
    health.layer === SharedObjectHealthLayer.BODY ? 'Body' : 'Shared Object'
  const badgeLabel = `${summary.badge} - ${layerLabel}`

  return (
    <div sx={stylexStyles4.rule2}>
      <BackButton floating onClick={onBack}>
        Back
      </BackButton>
      <div sx={stylexStyles4.rule3}>
        <div
          sx={[[stylexStyles6.rule1], [tone.cardBackground, tone.cardBorder]]}
        >
          <HealthSummaryHeader
            tone={tone}
            badgeLabel={badgeLabel}
            title={summary.title}
            description={summary.description}
          />

          <div sx={stylexStyles7.rule1}>
            <HealthIssuePanel
              sx={nativeChildSpacing.s3}
              hint={summary.hint}
              detail={health.error?.trim() ?? ''}
            />
            <RemediationPanel
              sx={nativeChildSpacing.s3}
              flow={flow}
              showRetry={
                health.remediationHint ===
                SharedObjectHealthRemediationHint.RETRY
              }
              onRetry={onRetry}
              mutationPermission={mutationPermission}
              mutationPending={mutationPending}
              mutationError={mutationError}
            />
          </div>
        </div>
      </div>
    </div>
  )
}

type SharedObjectHealthTone = ReturnType<typeof getSharedObjectHealthTone>

// HealthSummaryHeader renders the icon, badge, title, and description at the
// top of a shared object health card.
function HealthSummaryHeader({
  tone,
  badgeLabel,
  title,
  description,
}: {
  tone: SharedObjectHealthTone
  badgeLabel: string
  title: string
  description: string
}) {
  return (
    <div sx={stylexStyles4.rule4}>
      <div sx={[[stylexStyles6.rule2], [tone.iconWrap]]}>
        <tone.Icon sx={[healthIconStyles.size, tone.iconColor]} />
      </div>
      <span sx={[[stylexStyles6.rule3], [tone.badgeTone]]}>{badgeLabel}</span>
      <h1 sx={stylexStyles4.rule5}>{title}</h1>
      <p sx={stylexStyles4.rule6}>{description}</p>
    </div>
  )
}

// HealthIssuePanel renders the issue hint and the raw error detail.
function HealthIssuePanel({
  sx,
  hint,
  detail,
}: { hint: string; detail: string } & {
  sx?: stylex.StyleXStyles
  className?: never
}) {
  return (
    <div sx={[stylexStyles4.rule7, sx]}>
      <div sx={stylexStyles4.rule8}>
        <LuCircleAlert sx={stylexStyles2.rule1} />
        <span sx={stylexStyles4.rule9}>Issue</span>
      </div>
      <p sx={stylexStyles4.rule10}>
        {hint ||
          'Review the issue details below before choosing the next step.'}
      </p>
      {detail ? <div sx={stylexStyles4.rule11}>{detail}</div> : null}
    </div>
  )
}

interface RemediationFlow {
  selectedAction: SharedObjectRemediationAction
  confirmingRepair: boolean
  confirmingReinitialize: boolean
  requestRepair: () => void
  requestReinitialize: () => void
  cancelRepair: () => void
  cancelReinitialize: () => void
  confirmRepair: () => void
  confirmReinitialize: () => void
}

// useRemediationFlow tracks which remediation action the user selected and
// which confirmation prompts are open. Confirming an action selects it, closes
// both prompts, and runs the matching callback.
function useRemediationFlow(
  onRepair: () => void,
  onReinitialize: () => void,
): RemediationFlow {
  const [selectedAction, setSelectedAction] =
    useState<SharedObjectRemediationAction>(null)
  const [confirmingRepair, setConfirmingRepair] = useState(false)
  const [confirmingReinitialize, setConfirmingReinitialize] = useState(false)

  return {
    selectedAction,
    confirmingRepair,
    confirmingReinitialize,
    requestRepair: () => setConfirmingRepair(true),
    requestReinitialize: () => setConfirmingReinitialize(true),
    cancelRepair: () => setConfirmingRepair(false),
    cancelReinitialize: () => setConfirmingReinitialize(false),
    confirmRepair: () => {
      setSelectedAction('repair')
      setConfirmingRepair(false)
      setConfirmingReinitialize(false)
      onRepair()
    },
    confirmReinitialize: () => {
      setSelectedAction('reinitialize')
      setConfirmingReinitialize(false)
      onReinitialize()
    },
  }
}

// getRemediationMessage describes the next step for the selected action, or
// for the user's permission when nothing is selected.
function getRemediationMessage(
  selectedAction: SharedObjectRemediationAction,
  canMutate: boolean,
): string {
  if (selectedAction === 'repair') {
    return 'Repair keeps the current shared object identity, but it can rewrite recovery state. Confirm only after checking that the current state is backed up or recoverable.'
  }
  if (selectedAction === 'reinitialize') {
    return 'Reinitialize is destructive. It rewrites the broken shared object in place on the same shared object id and canonical URL.'
  }
  if (canMutate) {
    return 'Choose Repair only after checking the current shared object state. Reinitialize rewrites the shared object in place. You can also go back and decide later.'
  }
  return 'You do not have permission to repair or reinitialize this shared object from the current account. The action set stays visible here so the owner can recover it without losing this route.'
}

// RemediationPanel renders the next-step guidance, the retry, repair, and
// reinitialize actions, their confirmation prompts, and the mutation status.
function RemediationPanel({
  sx,
  flow,
  showRetry,
  onRetry,
  mutationPermission,
  mutationPending,
  mutationError,
}: {
  flow: RemediationFlow
  showRetry: boolean
  onRetry: () => void
  mutationPermission: SharedObjectMutationPermission
  mutationPending: boolean
  mutationError: string
} & { sx?: stylex.StyleXStyles; className?: never }) {
  const { selectedAction } = flow
  const disabledReason = mutationPermission.canMutate
    ? ''
    : mutationPermission.disabledReason

  return (
    <div sx={[stylexStyles4.rule7, sx]}>
      <div sx={stylexStyles4.rule8}>
        <LuArrowRight sx={stylexStyles2.rule1} />
        <span sx={stylexStyles4.rule9}>Next step</span>
      </div>
      <p sx={stylexStyles4.rule10}>
        {getRemediationMessage(selectedAction, mutationPermission.canMutate)}
      </p>
      <div sx={stylexStyles4.rule12}>
        {showRetry ? (
          <DashboardButton
            icon={<LuRotateCcw sx={stylexStyles2.rule2} />}
            onClick={onRetry}
          >
            Retry
          </DashboardButton>
        ) : null}
        <RemediationActionButton
          icon={<LuRefreshCw sx={stylexStyles2.rule2} />}
          label="Repair"
          onClick={flow.requestRepair}
          disabledReason={disabledReason}
          disabled={mutationPending}
          active={selectedAction === 'repair'}
        />
        <RemediationActionButton
          icon={<LuShieldAlert sx={stylexStyles2.rule2} />}
          label="Reinitialize"
          onClick={flow.requestReinitialize}
          disabledReason={disabledReason}
          disabled={mutationPending}
          active={selectedAction === 'reinitialize'}
          variant="destructive"
        />
      </div>
      {flow.confirmingRepair ? (
        <RemediationConfirm
          Icon={LuTriangleAlert}
          title="Confirm repair"
          description="Repair keeps this shared object id and URL, but it can post a replacement root. Continue only if you have checked that the current state is backed up or recoverable."
          pending={mutationPending}
          onCancel={flow.cancelRepair}
          onConfirm={flow.confirmRepair}
        />
      ) : null}
      {flow.confirmingReinitialize ? (
        <RemediationConfirm
          Icon={LuShieldAlert}
          title="Confirm reinitialize"
          description="Reinitialize is destructive. It rewrites this shared object in place on the same shared object id and URL. Use repair first when you want Alpha to retry the normal recovery path without discarding the current state."
          pending={mutationPending}
          onCancel={flow.cancelReinitialize}
          onConfirm={flow.confirmReinitialize}
        />
      ) : null}
      {selectedAction ? (
        <p sx={stylexStyles4.rule13}>
          {selectedAction === 'repair'
            ? 'Repair is selected for this broken shared object.'
            : 'Reinitialize is selected for this broken shared object.'}
        </p>
      ) : null}
      {mutationError ? <p sx={stylexStyles4.rule14}>{mutationError}</p> : null}
    </div>
  )
}

// RemediationConfirm renders a destructive confirmation prompt whose header and
// confirm button share Icon.
function RemediationConfirm({
  Icon,
  title,
  description,
  pending,
  onCancel,
  onConfirm,
}: {
  Icon: IconType
  title: string
  description: string
  pending: boolean
  onCancel: () => void
  onConfirm: () => void
}) {
  return (
    <div sx={stylexStyles4.rule15}>
      <div sx={stylexStyles4.rule8}>
        <Icon sx={stylexStyles3.rule1} />
        <span sx={stylexStyles4.rule16}>{title}</span>
      </div>
      <p sx={stylexStyles4.rule10}>{description}</p>
      <div sx={stylexStyles4.rule12}>
        <DashboardButton
          icon={<LuRotateCcw sx={stylexStyles2.rule2} />}
          onClick={onCancel}
        >
          Cancel
        </DashboardButton>
        <DashboardButton
          icon={<Icon sx={stylexStyles3.rule2} />}
          onClick={onConfirm}
          disabled={pending}
          variant="destructive"
        >
          {title}
        </DashboardButton>
      </div>
    </div>
  )
}

type SessionNavigate = ReturnType<typeof useSessionNavigate>

type MountedSharedObject = ReturnType<typeof useMountedSharedObject>
type SharedObjectResource = MountedSharedObject['sharedObjectResource']
type SharedObjectBodyResource = MountedSharedObject['sharedObjectBodyResource']

// useSpaceOrgRedirect redirects /u/:idx/so/:spaceId to
// /u/:idx/org/:orgId/so/:spaceId when the space is org-owned. It skips the
// redirect when the route is already nested under /org/.
function useSpaceOrgRedirect(sharedObjectId: string) {
  // Read the current route and organization context for this shared object.
  const navigateSession = useSessionNavigate()
  const parentPaths = useParentPaths()
  const orgListCtx = SpacewaveOrgListContext.useContextSafe()

  // Find the owning organization when the current route is outside it.
  const orgRedirectId = useMemo(() => {
    // Avoid redirecting a missing object or a route already under an organization.
    if (!sharedObjectId) return ''
    const underOrg = parentPaths.some((p) => p.includes('/org/'))
    if (underOrg) return ''
    const orgs = orgListCtx?.organizations ?? []
    for (const org of orgs) {
      if (!org.id || !org.spaceIds) continue
      const spaceIds = new Set(org.spaceIds)
      if (spaceIds.has(sharedObjectId)) return org.id
    }
    return ''
  }, [orgListCtx, parentPaths, sharedObjectId])

  // Replace a personal object route with its owning organization route.
  useEffect(() => {
    if (!orgRedirectId) return
    navigateSession({
      path: `org/${orgRedirectId}/so/${sharedObjectId}`,
      replace: true,
    })
  }, [navigateSession, orgRedirectId, sharedObjectId])
}

// useMountedSharedObject mounts the shared object and then its body through
// the shared mounts, so tabs on the same shared object share one mount. A
// shared object that is not found redirects to the session root.
function useMountedSharedObject(
  session: Resource<Session>,
  sharedObjectId: string,
  navigateSession: SessionNavigate,
) {
  // Mount the shared object before mounting its body resource.
  const sharedObjectResource = useSharedObjectMount(session, sharedObjectId)
  const sharedObjectBodyResource =
    useSharedObjectBodyMount(sharedObjectResource)

  // Leave a shared object the session does not have.
  const sharedObjectMissing =
    !!session.value &&
    !!sharedObjectId &&
    !sharedObjectResource.loading &&
    !sharedObjectResource.error &&
    !sharedObjectResource.value
  useEffect(() => {
    if (!sharedObjectMissing) return
    console.warn(
      'mount shared object returned not found, redirecting to session',
      sharedObjectId,
    )
    navigateSession({ path: '', replace: true })
    // The outer shell router recreates navigate functions as the current
    // path changes; only a new miss should redirect.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sharedObjectId, sharedObjectMissing])

  return { sharedObjectResource, sharedObjectBodyResource }
}

// useMissingSpaceGuard reports whether a mounted shared object is absent from
// the published resources list and should redirect to the session root.
//
// Quickstart can open this route before WatchResourcesList publishes the new
// Space. Keep the guard event-driven; background tabs throttle timers, so a
// timeout would reintroduce the false redirect.
function useMissingSpaceGuard(
  sharedObjectId: string,
  mounted: boolean,
  resourcesList: WatchResourcesListResponse | null | undefined,
): boolean {
  // Identify this session's quickstart route within the running app instance.
  const environment = useAppEnvironment()
  const sessionIndex = useSessionIndex()
  const instanceKey = environment.instanceKey

  // Retain the quickstart route while this container owns it.
  useEffect(
    () => retainQuickstartRoute(sessionIndex, sharedObjectId, instanceKey),
    [sessionIndex, sharedObjectId, instanceKey],
  )

  // Check whether the normal resources watch has caught up with the new space.
  const inResourcesList = useMemo(
    () =>
      resourcesList?.spacesList?.some(
        (entry) => entry.entry?.ref?.providerResourceRef?.id === sharedObjectId,
      ) ?? false,
    [resourcesList, sharedObjectId],
  )

  // Clear quickstart's temporary routing allowance when the space is listed.
  useEffect(() => {
    if (inResourcesList) {
      clearQuickstartRouteAwaitingResourcesList(
        sessionIndex,
        sharedObjectId,
        instanceKey,
      )
    }
  }, [sessionIndex, inResourcesList, sharedObjectId, instanceKey])

  // Wait for the resources watch before treating a new quickstart space as missing.
  const awaitingResourcesList =
    !inResourcesList &&
    isQuickstartRouteAwaitingResourcesList(
      sessionIndex,
      sharedObjectId,
      instanceKey,
    )

  return (
    mounted && !!resourcesList && !awaitingResourcesList && !inResourcesList
  )
}

// useSharedObjectRemediation owns the repair and reinitialize mutations, the
// credential repair step-up, and the self-enrollment step-up for a shared
// object. Both step-ups retry the mount after they succeed.
function useSharedObjectRemediation(
  sharedObjectId: string,
  sessionValue: Session | null | undefined,
  onRetry: () => void,
) {
  // Keep remediation requests and credential dialogs tied to the current account.
  const { providerId, accountId } = useSessionInfo(sessionValue)
  const selfEnrollmentStatus = useSessionSelfEnrollmentStatus()
  const [mutationPending, setMutationPending] = useState(false)
  const [mutationError, setMutationError] = useState('')
  const [credentialRepairOpen, setCredentialRepairOpen] = useState(false)
  const [selfEnrollmentStepUpOpen, setSelfEnrollmentStepUpOpen] =
    useState(false)
  const [selfEnrollmentAutoOpenKey, setSelfEnrollmentAutoOpenKey] = useState('')

  // Require step-up only for the enrolled shared objects named by the watched status.
  const needsSelfEnrollmentStepUp = useMemo(
    () =>
      !!sharedObjectId &&
      selfEnrollmentStatus.credentialRequired &&
      (selfEnrollmentStatus.snapshot?.sharedObjectIds?.includes(
        sharedObjectId,
      ) ??
        false),
    [
      selfEnrollmentStatus.credentialRequired,
      selfEnrollmentStatus.snapshot?.sharedObjectIds,
      sharedObjectId,
    ],
  )

  // Open the step-up once per enrollment generation and shared object.
  const autoOpenKey = needsSelfEnrollmentStepUp
    ? `${selfEnrollmentStatus.generationKey}:${sharedObjectId}`
    : ''
  if (autoOpenKey && autoOpenKey !== selfEnrollmentAutoOpenKey) {
    setSelfEnrollmentAutoOpenKey(autoOpenKey)
    setSelfEnrollmentStepUpOpen(true)
  }

  // Run the selected recovery action through the session's shared object service.
  const runRepairAction = useCallback(
    async (kind: SharedObjectRemediationAction) => {
      if (!sharedObjectId || !sessionValue || mutationPending) {
        return
      }
      setMutationPending(true)
      setMutationError('')
      try {
        if (kind === 'repair') {
          const resp =
            await sessionValue.spacewave.repairSharedObject(sharedObjectId)
          if (resp.credentialRequired) {
            if (providerId === 'spacewave' && !!accountId) {
              setCredentialRepairOpen(true)
            } else {
              setMutationError(credentialRequiredMessage)
            }
            return
          }
        } else if (kind === 'reinitialize') {
          await sessionValue.spacewave.reinitializeSharedObject(sharedObjectId)
        }
        onRetry()
      } catch (err) {
        setMutationError(err instanceof Error ? err.message : 'Action failed')
      } finally {
        setMutationPending(false)
      }
    },
    [
      accountId,
      onRetry,
      mutationPending,
      providerId,
      sessionValue,
      sharedObjectId,
    ],
  )

  // Retry credential repair after the account key has been unlocked.
  const confirmCredentialRepair = useCallback(async () => {
    if (!sharedObjectId || !sessionValue) {
      return
    }
    setMutationPending(true)
    setMutationError('')
    try {
      const resp =
        await sessionValue.spacewave.repairSharedObject(sharedObjectId)
      if (resp.credentialRequired) {
        throw new Error(credentialRequiredMessage)
      }
      setCredentialRepairOpen(false)
      onRetry()
    } catch (err) {
      setMutationError(err instanceof Error ? err.message : 'Action failed')
      throw err
    } finally {
      setMutationPending(false)
    }
  }, [onRetry, sessionValue, sharedObjectId])

  // Complete the pending enrollment step-up before retrying the mount.
  const confirmSelfEnrollmentStepUp = useCallback(async () => {
    if (!selfEnrollmentStatus.resource) return
    setMutationPending(true)
    setMutationError('')
    try {
      await selfEnrollmentStatus.resource.start()
      setSelfEnrollmentStepUpOpen(false)
      onRetry()
    } catch (err) {
      setMutationError(err instanceof Error ? err.message : 'Action failed')
      throw err
    } finally {
      setMutationPending(false)
    }
  }, [onRetry, selfEnrollmentStatus.resource])

  return {
    providerId,
    accountId,
    mutationPending,
    mutationError,
    credentialRepairOpen,
    setCredentialRepairOpen,
    selfEnrollmentStepUpOpen,
    setSelfEnrollmentStepUpOpen,
    needsSelfEnrollmentStepUp,
    runRepairAction,
    confirmCredentialRepair,
    confirmSelfEnrollmentStepUp,
  }
}

// StepUpDialog renders an AuthConfirmDialog that unlocks an account key before
// a shared object action.
function StepUpDialog({
  account,
  open,
  onOpenChange,
  title,
  description,
  confirmLabel,
  intentDescription,
  onConfirm,
}: {
  account: ComponentProps<typeof AccountDashboardStateProvider>['account']
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description: string
  confirmLabel: string
  intentDescription: string
  onConfirm: () => Promise<void>
}) {
  return (
    <AccountDashboardStateProvider account={account}>
      <AuthConfirmDialog
        open={open}
        onOpenChange={onOpenChange}
        title={title}
        description={description}
        confirmLabel={confirmLabel}
        intent={{
          kind: AccountEscalationIntentKind.AccountEscalationIntentKind_ACCOUNT_ESCALATION_INTENT_KIND_UNSPECIFIED,
          title,
          description: intentDescription,
        }}
        onConfirm={onConfirm}
        account={account}
        retainAfterClose
      />
    </AccountDashboardStateProvider>
  )
}

// SharedObjectDebugInfo renders the mount state of the shared object and its
// body in the debug overlay.
function SharedObjectDebugInfo({
  sharedObjectId,
  sharedObjectResource,
  sharedObjectBodyResource,
}: {
  sharedObjectId: string
  sharedObjectResource: SharedObjectResource
  sharedObjectBodyResource: SharedObjectBodyResource
}) {
  return (
    <DebugInfo>
      Shared Object ID: {sharedObjectId}
      <br />
      Loading: {sharedObjectResource.loading.toString()}
      <br />
      Shared object loaded: {(!!sharedObjectResource.value).toString()}
      <br />
      Error: {sharedObjectResource.error?.toString() ?? 'none'}
      <br />
      Meta:{' '}
      <pre>
        {sharedObjectResource.value
          ? JSON.stringify(
              MountSharedObjectResponse.toJson(sharedObjectResource.value.meta),
              null,
              4,
            )
          : 'none'}
      </pre>
      Shared object body loaded: {(!!sharedObjectBodyResource.value).toString()}
      <br />
      Body loading: {sharedObjectBodyResource.loading.toString()}
      <br />
      Body error: {sharedObjectBodyResource.error?.toString() ?? 'none'}
    </DebugInfo>
  )
}

// SharedObjectRouteState renders the state the route is in: the mounted shared
// object, a step-up prompt, a DMCA block, a health card, or the mounting screen.
function SharedObjectRouteState({
  mounted,
  needsSelfEnrollmentStepUp,
  onStepUp,
  isBlocked,
  resourceError,
  activeHealth,
  healthResp,
  onRetry,
  onRepair,
  onReinitialize,
  onBack,
  mutationPermission,
  mutationPending,
  mutationError,
}: {
  mounted: boolean
  needsSelfEnrollmentStepUp: boolean
  onStepUp: () => void
  isBlocked: boolean
  resourceError: Error | null | undefined
  activeHealth: SharedObjectHealth | null | undefined
  healthResp: WatchSharedObjectHealthResponse | null | undefined
  onRetry: () => void
  onRepair: () => void
  onReinitialize: () => void
  onBack: () => void
  mutationPermission: SharedObjectMutationPermission
  mutationPending: boolean
  mutationError: string
}) {
  // Resolve the policy link for an unavailable shared object.
  const dmcaHref = useStaticHref('/dmca')

  // Render mounted contents or the corresponding unavailable state.
  if (mounted) {
    return (
      <>
        <SharedObjectSyncNotice health={healthResp?.health} />
        <SharedObjectBodyContainer />
      </>
    )
  }
  if (needsSelfEnrollmentStepUp) {
    return (
      <ErrorState
        variant="fullscreen"
        title="Unlock to open this Space"
        message="This Space needs your account key so this session can be connected before opening it."
        onRetry={onStepUp}
      />
    )
  }
  if (isBlocked) {
    return (
      <ErrorState
        variant="fullscreen"
        title="Content Unavailable"
        message="This content has been disabled due to a DMCA takedown notice. If you believe this is an error, you can file a counter-notice."
        onRetry={onRetry}
      >
        <a href={dmcaHref} sx={stylexStyles4.rule17}>
          DMCA Policy
        </a>
      </ErrorState>
    )
  }
  const health = resourceError
    ? (activeHealth ??
      buildSharedObjectFallbackHealth(
        resourceError,
        SharedObjectHealthLayer.SHARED_OBJECT,
      ))
    : activeHealth
  if (!health) {
    return (
      <SpaceMountingScreen
        stage="resolve"
        detail="Looking up the shared object."
        onBack={onBack}
      />
    )
  }
  return (
    <SharedObjectHealthCard
      health={health}
      onRetry={onRetry}
      onRepair={onRepair}
      onReinitialize={onReinitialize}
      onBack={onBack}
      mutationPermission={mutationPermission}
      mutationPending={mutationPending}
      mutationError={mutationError}
    />
  )
}

// SessionSharedObjectContainer displays a shared object.
export function SessionSharedObjectContainer() {
  // Resolve the routed shared object and the current session's contexts.
  const params = useParams()
  const sharedObjectId = params['sharedObjectId'] ?? ''
  const navigate = useNavigate()
  const navigateSession = useSessionNavigate()
  const session = SessionContext.useContext()
  const sessionValue = useResourceValue(session)
  const orgListCtx = SpacewaveOrgListContext.useContextSafe()

  // Use the owning organization's route when one is available.
  useSpaceOrgRedirect(sharedObjectId)

  // Watch the session's accessible spaces through the existing shared stream.
  const resourcesList = useWatchStateRpc(
    useCallback(
      (req: WatchResourcesListRequest, signal: AbortSignal) =>
        sessionValue?.watchResourcesList(req, signal) ?? null,
      [sessionValue],
    ),
    {},
    WatchResourcesListRequest.equals,
    WatchResourcesListResponse.equals,
  )

  // Watch this shared object's reported health beside its mounted resources.
  const sharedObjectHealthResp = useWatchStateRpc(
    useCallback(
      (req: WatchSharedObjectHealthRequest, signal: AbortSignal) =>
        sessionValue?.watchSharedObjectHealth(req, signal) ?? null,
      [sessionValue],
    ),
    { sharedObjectId },
    WatchSharedObjectHealthRequest.equals,
    WatchSharedObjectHealthResponse.equals,
  )

  // Mount the shared object and body through their existing resource owners.
  const { sharedObjectResource, sharedObjectBodyResource } =
    useMountedSharedObject(session, sharedObjectId, navigateSession)

  // Guard a missing space while allowing an unfinished quickstart listing.
  const shouldRedirectMissingSpace = useMissingSpaceGuard(
    sharedObjectId,
    !!sharedObjectResource.value,
    resourcesList,
  )

  // Use the first mount failure reported by the object or its body.
  const resourceError =
    sharedObjectResource.error ?? sharedObjectBodyResource.error

  // Reconcile watched health with the current object and body mount states.
  const activeHealth = useMemo(() => {
    return getSharedObjectRouteHealth({
      mounted: !!sharedObjectResource.value,
      bodyLoading: sharedObjectBodyResource.loading,
      watchedHealth: sharedObjectHealthResp?.health,
      mountError: sharedObjectResource.error,
      bodyError: sharedObjectBodyResource.error,
    })
  }, [
    sharedObjectBodyResource.error,
    sharedObjectBodyResource.loading,
    sharedObjectHealthResp?.health,
    sharedObjectResource.error,
    sharedObjectResource.value,
  ])

  // Distinguish provider blocks from ordinary mount failures.
  const isBlocked =
    activeHealth?.commonReason ===
    SharedObjectHealthCommonReason.RESOURCE_BLOCKED

  // Open storage recovery when the mount reports a quota incident.
  if (resourceError && isStorageQuotaError(resourceError)) {
    queueMicrotask(() =>
      navigateSession({
        path: 'settings/storage/recovery/incident',
        replace: true,
      }),
    )
  } else if (shouldRedirectMissingSpace) {
    queueMicrotask(() => navigateSession({ path: '', replace: true }))
  }

  // Remount the shared object, which remounts its body, unless only the body
  // failed.
  const handleRetry = useCallback(() => {
    if (sharedObjectBodyResource.error && !sharedObjectResource.error) {
      sharedObjectBodyResource.retry()
      return
    }
    sharedObjectResource.retry()
  }, [sharedObjectBodyResource, sharedObjectResource])

  // Return to the parent route from this shared object.
  const handleBack = useCallback(() => {
    navigate({ path: '../' })
  }, [navigate])

  // Bind remediation and account unlocking to the selected shared object.
  const remediation = useSharedObjectRemediation(
    sharedObjectId,
    sessionValue,
    handleRetry,
  )
  const { runRepairAction } = remediation
  const accountResource = useMountAccount(
    remediation.providerId,
    remediation.accountId,
  )

  // Resolve whether this account may mutate the unavailable shared object.
  const mutationPermission = useMemo(
    () =>
      getSharedObjectMutationPermission(
        sharedObjectId,
        resourcesList,
        orgListCtx?.organizations ?? [],
        orgListCtx?.loading ?? false,
      ),
    [
      orgListCtx?.loading,
      orgListCtx?.organizations,
      resourcesList,
      sharedObjectId,
    ],
  )

  return (
    <SharedObjectContext.Provider resource={sharedObjectResource}>
      <SharedObjectBodyContext.Provider resource={sharedObjectBodyResource}>
        <DebugInfoProvider>
          <SessionFrame>
            <SharedObjectDebugInfo
              sharedObjectId={sharedObjectId}
              sharedObjectResource={sharedObjectResource}
              sharedObjectBodyResource={sharedObjectBodyResource}
            />
            <SharedObjectRouteState
              mounted={
                !!sharedObjectResource.value && !!sharedObjectBodyResource.value
              }
              needsSelfEnrollmentStepUp={remediation.needsSelfEnrollmentStepUp}
              onStepUp={() => remediation.setSelfEnrollmentStepUpOpen(true)}
              isBlocked={isBlocked}
              resourceError={resourceError}
              activeHealth={activeHealth}
              healthResp={sharedObjectHealthResp}
              onRetry={handleRetry}
              onRepair={() => void runRepairAction('repair')}
              onReinitialize={() => void runRepairAction('reinitialize')}
              onBack={handleBack}
              mutationPermission={mutationPermission}
              mutationPending={remediation.mutationPending}
              mutationError={remediation.mutationError}
            />
            {remediation.credentialRepairOpen ? (
              <StepUpDialog
                account={accountResource}
                open={remediation.credentialRepairOpen}
                onOpenChange={remediation.setCredentialRepairOpen}
                title="Unlock shared object recovery"
                description="Unlock an account key to grant this session access before attempting shared object repair. Repair may post a replacement root, so continue only after checking that the current state is backed up or recoverable."
                confirmLabel="Continue repair"
                intentDescription="Unlock an account key to grant this session access before attempting shared object repair."
                onConfirm={remediation.confirmCredentialRepair}
              />
            ) : null}
            {remediation.needsSelfEnrollmentStepUp ? (
              <StepUpDialog
                account={accountResource}
                open={remediation.selfEnrollmentStepUpOpen}
                onOpenChange={remediation.setSelfEnrollmentStepUpOpen}
                title="Unlock Space access"
                description="This Space needs your account key so this session can be connected before opening it."
                confirmLabel="Unlock and open Space"
                intentDescription="Unlock an account key so this session can connect to this Space."
                onConfirm={remediation.confirmSelfEnrollmentStepUp}
              />
            ) : null}
          </SessionFrame>
        </DebugInfoProvider>
      </SharedObjectBodyContext.Provider>
    </SharedObjectContext.Provider>
  )
}

const healthIconStyles = stylex.create({
  size: { height: 24, width: 24 },
  '--color-foreground': { color: themeColors['--color-foreground'] },
  '--color-warning': { color: themeColors['--color-warning'] },
  '--color-destructive': { color: themeColors['--color-destructive'] },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
  },
})
