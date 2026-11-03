/* eslint-disable react-doctor/no-giant-component */
import { useCallback, useMemo, useState } from 'react'
import {
  LuBuilding2,
  LuPlus,
  LuSettings,
  LuZap,
} from '@s4wave/web/images/icons.js'
import { RxPerson } from '@s4wave/web/images/icons.js'

import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'

import {
  SessionContext,
  useSessionIndex,
  useSessionNavigate,
} from '@s4wave/web/contexts/contexts.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { usePromise } from '@s4wave/web/hooks/usePromise.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import { SpacewaveOrgListContext } from '@s4wave/web/contexts/SpacewaveOrgListContext.js'
import { Redirect } from '@s4wave/web/router/Redirect.js'
import { usePath } from '@s4wave/web/router/router.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { useSessionMetadata } from '@s4wave/app/hooks/useSessionMetadata.js'
import type { Session } from '@s4wave/sdk/session/session.js'
import type { ManagedBillingAccount } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import {
  isStatusActive,
  lifecycleStateLabel,
  subscriptionStatusBadgeStyles,
  statusLabel,
} from '@s4wave/app/billing/billing-utils.js'
import { PageFooter, PageWrapper } from './CloudConfirmationPage.js'
import { useBillingAccountCheckout } from './useBillingAccountCheckout.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
  radii as themeRadii,
  motion as themeMotion,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-widest)',
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
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    justifyContent: 'flex-start',
    marginTop: `calc(${themeLayout['--spacing']} * 4)`,
    width: '100%',
  },
  rule2: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 35%, transparent)`,
    display: 'inline-flex',
  },
  rule3: {
    borderRadius: 'var(--radius-xl)',
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-brand'],
    display: 'flex',
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule4: { minWidth: 0 },
  rule5: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeLayout['--text-metadata'],
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: '.18em',
    lineHeight: `var(--tw-leading, ${themeLayout['--text-metadata--line-height']})`,
    textTransform: 'uppercase',
  },
  rule6: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-tight)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    maxWidth: `calc(${themeLayout['--spacing']} * 64)`,
  },
  rule7: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
  },
  rule8: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wide)',
    fontSize: 'var(--text-xl)',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-wide)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule9: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textAlign: 'center',
    maxWidth: 'var(--container-md)',
  },
  rule10: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule11: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule12: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule13: { color: themeColors['--color-foreground'] },
  rule14: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      },
    },
    color: themeColors['--color-foreground'],
    cursor: 'pointer',
    display: 'inline-flex',
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule15: {
    '--tw-duration': '.15s',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
      },
    },
    transitionDuration: '.15s',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule16: {
    gap: themeLayout['--spacing'],
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    display: 'flex',
    flexDirection: 'column',
  },
  rule17: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    userSelect: 'none',
  },
  rule18: {
    WebkitUserSelect: 'none',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: '.6rem',
    lineHeight: 1.5,
    userSelect: 'none',
  },
  rule19: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'flex-end',
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
  },
  rule20: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    cursor: 'pointer',
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule21: { WebkitUserSelect: 'none', userSelect: 'none' },
  rule22: {
    '--tw-duration': '.3s',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    WebkitUserSelect: 'none',
    borderColor: themeColors['--color-brand'],
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      },
    },
    color: themeColors['--color-foreground'],
    cursor: { default: 'pointer', ':disabled': 'not-allowed' },
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    opacity: { default: null, ':disabled': 0.5 },
    transitionDuration: '.3s',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    userSelect: 'none',
  },
  rule23: { display: 'flex', justifyContent: 'center' },
  rule24: {
    '--tw-duration': '.3s',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    WebkitUserSelect: 'none',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 5)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      },
    },
    color: themeColors['--color-foreground'],
    cursor: { default: 'pointer', ':disabled': 'not-allowed' },
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    opacity: { default: null, ':disabled': 0.5 },
    transitionDuration: '.3s',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    userSelect: 'none',
  },
  rule25: {
    '--tw-duration': '.3s',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    WebkitUserSelect: 'none',
    borderColor: themeColors['--color-brand'],
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 5)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      },
    },
    color: themeColors['--color-foreground'],
    cursor: { default: 'pointer', ':disabled': 'not-allowed' },
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    opacity: { default: null, ':disabled': 0.5 },
    transitionDuration: '.3s',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    userSelect: 'none',
  },
  rule26: { alignItems: 'center', display: 'flex', justifyContent: 'center' },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule2: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule3: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

interface BillingSetupTarget {
  ownerType: 'account' | 'organization'
  ownerId: string
  label: string
}

function getNoActiveBillingTargetOverride(path: string): {
  ownerType: 'account' | 'organization'
  ownerId: string
} | null {
  const query = path.split('?')[1] ?? ''
  const params = new URLSearchParams(query)
  const ownerType = params.get('ownerType')
  const ownerId = params.get('ownerId')
  if (ownerType !== 'organization' || !ownerId) {
    return null
  }
  return { ownerType, ownerId }
}

function isAssignedToTarget(
  ba: ManagedBillingAccount,
  target: BillingSetupTarget,
): boolean {
  return (ba.assignees ?? []).some(
    (assignee) =>
      assignee.ownerType === target.ownerType &&
      assignee.ownerId === target.ownerId,
  )
}

type BillingCheckout = ReturnType<typeof useBillingAccountCheckout>

/** useManagedBillingAccounts loads the caller's managed billing accounts and exposes a reload. */
function useManagedBillingAccounts(session: Session | null | undefined) {
  const [reloadKey, setReloadKey] = useState(0)
  const { data, loading, error } = usePromise(
    useCallback(
      (signal: AbortSignal) =>
        session?.spacewave.listManagedBillingAccounts(signal) ??
        Promise.resolve(null),
      [session, reloadKey], // eslint-disable-line react-hooks/exhaustive-deps -- reloadKey triggers re-fetch
    ),
  )
  const accounts = useMemo<ManagedBillingAccount[]>(
    () => data?.accounts ?? [],
    [data?.accounts],
  )
  const reload = useCallback(() => {
    setReloadKey((k) => k + 1)
  }, [])

  return { accounts, loading, error, reload }
}

/** useBillingTarget resolves the owner the billing account is being set up for. */
function useBillingTarget(
  session: Session | null | undefined,
): BillingSetupTarget {
  const path = usePath()
  const sessionIdx = useSessionIndex() || null
  const sessionMetadata = useSessionMetadata(sessionIdx)
  const { accountId: callerAccountId } = useSessionInfo(session)
  const orgListCtx = SpacewaveOrgListContext.useContextSafe()
  const targetOverride = useMemo(
    () => getNoActiveBillingTargetOverride(path),
    [path],
  )

  return useMemo<BillingSetupTarget>(() => {
    if (targetOverride?.ownerType === 'organization') {
      const org = (orgListCtx?.organizations ?? []).find(
        (item) => item.id === targetOverride.ownerId,
      )
      return {
        ownerType: 'organization',
        ownerId: targetOverride.ownerId,
        label: org?.displayName || org?.id || 'Organization',
      }
    }
    return {
      ownerType: 'account',
      ownerId: callerAccountId,
      label:
        sessionMetadata?.displayName ||
        sessionMetadata?.cloudEntityId ||
        'Personal account',
    }
  }, [
    callerAccountId,
    orgListCtx?.organizations,
    sessionMetadata?.cloudEntityId,
    sessionMetadata?.displayName,
    targetOverride,
  ])
}

/**
 * useBillingAccountActions activates an existing billing account for the
 * target or creates a new one, tracking the in-flight account and any error.
 */
function useBillingAccountActions(
  session: Session | null | undefined,
  target: BillingSetupTarget,
  checkout: BillingCheckout,
  reload: () => void,
) {
  const navigateSession = useSessionNavigate()
  const [activatingBaId, setActivatingBaId] = useState<string | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)

  const handleActivate = useCallback(
    async (ba: ManagedBillingAccount) => {
      const baId = ba.id ?? ''
      if (
        !session ||
        !target.ownerId ||
        !baId ||
        activatingBaId ||
        checkout.polling
      ) {
        return
      }
      setActivatingBaId(baId)
      setActionError(null)
      try {
        if (isStatusActive(ba.subscriptionStatus)) {
          if (!isAssignedToTarget(ba, target)) {
            await session.spacewave.assignBillingAccount(
              baId,
              target.ownerType,
              target.ownerId,
            )
          }
          navigateSession({ path: 'setup' })
          return
        }
        await checkout.startCheckout(baId)
      } catch (e) {
        setActionError(e instanceof Error ? e.message : String(e))
      } finally {
        setActivatingBaId(null)
      }
    },
    [activatingBaId, checkout, navigateSession, session, target],
  )

  const handleCreate = useCallback(async () => {
    if (!session || creating || checkout.polling) return
    setCreating(true)
    try {
      const baId =
        await session.spacewave.createBillingAccount('Billing Account')
      reload()
      await checkout.startCheckout(baId)
    } catch (e) {
      setActionError(e instanceof Error ? e.message : String(e))
    } finally {
      setCreating(false)
    }
  }, [checkout, creating, reload, session])

  return {
    activatingBaId,
    actionError,
    creating,
    handleActivate,
    handleCreate,
  }
}

/** BillingTargetBadge shows who the billing account is being set up for. */
function BillingTargetBadge({ target }: { target: BillingSetupTarget }) {
  return (
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        <div sx={stylexStyles2.rule3}>
          {target.ownerType === 'organization' ? (
            <LuBuilding2 sx={stylexStyles.rule1} />
          ) : (
            <RxPerson sx={stylexStyles.rule1} />
          )}
        </div>
        <div sx={stylexStyles2.rule4}>
          <div sx={stylexStyles2.rule5}>Setting billing for</div>
          <div sx={stylexStyles2.rule6}>{target.label}</div>
        </div>
      </div>
    </div>
  )
}

/** BillingHeader renders the logo, title, and subtitle for the reactivation prompt. */
function BillingHeader({ hasAccounts }: { hasAccounts: boolean }) {
  const title = hasAccounts
    ? 'Reactivate a billing account'
    : 'Create a billing account'
  const subtitle = hasAccounts
    ? 'None of your billing accounts are currently active. Reactivate one below or create a new one to continue using Spacewave Cloud.'
    : 'Create a billing account to continue using Spacewave Cloud.'

  return (
    <div sx={stylexStyles2.rule7}>
      <AnimatedLogo followMouse={false} />
      <h1 sx={stylexStyles2.rule8}>{title}</h1>
      <p sx={stylexStyles2.rule9}>{subtitle}</p>
    </div>
  )
}

/** ErrorNotice renders a destructive-toned error message. */
function ErrorNotice({ message }: { message: string }) {
  return <div sx={stylexStyles2.rule10}>{message}</div>
}

/** CheckoutPollingNotice explains that activation is pending and offers the Stripe retry. */
function CheckoutPollingNotice({ checkout }: { checkout: BillingCheckout }) {
  return (
    <div sx={stylexStyles2.rule11}>
      <div sx={stylexStyles2.rule12}>
        <Spinner variant="brand" />
        <span sx={stylexStyles2.rule13}>
          Activating subscription, this page will update when confirmation
          arrives.
        </span>
      </div>
      {checkout.showRetry && (
        <button
          type="button"
          onClick={checkout.continueCheckout}
          sx={stylexStyles2.rule14}
        >
          <LuZap sx={stylexStyles.rule2} />
          <span>Continue with Stripe</span>
        </button>
      )}
    </div>
  )
}

interface BillingAccountRowProps {
  account: ManagedBillingAccount
  busy: boolean
  disabled: boolean
  onManage: (baId: string) => void
  onActivate: (ba: ManagedBillingAccount) => Promise<void>
}

/** BillingAccountRow lists one billing account with its manage and activate actions. */
function BillingAccountRow({
  sx,
  account,
  busy,
  disabled,
  onManage,
  onActivate,
}: BillingAccountRowProps & { sx?: stylex.StyleXStyles; className?: never }) {
  const baId = account.id ?? ''
  const activateLabel = isStatusActive(account.subscriptionStatus)
    ? 'Use this billing account'
    : 'Activate'

  return (
    <li sx={[[stylexStyles2.rule15], sx]}>
      <div sx={stylexStyles2.rule16}>
        <div sx={stylexStyles2.rule12}>
          <span sx={stylexStyles2.rule17}>{account.displayName || baId}</span>
          <span
            sx={[
              [stylexStyles3.rule1],
              [subscriptionStatusBadgeStyles(account.subscriptionStatus)],
            ]}
          >
            {statusLabel(account.subscriptionStatus)}
          </span>
        </div>
        {account.lifecycleState && (
          <div sx={stylexStyles2.rule18}>
            {lifecycleStateLabel(account.lifecycleState)}
          </div>
        )}
      </div>
      <div sx={stylexStyles2.rule19}>
        <button
          type="button"
          onClick={() => onManage(baId)}
          sx={stylexStyles2.rule20}
        >
          <LuSettings sx={stylexStyles.rule2} />
          <span sx={stylexStyles2.rule21}>Manage</span>
        </button>
        <button
          type="button"
          onClick={() => void onActivate(account)}
          disabled={busy || disabled}
          sx={[stylexStyles2.rule22]}
        >
          {busy ? <Spinner size="sm" /> : <LuZap sx={stylexStyles.rule2} />}
          <span>{busy ? 'Starting…' : activateLabel}</span>
        </button>
      </div>
    </li>
  )
}

interface CreateBillingAccountButtonProps {
  creating: boolean
  disabled: boolean
  hasAccounts: boolean
  onCreate: () => Promise<void>
}

/** CreateBillingAccountButton starts creating a new billing account. */
function CreateBillingAccountButton({
  creating,
  disabled,
  hasAccounts,
  onCreate,
}: CreateBillingAccountButtonProps) {
  return (
    <div sx={stylexStyles2.rule23}>
      <button
        type="button"
        onClick={() => void onCreate()}
        disabled={creating || disabled}
        sx={hasAccounts ? stylexStyles2.rule24 : stylexStyles2.rule25}
      >
        {creating ? <Spinner /> : <LuPlus sx={stylexStyles.rule3} />}
        <span>{creating ? 'Creating…' : 'Create new billing account'}</span>
      </button>
    </div>
  )
}

// NoActiveBillingAccountPage lists the caller's managed billing accounts when
// none are currently active, and offers per-row activation plus a create-new
// CTA that routes through the standard checkout flow.
export function NoActiveBillingAccountPage() {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const navigateSession = useSessionNavigate()
  const checkout = useBillingAccountCheckout({
    onCompleted: () => navigateSession({ path: 'setup' }),
  })
  const { accounts, loading, error, reload } =
    useManagedBillingAccounts(session)
  const target = useBillingTarget(session)
  const {
    activatingBaId,
    actionError,
    creating,
    handleActivate,
    handleCreate,
  } = useBillingAccountActions(session, target, checkout, reload)

  const hasAccounts = accounts.length > 0
  const targetHasAssignedActiveBilling = accounts.some(
    (ba) =>
      isStatusActive(ba.subscriptionStatus) && isAssignedToTarget(ba, target),
  )
  const disableActions = !session || checkout.polling || !target.ownerId
  const handleManage = useCallback(
    (baId: string) => {
      navigateSession({ path: `billing/${baId}` })
    },
    [navigateSession],
  )

  // Relative redirects resolve against the current URL, not the session
  // base. This component renders at /plan/no-active, so two levels up is
  // the session root. A bare "../setup" would land on /plan/setup which
  // has no route.
  if (targetHasAssignedActiveBilling) {
    return <Redirect to="../../setup" />
  }

  return (
    <PageWrapper>
      {checkout.consentDialog}
      <BillingTargetBadge target={target} />
      <BillingHeader hasAccounts={hasAccounts} />

      {loading && (
        <div sx={stylexStyles2.rule26}>
          <LoadingInline
            label="Loading billing accounts"
            tone="muted"
            size="sm"
          />
        </div>
      )}
      {error && <ErrorNotice message={error.message} />}
      {(actionError || checkout.error) && (
        <ErrorNotice message={actionError || checkout.error || ''} />
      )}
      {checkout.polling && <CheckoutPollingNotice checkout={checkout} />}

      {hasAccounts && (
        <ul>
          {accounts.map((ba) => (
            <BillingAccountRow
              sx={nativeChildSpacing.s2}
              key={ba.id ?? ''}
              account={ba}
              busy={activatingBaId === (ba.id ?? '')}
              disabled={disableActions}
              onManage={handleManage}
              onActivate={handleActivate}
            />
          ))}
        </ul>
      )}

      <CreateBillingAccountButton
        creating={creating}
        disabled={disableActions}
        hasAccounts={hasAccounts}
        onCreate={handleCreate}
      />

      <PageFooter />
    </PageWrapper>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
  },
})
