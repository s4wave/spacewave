import { useCallback, useId, useState, type ReactNode } from 'react'
import {
  LuBuilding2,
  LuCircleAlert,
  LuLogOut,
  LuUsers,
  LuLink,
  LuPencil,
  LuPlus,
  LuRefreshCw,
  LuSave,
  LuSettings,
  LuShieldAlert,
  LuTriangleAlert,
  LuFingerprint,
  LuUserPlus,
  LuX,
} from '@s4wave/web/images/icons.js'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'

import {
  SharedObjectHealthCommonReason,
  SharedObjectHealthStatus,
  type SharedObjectHealth,
} from '@s4wave/core/sobject/sobject.pb.js'
import type {
  OrganizationRootStateInfo,
  OrgInviteInfo,
  OrgMemberInfo,
  SharedObjectMutationPermission,
  WatchOrganizationStateResponse,
} from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import type { Session } from '@s4wave/sdk/session/session.js'
import {
  SessionContext,
  useSessionNavigate,
} from '@s4wave/web/contexts/contexts.js'
import { cn } from '@s4wave/web/style/utils.js'
import {
  CollapsibleSection,
  collapsibleSectionActionStyles,
} from '@s4wave/web/ui/CollapsibleSection.js'
import { CopyableField } from '@s4wave/web/ui/CopyableField.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@s4wave/web/ui/tooltip.js'
import { useStateAtom, useStateNamespace } from '@s4wave/web/state/persist.js'
import { toast } from '@s4wave/web/ui/toaster.js'

import { ORG_ROLE_OWNER } from './org-constants.js'
import { OrgMemberList } from './OrgMemberList.js'
import { OrgInviteSection } from './OrgInviteSection.js'
import { OrgActionsSection } from './OrgActionsSection.js'
import { OrgBillingSection } from './OrgBillingSection.js'

import * as stylex from '@stylexjs/stylex'

import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: { display: 'inline-flex' },
  rule2: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule3: { minWidth: 0, flex: 1 },
  rule4: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule5: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 65%, transparent)`,
  },
  rule6: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeLayout['--text-metadata'],
    lineHeight: `var(--tw-leading, ${themeLayout['--text-metadata--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule7: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: '.65rem',
    lineHeight: 1.5,
    overflowWrap: 'break-word',
    whiteSpace: 'pre-wrap',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule8: {
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule9: {
    fontSize: '.58rem',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-widest)',
    letterSpacing: 'var(--tracking-widest)',
    color: `color-mix(in oklab, ${themeColors['--color-destructive']} 80%, transparent)`,
    textTransform: 'uppercase',
  },
  rule10: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule11: {
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'flex',
    flexWrap: 'wrap',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule12: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 20%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule13: {
    fontSize: '.58rem',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-widest)',
    letterSpacing: 'var(--tracking-widest)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 45%, transparent)`,
    textTransform: 'uppercase',
  },
  rule14: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule15: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 55%, transparent)`,
  },
  rule16: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule17: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule18: {
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 20%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule19: {
    marginBottom: `calc(${themeLayout['--spacing']} * 1.5)`,
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule20: { display: 'flex', gap: `calc(${themeLayout['--spacing']} * 2)` },
  rule21: {
    minWidth: 0,
    flex: 1,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    },
  },
  rule22: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule23: {
    width: '100%',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    },
  },
  rule24: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule25: {
    minWidth: 0,
    flex: 1,
    cursor: 'text',
    textAlign: 'left',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: {
      default: themeColors['--color-foreground'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground-alt'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule26: {
    marginBottom: themeLayout['--spacing'],
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule27: {
    display: 'flex',
    minHeight: `calc(${themeLayout['--spacing']} * 9)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule28: {
    display: 'flex',
    minWidth: 0,
    flex: 1,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule29: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '.25rem',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-brand'],
  },
  rule30: {
    minWidth: 0,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
  },
  rule31: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-normal)',
    fontWeight: 'var(--font-weight-normal)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule32: {
    display: 'flex',
    flexShrink: 0,
    flexWrap: 'wrap',
    justifyContent: 'flex-end',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule33: {
    display: { default: 'none', '@media (min-width: 48rem)': 'inline' },
  },
  rule34: {
    display: 'flex',
    height: '100%',
    width: '100%',
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: themeColors['--color-background-primary'],
    padding: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule35: { width: '100%', maxWidth: 'var(--container-sm)' },
  rule36: {
    display: 'flex',
    height: '100%',
    width: '100%',
    flexDirection: 'column',
    overflow: 'hidden',
    backgroundColor: themeColors['--color-background-primary'],
  },
  rule37: {
    minHeight: 0,
    flex: 1,
    overflow: 'auto',
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-warning'],
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-destructive'],
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule5: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

const stylexStyles = stylex.create({
  rule1: { maxWidth: 'var(--container-xs)' },
})

type OrgOpenSection =
  | 'recovery'
  | 'members'
  | 'invites'
  | 'settings'
  | 'billing'
  | 'identifiers'
  | null

export interface OrganizationDetailsProps {
  orgId: string
  orgState: WatchOrganizationStateResponse | null
  orgName?: string
  degraded?: boolean
  isOwner: boolean
  onCloseClick?: () => void
}

function getRoleLabel(role?: string): string {
  if (role === ORG_ROLE_OWNER) return 'Owner'
  return 'Member'
}

/** orgIdentity resolves the display name and role label of the organization. */
function orgIdentity(
  info: WatchOrganizationStateResponse['organization'],
  fallbackOrgName: string | undefined,
  isOwner: boolean,
): { orgName: string; roleLabel: string } {
  return {
    orgName: info?.displayName || fallbackOrgName || 'Organization',
    roleLabel: getRoleLabel(
      info?.role ?? (isOwner ? ORG_ROLE_OWNER : 'org:member'),
    ),
  }
}

function getRecoverySummary(health?: SharedObjectHealth | null): {
  tone: 'loading' | 'degraded' | 'closed'
  title: string
  description: string
  hint: string
} {
  // Explain why an unavailable organization root leaves the dashboard in degraded mode.
  if (!health) {
    return {
      tone: 'closed',
      title: 'Organization root shared object unavailable',
      description:
        'The organization dashboard is running in degraded mode because the org root shared object could not load.',
      hint: 'Repair retries the normal recovery path. Reinitialize is destructive and rewrites the same shared object id in place.',
    }
  }

  // Show verification while the organization root is still loading.
  if (health.status === SharedObjectHealthStatus.LOADING) {
    return {
      tone: 'loading',
      title: 'Checking organization root shared object',
      description:
        'Alpha is still verifying and mounting the organization root shared object.',
      hint: '',
    }
  }

  // Distinguish rejected initial state from a recoverable mount failure.
  if (
    health.commonReason ===
    SharedObjectHealthCommonReason.INITIAL_STATE_REJECTED
  ) {
    return {
      tone: 'closed',
      title: 'Organization root initial state rejected',
      description:
        'The organization root failed verification, so Alpha kept the dashboard available in degraded mode instead of looping.',
      hint: 'Repair retries owner-side recovery on the current shared object id. Reinitialize discards the broken state and reseeds the same id in place.',
    }
  }

  // Explain missing organization data and its recovery choices.
  if (health.commonReason === SharedObjectHealthCommonReason.BLOCK_NOT_FOUND) {
    return {
      tone: 'closed',
      title: 'Organization root data missing',
      description:
        'A required block for the organization root shared object could not be found.',
      hint: 'Retry if replication may still be catching up. Otherwise repair or reinitialize the organization root.',
    }
  }

  // Keep partially available organization state on the repair path.
  if (health.status === SharedObjectHealthStatus.DEGRADED) {
    return {
      tone: 'degraded',
      title: 'Organization root degraded',
      description:
        'The organization root is partially available, but Alpha detected a recoverable problem.',
      hint: 'Use repair first when you want Alpha to retry the normal recovery path without discarding the current state.',
    }
  }

  return {
    tone: 'closed',
    title: 'Organization root shared object unavailable',
    description:
      'Alpha could not mount the organization root shared object, so the dashboard stays available in degraded mode.',
    hint: 'Repair retries the normal owner recovery path. Reinitialize is destructive and rewrites the same shared object id in place.',
  }
}

function getRecoveryPermission(
  permission: SharedObjectMutationPermission | null | undefined,
  isOwner: boolean,
): SharedObjectMutationPermission {
  if (permission) {
    return permission
  }
  return {
    canRepair: isOwner,
    canReinitialize: isOwner,
    disabledReason: isOwner
      ? ''
      : 'Only organization owners can repair or reinitialize this shared object.',
  }
}

function RecoveryActionButton({
  label,
  icon,
  onClick,
  disabled,
  disabledReason,
  destructive = false,
}: {
  label: string
  icon: ReactNode
  onClick: () => void
  disabled: boolean
  disabledReason: string
  destructive?: boolean
}) {
  const button = (
    <DashboardButton
      icon={icon}
      onClick={onClick}
      disabled={disabled || !!disabledReason}
      variant={destructive ? 'destructive' : undefined}
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
        <span sx={stylexStyles3.rule1}>{button}</span>
      </TooltipTrigger>
      <TooltipContent side="top" sx={stylexStyles.rule1}>
        {disabledReason}
      </TooltipContent>
    </Tooltip>
  )
}

/** SectionProps is the open state a CollapsibleSection needs. */
interface SectionProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

/** SectionPropsFor returns the open state of the named section. */
type SectionPropsFor = (section: Exclude<OrgOpenSection, null>) => SectionProps

/**
 * tolerateNotFound runs a removal and reports a not-found failure as already
 * done, since the watched org state will drop the entry on its own.
 */
async function tolerateNotFound(
  run: () => Promise<unknown>,
  alreadyDoneMessage: string,
) {
  try {
    await run()
  } catch (err) {
    const msg = err instanceof Error ? err.message : ''
    if (msg.toLowerCase().includes('not found')) {
      toast.info(alreadyDoneMessage, { duration: 3000 })
      return
    }
    throw err
  }
}

/** recoveryToneClasses maps a recovery tone to its card and icon colors. */
const recoveryToneClasses = {
  loading: {
    card: 'border-foreground/8 bg-background-card/30',
    icon: 'bg-foreground/5',
  },
  degraded: {
    card: 'border-warning/20 bg-warning/5',
    icon: 'bg-warning/10',
  },
  closed: {
    card: 'border-destructive/20 bg-destructive/5',
    icon: 'bg-destructive/10',
  },
} as const

type RecoverySummary = ReturnType<typeof getRecoverySummary>

/** RecoveryToneIcon renders the icon for a recovery tone. */
function RecoveryToneIcon({ tone }: { tone: RecoverySummary['tone'] }) {
  if (tone === 'loading') return <Spinner variant="foreground" />
  if (tone === 'degraded') {
    return <LuTriangleAlert sx={stylexStyles2.rule1} />
  }
  return <LuShieldAlert sx={stylexStyles2.rule2} />
}

/** RecoverySummaryCard renders the health summary of the org root object. */
function RecoverySummaryCard({
  summary,
  healthError,
}: {
  summary: RecoverySummary
  healthError?: string
}) {
  const classes = recoveryToneClasses[summary.tone]
  return (
    <div
      className={
        /* TODO(stylex): dynamic class or forwarded className */ cn(
          'rounded-md border px-3 py-2',
          classes.card,
        )
      }
    >
      <div sx={stylexStyles3.rule2}>
        <div
          className={
            /* TODO(stylex): dynamic class or forwarded className */ cn(
              'flex size-7 shrink-0 items-center justify-center rounded-md',
              classes.icon,
            )
          }
        >
          <RecoveryToneIcon tone={summary.tone} />
        </div>
        <div sx={stylexStyles3.rule3}>
          <p sx={stylexStyles3.rule4}>{summary.title}</p>
          <p sx={stylexStyles3.rule5}>{summary.description}</p>
          {summary.hint && <p sx={stylexStyles3.rule6}>{summary.hint}</p>}
          {healthError && <div sx={stylexStyles3.rule7}>{healthError}</div>}
        </div>
      </div>
    </div>
  )
}

/** useRecoveryAction runs repair or reinitialize on the org root object. */
function useRecoveryAction(session: Session | null, sharedObjectId: string) {
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')

  const run = useCallback(
    async (kind: 'repair' | 'reinitialize') => {
      if (!session || !sharedObjectId || pending) return
      setPending(true)
      setError('')
      try {
        if (kind === 'repair') {
          const resp =
            await session.spacewave.repairSharedObject(sharedObjectId)
          if (resp.credentialRequired) {
            setError('Shared object recovery requires entity credentials')
          }
        } else {
          await session.spacewave.reinitializeSharedObject(sharedObjectId)
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Action failed')
      } finally {
        setPending(false)
      }
    },
    [pending, sharedObjectId, session],
  )

  return { pending, error, run }
}

/** ReinitializeConfirm asks the owner to confirm the destructive reinitialize. */
function ReinitializeConfirm({
  pending,
  onCancel,
  onConfirm,
}: {
  pending: boolean
  onCancel: () => void
  onConfirm: () => void
}) {
  return (
    <div sx={stylexStyles3.rule8}>
      <div sx={stylexStyles3.rule9}>Confirm Reinitialize</div>
      <p sx={stylexStyles3.rule10}>
        Reinitialize is destructive. It rewrites the organization root shared
        object in place on the same shared object id and canonical org route.
      </p>
      <div sx={stylexStyles3.rule11}>
        <DashboardButton
          icon={<LuX sx={stylexStyles2.rule3} />}
          onClick={onCancel}
        >
          Cancel
        </DashboardButton>
        <DashboardButton
          icon={<LuShieldAlert sx={stylexStyles2.rule3} />}
          onClick={onConfirm}
          disabled={pending}
          variant="destructive"
        >
          {pending ? 'Reinitializing…' : 'Confirm reinitialize'}
        </DashboardButton>
      </div>
    </div>
  )
}

/** disabledReasonFor returns the reason an action is blocked, or ''. */
function disabledReasonFor(
  allowed: boolean | undefined,
  permission: SharedObjectMutationPermission,
): string {
  return allowed ? '' : (permission.disabledReason ?? '')
}

/** RecoveryRemediation renders the repair and reinitialize controls. */
function RecoveryRemediation({
  sx,
  session,
  sharedObjectId,
  permission,
}: {
  session: Session | null
  sharedObjectId: string
  permission: SharedObjectMutationPermission
} & { sx?: stylex.StyleXStyles; className?: never }) {
  const { pending, error, run } = useRecoveryAction(session, sharedObjectId)
  const [confirming, setConfirming] = useState(false)

  return (
    <div sx={[stylexStyles3.rule12, sx]}>
      <div sx={stylexStyles3.rule13}>Remediation</div>
      <p sx={stylexStyles3.rule5}>
        This works in place on the canonical organization root shared object at
        the current org-owned id.
      </p>
      <div sx={stylexStyles3.rule11}>
        <RecoveryActionButton
          label={pending ? 'Repairing...' : 'Repair'}
          icon={<LuRefreshCw sx={stylexStyles2.rule3} />}
          onClick={() => {
            setConfirming(false)
            void run('repair')
          }}
          disabled={pending}
          disabledReason={disabledReasonFor(permission.canRepair, permission)}
        />
        <RecoveryActionButton
          label="Reinitialize"
          icon={<LuShieldAlert sx={stylexStyles2.rule3} />}
          onClick={() => setConfirming(true)}
          disabled={pending}
          disabledReason={disabledReasonFor(
            permission.canReinitialize,
            permission,
          )}
          destructive={true}
        />
      </div>
      {confirming && (
        <ReinitializeConfirm
          pending={pending}
          onCancel={() => setConfirming(false)}
          onConfirm={() => {
            setConfirming(false)
            void run('reinitialize')
          }}
        />
      )}
      {error && <p sx={stylexStyles3.rule14}>{error}</p>}
      <p sx={stylexStyles3.rule15}>Shared object ID: {sharedObjectId}</p>
    </div>
  )
}

/** OrgRecoverySection renders the degraded-mode recovery tools. */
function OrgRecoverySection({
  orgId,
  rootState,
  isOwner,
  open,
  onOpenChange,
}: SectionProps & {
  orgId: string
  rootState?: OrganizationRootStateInfo
  isOwner: boolean
}) {
  const session = SessionContext.useContext().value

  return (
    <CollapsibleSection
      title="Recovery"
      icon={<LuCircleAlert sx={stylexStyles2.rule3} />}
      open={open}
      onOpenChange={onOpenChange}
    >
      <InfoCard>
        <div
          className={
            /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ 'space-y-3'
          }
        >
          <RecoverySummaryCard
            summary={getRecoverySummary(rootState?.health)}
            healthError={rootState?.health?.error}
          />
          <RecoveryRemediation
            session={session}
            sharedObjectId={rootState?.sharedObjectId || orgId}
            permission={getRecoveryPermission(
              rootState?.mutationPermission,
              isOwner,
            )}
          />
        </div>
      </InfoCard>
    </CollapsibleSection>
  )
}

/** OrgMembersSection lists the members and lets owners remove them. */
function OrgMembersSection({
  orgId,
  members,
  isOwner,
  open,
  onOpenChange,
}: SectionProps & {
  orgId: string
  members: OrgMemberInfo[]
  isOwner: boolean
}) {
  const session = SessionContext.useContext().value

  const handleRemoveMember = useCallback(
    async (memberId: string) => {
      if (!session) return
      await tolerateNotFound(
        () => session.spacewave.removeOrgMember(orgId, memberId),
        'Member already removed. Refreshing...',
      )
    },
    [session, orgId],
  )

  return (
    <CollapsibleSection
      title="Members"
      icon={<LuUsers sx={stylexStyles2.rule3} />}
      open={open}
      onOpenChange={onOpenChange}
      badge={<span sx={stylexStyles3.rule16}>{members.length}</span>}
    >
      <p sx={stylexStyles3.rule17}>
        Members are shown by username first. Their account ID stays underneath
        for review or copy.
      </p>
      <OrgMemberList
        members={members}
        isOwner={isOwner}
        onRemove={handleRemoveMember}
      />
    </CollapsibleSection>
  )
}

/** useUsernameInvite sends an org invite to a Spacewave username. */
function useUsernameInvite(session: Session | null, orgId: string) {
  // Keep the invitation recipient, pending request and sent acknowledgement together.
  const [username, setUsername] = useState('')
  const [pending, setPending] = useState(false)
  const [sent, setSent] = useState(false)

  // Clear the sent acknowledgement when the recipient changes.
  const change = useCallback((next: string) => {
    setUsername(next)
    setSent(false)
  }, [])

  // Send one invitation to the currently entered username.
  const submit = useCallback(async () => {
    // Require a session and recipient before starting the invitation request.
    const target = username.trim()
    if (!session || !target || pending) return
    setPending(true)
    setSent(false)
    try {
      await session.spacewave.createOrganizationTargetedInvitationByUsername(
        target,
        orgId,
        'org:member',
      )
      setUsername('')
      setSent(true)
    } finally {
      setPending(false)
    }
  }, [session, orgId, username, pending])

  return { username, pending, sent, change, submit }
}

/** usernameInviteLabel returns the invite button label for its state. */
function usernameInviteLabel(pending: boolean, sent: boolean): string {
  if (pending) return 'Sending…'
  return sent ? 'Sent' : 'Invite'
}

/** UsernameInviteForm renders the username field and its invite button. */
function UsernameInviteForm({
  session,
  orgId,
}: {
  session: Session | null
  orgId: string
}) {
  const inputId = useId()
  const invite = useUsernameInvite(session, orgId)

  return (
    <div sx={stylexStyles3.rule18}>
      <label htmlFor={inputId} sx={stylexStyles3.rule19}>
        Spacewave username
      </label>
      <div sx={stylexStyles3.rule20}>
        <input
          id={inputId}
          value={invite.username}
          onChange={(e) => invite.change(e.target.value)}
          placeholder="alice"
          sx={[stylexStyles3.rule21]}
        />
        <DashboardButton
          icon={<LuUserPlus sx={stylexStyles2.rule4} />}
          onClick={() => void invite.submit()}
          disabled={invite.pending || !invite.username.trim() || !session}
        >
          {usernameInviteLabel(invite.pending, invite.sent)}
        </DashboardButton>
      </div>
    </div>
  )
}

/** OrgInvitesSection lists invites and lets owners create and revoke them. */
function OrgInvitesSection({
  orgId,
  invites,
  open,
  onOpenChange,
}: SectionProps & { orgId: string; invites: OrgInviteInfo[] }) {
  // Keep invitation creation tied to the current session and organization.
  const session = SessionContext.useContext().value
  const [creatingInvite, setCreatingInvite] = useState(false)

  // Create one invitation code and clear its pending state on completion.
  const handleCreateInvite = useCallback(async () => {
    if (!session || creatingInvite) return
    setCreatingInvite(true)
    try {
      await session.spacewave.createOrgInvite({ orgId, type: 'code' })
    } finally {
      setCreatingInvite(false)
    }
  }, [session, creatingInvite, orgId])

  // Revoke the selected invitation and tolerate a prior revocation.
  const handleRevokeInvite = useCallback(
    async (inviteId: string) => {
      if (!session) return
      await tolerateNotFound(
        () => session.spacewave.revokeOrgInvite(orgId, inviteId),
        'Invite already revoked. Refreshing...',
      )
    },
    [session, orgId],
  )

  return (
    <CollapsibleSection
      title="Invites"
      icon={<LuLink sx={stylexStyles2.rule3} />}
      open={open}
      onOpenChange={onOpenChange}
      badge={
        invites.length > 0 ? (
          <span sx={stylexStyles3.rule16}>{invites.length}</span>
        ) : undefined
      }
      headerActions={
        <button
          type="button"
          onClick={() => void handleCreateInvite()}
          disabled={creatingInvite}
          sx={collapsibleSectionActionStyles.button}
          aria-label="Create invite"
          title="Create invite"
        >
          <LuPlus sx={stylexStyles2.rule3} />
        </button>
      }
    >
      <UsernameInviteForm session={session} orgId={orgId} />
      <OrgInviteSection invites={invites} onRevoke={handleRevokeInvite} />
    </CollapsibleSection>
  )
}

/** focusInput focuses the input when it mounts. */
function focusInput(node: HTMLInputElement | null) {
  node?.focus()
}

/** useOrgRename edits and saves the organization display name. */
function useOrgRename(session: Session | null, orgId: string, orgName: string) {
  // Keep the organization rename draft and save state together.
  const [value, setValue] = useState('')
  const [renaming, setRenaming] = useState(false)
  const [saving, setSaving] = useState(false)

  // Seed the rename editor with the current organization name.
  const start = useCallback(() => {
    setValue(orgName)
    setRenaming(true)
  }, [orgName])

  // Discard the rename draft when editing is cancelled.
  const cancel = useCallback(() => {
    setRenaming(false)
    setValue('')
  }, [])

  // Persist a changed organization name before closing its editor.
  const save = useCallback(async () => {
    if (!session || saving || value.trim() === orgName) return
    setSaving(true)
    try {
      await session.spacewave.updateOrganization(orgId, value.trim())
      setRenaming(false)
    } finally {
      setSaving(false)
    }
  }, [session, orgId, value, orgName, saving])

  return { value, renaming, saving, setValue, start, cancel, save }
}

type OrgRename = ReturnType<typeof useOrgRename>

/** OrgRenameEditor renders the display name input with save and cancel. */
function OrgRenameEditor({
  inputId,
  orgName,
  rename,
}: {
  inputId: string
  orgName: string
  rename: OrgRename
}) {
  return (
    <div sx={stylexStyles3.rule22}>
      <input
        ref={focusInput}
        id={inputId}
        type="text"
        value={rename.value}
        onChange={(e) => rename.setValue(e.target.value)}
        onKeyDown={(e) => {
          if (e.nativeEvent.isComposing) return
          if (e.key === 'Enter') void rename.save()
          if (e.key === 'Escape') rename.cancel()
        }}
        sx={[stylexStyles3.rule23]}
      />
      <DashboardButton
        icon={<LuSave sx={stylexStyles2.rule4} />}
        onClick={() => void rename.save()}
        disabled={rename.saving || rename.value.trim() === orgName}
      >
        {rename.saving ? 'Saving…' : 'Save'}
      </DashboardButton>
      <DashboardButton
        icon={<LuX sx={stylexStyles2.rule4} />}
        onClick={rename.cancel}
        disabled={rename.saving}
      >
        Cancel
      </DashboardButton>
    </div>
  )
}

/** OrgRenameDisplay renders the display name with its edit affordances. */
function OrgRenameDisplay({
  orgName,
  onStart,
}: {
  orgName: string
  onStart: () => void
}) {
  return (
    <div sx={stylexStyles3.rule24}>
      <button
        type="button"
        sx={stylexStyles3.rule25}
        onDoubleClick={onStart}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault()
            onStart()
          }
        }}
      >
        {orgName}
      </button>
      <DashboardButton
        icon={<LuPencil sx={stylexStyles2.rule4} />}
        onClick={onStart}
      >
        Edit
      </DashboardButton>
    </div>
  )
}

/** OrgSettingsSection lets owners rename the organization. */
function OrgSettingsSection({
  orgId,
  orgName,
  open,
  onOpenChange,
}: SectionProps & { orgId: string; orgName: string }) {
  const renameInputId = useId()
  const session = SessionContext.useContext().value
  const rename = useOrgRename(session, orgId, orgName)

  return (
    <CollapsibleSection
      title="Settings"
      icon={<LuSettings sx={stylexStyles2.rule3} />}
      open={open}
      onOpenChange={onOpenChange}
    >
      <InfoCard>
        <div>
          <div sx={nativeChildSpacing.s2}>
            <label htmlFor={renameInputId} sx={stylexStyles3.rule26}>
              Display Name
            </label>
            {rename.renaming ? (
              <OrgRenameEditor
                inputId={renameInputId}
                orgName={orgName}
                rename={rename}
              />
            ) : (
              <OrgRenameDisplay orgName={orgName} onStart={rename.start} />
            )}
          </div>
        </div>
      </InfoCard>
    </CollapsibleSection>
  )
}

/** OrgOwnerSections renders the invite, settings, and billing sections. */
function OrgOwnerSections({
  orgId,
  orgName,
  billingAccountId,
  invites,
  sectionProps,
}: {
  orgId: string
  orgName: string
  billingAccountId?: string
  invites: OrgInviteInfo[]
  sectionProps: SectionPropsFor
}) {
  return (
    <>
      <OrgInvitesSection
        orgId={orgId}
        invites={invites}
        {...sectionProps('invites')}
      />
      <OrgSettingsSection
        orgId={orgId}
        orgName={orgName}
        {...sectionProps('settings')}
      />
      <OrgBillingSection
        orgId={orgId}
        billingAccountId={billingAccountId}
        {...sectionProps('billing')}
      />
    </>
  )
}

/** OrgHeader renders the organization title, leave action, and close button. */
function OrgHeader({
  orgId,
  orgName,
  roleLabel,
  canLeave,
  onCloseClick,
}: {
  orgId: string
  orgName: string
  roleLabel: string
  canLeave: boolean
  onCloseClick?: () => void
}) {
  const session = SessionContext.useContext().value
  const navigateSession = useSessionNavigate()

  const handleLeave = useCallback(async () => {
    if (!session) return
    await session.spacewave.leaveOrganization(orgId)
    navigateSession({ path: '' })
  }, [session, orgId, navigateSession])

  return (
    <div sx={stylexStyles3.rule27}>
      <div sx={stylexStyles3.rule28}>
        <div sx={stylexStyles3.rule29}>
          <LuBuilding2 sx={stylexStyles2.rule4} />
        </div>
        <span sx={stylexStyles3.rule30}>{orgName}</span>
        <span sx={stylexStyles3.rule31}>{roleLabel}</span>
      </div>
      <div sx={stylexStyles3.rule32}>
        {canLeave && (
          <Tooltip>
            <TooltipTrigger asChild>
              <DashboardButton
                icon={<LuLogOut sx={stylexStyles2.rule5} />}
                variant="destructive"
                onClick={() => void handleLeave()}
              >
                <span sx={stylexStyles3.rule33}>Leave</span>
              </DashboardButton>
            </TooltipTrigger>
            <TooltipContent side="bottom">Leave organization</TooltipContent>
          </Tooltip>
        )}
        {onCloseClick && (
          <Tooltip>
            <TooltipTrigger asChild>
              <DashboardButton
                icon={<LuX sx={stylexStyles2.rule5} />}
                onClick={onCloseClick}
              />
            </TooltipTrigger>
            <TooltipContent side="bottom">Close</TooltipContent>
          </Tooltip>
        )}
      </div>
    </div>
  )
}

/** OrgLoading renders the placeholder shown until the org state arrives. */
function OrgLoading() {
  return (
    <div sx={stylexStyles3.rule34}>
      <div sx={stylexStyles3.rule35}>
        <LoadingCard
          view={{
            state: 'active',
            title: 'Loading organization',
            detail: 'Reading the organization state and sharing details.',
          }}
        />
      </div>
    </div>
  )
}

// OrganizationDetails renders the overlay panel for organization management.
// Mirrors SessionDetails: header bar + scrollable collapsible sections.
export function OrganizationDetails({
  orgId,
  orgState,
  orgName: fallbackOrgName,
  degraded = false,
  isOwner,
  onCloseClick,
}: OrganizationDetailsProps) {
  // Persist which organization section is open for this dashboard.
  const ns = useStateNamespace(['org-details'])
  const [openSection, setOpenSection] = useStateAtom<OrgOpenSection>(
    ns,
    'open-section',
    degraded ? 'recovery' : 'members',
  )

  // Bind each section's controls to the single open-section value.
  const sectionProps = useCallback<SectionPropsFor>(
    (section) => ({
      open: openSection === section,
      onOpenChange: (open) => setOpenSection(open ? section : null),
    }),
    [openSection, setOpenSection],
  )

  // Resolve the organization name and role from the available identity.
  const info = orgState?.organization
  const { orgName, roleLabel } = orgIdentity(info, fallbackOrgName, isOwner)

  // Wait for organization state unless the dashboard is explicitly degraded.
  if (!orgState && !degraded) {
    return <OrgLoading />
  }

  return (
    <div sx={stylexStyles3.rule36}>
      <OrgHeader
        orgId={orgId}
        orgName={orgName}
        roleLabel={roleLabel}
        canLeave={!isOwner && !!info}
        onCloseClick={onCloseClick}
      />

      <div sx={stylexStyles3.rule37}>
        <div
          className={
            /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ 'space-y-3'
          }
        >
          {degraded && (
            <OrgRecoverySection
              orgId={orgId}
              rootState={orgState?.rootState}
              isOwner={isOwner}
              {...sectionProps('recovery')}
            />
          )}
          {orgState && (
            <OrgMembersSection
              orgId={orgId}
              members={orgState.members ?? []}
              isOwner={isOwner}
              {...sectionProps('members')}
            />
          )}
          {isOwner && orgState && (
            <OrgOwnerSections
              orgId={orgId}
              orgName={orgName}
              billingAccountId={info?.billingAccountId}
              invites={orgState.invites ?? []}
              sectionProps={sectionProps}
            />
          )}

          <CollapsibleSection
            title="Identifiers"
            icon={<LuFingerprint sx={stylexStyles2.rule3} />}
            {...sectionProps('identifiers')}
          >
            <InfoCard>
              <div>
                <CopyableField
                  sx={nativeChildSpacing.s2}
                  label="Organization ID"
                  value={orgId}
                />
              </div>
            </InfoCard>
          </CollapsibleSection>

          {isOwner && orgState && (
            <OrgActionsSection
              orgId={orgId}
              displayName={orgName}
              spaceCount={orgState.spaces?.length ?? 0}
            />
          )}
        </div>
      </div>
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
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
