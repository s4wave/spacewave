import { useBillingConsent } from '../provider/spacewave/useBillingConsent.js'
/* eslint-disable react-doctor/no-giant-component */
import { useCallback, useMemo, useState } from 'react'
import { LuCheck, LuCreditCard, LuPlus, LuX } from '@s4wave/web/images/icons.js'

import { useNavigate } from '@s4wave/web/router/router.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import { useSessionNavigate } from '@s4wave/web/contexts/contexts.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { SpacewaveOrgListContext } from '@s4wave/web/contexts/SpacewaveOrgListContext.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { usePromise } from '@s4wave/web/hooks/usePromise.js'
import {
  CheckoutStatus,
  type ManagedBillingAccount,
  type OrganizationInfo,
} from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@s4wave/web/ui/DropdownMenu.js'
import { BackButton } from '@s4wave/web/ui/BackButton.js'
import { DropdownTriggerButton } from '@s4wave/web/ui/DropdownTriggerButton.js'
import { EmptyState } from '@s4wave/web/ui/EmptyState.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { ORG_ROLE_OWNER } from '../org/org-constants.js'
import { useCloudProviderConfig } from '../provider/spacewave/useSpacewaveAuth.js'
import { getCheckoutResultBaseUrl } from '../provider/spacewave/checkout-url.js'
import {
  DetachAssignmentDialog,
  type DetachAssignmentTarget,
} from './DetachAssignmentDialog.js'
import {
  lifecycleStateLabel,
  subscriptionStatusBadgeStyles,
  statusLabel,
} from './billing-utils.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  fonts as themeFonts,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wider)',
    borderRadius: '3.40282e38px',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: '10px',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-wider)',
    lineHeight: 1.5,
    textTransform: 'uppercase',
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    alignItems: 'flex-start',
    display: 'flex',
    justifyContent: 'center',
    position: 'relative',
    height: '100%',
    overflowY: 'auto',
    paddingBottom: `calc(${themeLayout['--spacing']} * 8)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 16)`,
    width: '100%',
  },
  rule2: {
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    maxWidth: 'var(--container-md)',
    width: '100%',
  },
  rule3: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule4: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule5: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
    fontSize: 'var(--text-lg)',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-tight)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
  },
  rule6: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: themeLayout['--spacing'],
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      },
    },
    color: themeColors['--color-brand'],
    cursor: { default: 'pointer', ':disabled': 'not-allowed' },
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    opacity: { default: null, ':disabled': 0.5 },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule7: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule8: {
    marginInline: 'auto',
    maxWidth: 'var(--container-sm)',
    width: '100%',
  },
  rule9: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule10: {
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    overflow: 'hidden',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
      },
    },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule11: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    gap: themeLayout['--spacing'],
    cursor: 'pointer',
    display: 'flex',
    flexDirection: 'column',
    textAlign: 'left',
    width: '100%',
  },
  rule12: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule13: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule14: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
  },
  rule15: {
    flex: 1,
    gap: themeLayout['--spacing'],
    alignItems: 'center',
    display: 'flex',
    flexWrap: 'wrap',
    minWidth: 0,
  },
  rule16: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: themeLayout['--spacing'],
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: themeColors['--color-foreground-alt'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule17: {
    color: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-destructive'],
      },
    },
    cursor: 'pointer',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    color: themeColors['--color-foreground'],
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule2: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule3: {
    color: themeColors['--color-foreground-alt'],
    height: `calc(${themeLayout['--spacing']} * 7)`,
    width: `calc(${themeLayout['--spacing']} * 7)`,
  },
  rule4: {
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule5: { color: themeColors['--color-brand'] },
  rule6: { color: '#0000' },
})

// AssignTarget is one option offered in the per-row "Assign to..." menu.
interface AssignTarget {
  ownerType: 'account' | 'organization'
  ownerId: string
  label: string
}

function formatBillingAccountDate(
  timestampMs: number | string | bigint,
): string {
  return new Date(Number(timestampMs)).toLocaleDateString()
}

// BillingAccountsPage lists every BillingAccount the caller manages.
// Each row links into the detail view at /billing/:baId.
export function BillingAccountsPage() {
  const { requestConsent, consentDialog } = useBillingConsent()
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const orgList = SpacewaveOrgListContext.useContext()
  const navigate = useNavigate()
  const navigateSession = useSessionNavigate()
  const cloudProviderConfig = useCloudProviderConfig()
  const checkoutResultBaseUrl = getCheckoutResultBaseUrl(cloudProviderConfig)
  const { accountId: callerAccountId } = useSessionInfo(session)

  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)
  const [assigningBaId, setAssigningBaId] = useState<string | null>(null)
  const [assignError, setAssignError] = useState<string | null>(null)
  const [detachTarget, setDetachTarget] =
    useState<DetachAssignmentTarget | null>(null)
  const [detaching, setDetaching] = useState(false)
  const [detachError, setDetachError] = useState<string | null>(null)

  const { data, loading, error } = usePromise(
    useCallback(
      (signal: AbortSignal) =>
        session?.spacewave.listManagedBillingAccounts(signal) ??
        Promise.resolve(null),
      [session, reloadKey], // eslint-disable-line react-hooks/exhaustive-deps -- reloadKey triggers re-fetch after assign
    ),
  )

  const handleBack = useCallback(() => {
    navigate({ path: '../' })
  }, [navigate])

  const handleOpen = useCallback(
    (baId: string) => {
      navigateSession({ path: `billing/${baId}` })
    },
    [navigateSession],
  )

  const ownedOrgs: OrganizationInfo[] = useMemo(
    () => orgList.organizations.filter((o) => o.role === ORG_ROLE_OWNER),
    [orgList.organizations],
  )

  const assignTargets: AssignTarget[] = useMemo(() => {
    const list: AssignTarget[] = []
    if (callerAccountId) {
      list.push({
        ownerType: 'account',
        ownerId: callerAccountId,
        label: 'Personal account',
      })
    }
    for (const o of ownedOrgs) {
      if (!o.id) continue
      list.push({
        ownerType: 'organization',
        ownerId: o.id,
        label: o.displayName || o.id,
      })
    }
    return list
  }, [callerAccountId, ownedOrgs])

  const handleAssign = useCallback(
    async (baId: string, target: AssignTarget) => {
      if (!session || assigningBaId) return
      setAssigningBaId(baId)
      setAssignError(null)
      try {
        await session.spacewave.assignBillingAccount(
          baId,
          target.ownerType,
          target.ownerId,
        )
        setReloadKey((k) => k + 1)
      } catch (e) {
        setAssignError(e instanceof Error ? e.message : String(e))
      } finally {
        setAssigningBaId(null)
      }
    },
    [session, assigningBaId],
  )

  const handleDetachConfirm = useCallback(async () => {
    if (!session || !detachTarget || detaching) return
    setDetaching(true)
    setDetachError(null)
    try {
      await session.spacewave.detachBillingAccount(
        detachTarget.ownerType,
        detachTarget.ownerId,
      )
      setDetachTarget(null)
      setReloadKey((k) => k + 1)
    } catch (e) {
      setDetachError(e instanceof Error ? e.message : String(e))
    } finally {
      setDetaching(false)
    }
  }, [session, detachTarget, detaching])

  const handleDetachCancel = useCallback(() => {
    if (detaching) return
    setDetachTarget(null)
    setDetachError(null)
  }, [detaching])

  const handleCreate = useCallback(async () => {
    if (!session || !checkoutResultBaseUrl || creating) return
    const consent = await requestConsent()
    if (!consent) return
    setCreating(true)
    setCreateError(null)
    try {
      const sw = session.spacewave
      const baId = await sw.createBillingAccount('Billing Account')
      const successUrl = checkoutResultBaseUrl + '/checkout/success'
      const cancelUrl = checkoutResultBaseUrl + '/checkout/cancel'
      const resp = await sw.createCheckoutSession({
        billingAccountId: baId,
        consent,
        successUrl,
        cancelUrl,
      })
      if (resp.status === CheckoutStatus.CheckoutStatus_COMPLETED) {
        navigateSession({ path: `billing/${baId}` })
        return
      }
      const url = resp.checkoutUrl ?? ''
      if (url) {
        window.open(url, '_blank', 'noopener,noreferrer')
      }
      navigateSession({ path: `billing/${baId}` })
    } catch (e) {
      setCreateError(e instanceof Error ? e.message : String(e))
    } finally {
      setCreating(false)
    }
  }, [
    session,
    checkoutResultBaseUrl,
    creating,
    navigateSession,
    requestConsent,
  ])

  const accounts: ManagedBillingAccount[] = data?.accounts ?? []

  return (
    <div sx={stylexStyles2.rule1}>
      {consentDialog}
      <BackButton floating onClick={handleBack}>
        Back
      </BackButton>
      <div sx={stylexStyles2.rule2}>
        <div sx={stylexStyles2.rule3}>
          <div sx={stylexStyles2.rule4}>
            <LuCreditCard sx={stylexStyles.rule1} />
            <h1 sx={stylexStyles2.rule5}>Billing Accounts</h1>
          </div>
          <button
            type="button"
            onClick={() => void handleCreate()}
            disabled={creating || !checkoutResultBaseUrl}
            sx={stylexStyles2.rule6}
          >
            <LuPlus sx={stylexStyles.rule2} />
            <span>{creating ? 'Creating…' : 'New billing account'}</span>
          </button>
        </div>
        {createError && <div sx={stylexStyles2.rule7}>{createError}</div>}
        {assignError && <div sx={stylexStyles2.rule7}>{assignError}</div>}
        {loading && (
          <div sx={stylexStyles2.rule8}>
            <LoadingCard
              view={{
                state: 'active',
                title: 'Loading billing accounts',
                detail: 'Reading your billing accounts from the cloud.',
              }}
            />
          </div>
        )}
        {error && <div sx={stylexStyles2.rule9}>{error.message}</div>}
        {!loading && !error && accounts.length === 0 && (
          <EmptyState
            icon={<LuCreditCard sx={stylexStyles.rule3} />}
            title="No billing accounts yet"
            description="A billing account holds your subscription. Create one, run checkout to activate it, then assign it to your personal account or to an organization you own."
            action={{
              label: creating
                ? 'Creating...'
                : 'Create your first BillingAccount',
              onClick: () => void handleCreate(),
            }}
            variant="panel"
          />
        )}
        {detachError && <div sx={stylexStyles2.rule7}>{detachError}</div>}
        {accounts.length > 0 && (
          <ul>
            {accounts.map((ba) => {
              const baId = ba.id ?? ''
              const assignees = ba.assignees ?? []
              const isBusy = assigningBaId === baId
              return (
                <li
                  key={baId}
                  sx={[nativeChildSpacing.s2, stylexStyles2.rule10]}
                >
                  <button
                    type="button"
                    onClick={() => handleOpen(baId)}
                    sx={stylexStyles2.rule11}
                  >
                    <div sx={stylexStyles2.rule4}>
                      <span sx={stylexStyles2.rule12}>
                        {ba.displayName || baId}
                      </span>
                      <span
                        sx={[
                          [stylexStyles3.rule1],
                          [
                            subscriptionStatusBadgeStyles(
                              ba.subscriptionStatus,
                            ),
                          ],
                        ]}
                      >
                        {statusLabel(ba.subscriptionStatus)}
                      </span>
                    </div>
                    <div suppressHydrationWarning sx={stylexStyles2.rule13}>
                      {[
                        lifecycleStateLabel(ba.lifecycleState),
                        ba.createdAt &&
                          `Created ${formatBillingAccountDate(ba.createdAt)}`,
                      ]
                        .filter(Boolean)
                        .join(' \u00b7 ')}
                    </div>
                  </button>
                  <div sx={stylexStyles2.rule14}>
                    <div sx={stylexStyles2.rule15}>
                      {assignees.length === 0 && (
                        <span sx={stylexStyles2.rule13}>Unassigned</span>
                      )}
                      {assignees.map((a) => {
                        const isPersonal =
                          a.ownerType === 'account' &&
                          a.ownerId === callerAccountId
                        const label = isPersonal
                          ? 'Personal'
                          : a.displayName || a.ownerId || ''
                        return (
                          <span
                            key={`${a.ownerType}:${a.ownerId}`}
                            sx={stylexStyles2.rule16}
                          >
                            <span>{label}</span>
                            <button
                              type="button"
                              onClick={() =>
                                setDetachTarget({
                                  ownerType: a.ownerType as
                                    | 'account'
                                    | 'organization',
                                  ownerId: a.ownerId ?? '',
                                  label,
                                })
                              }
                              aria-label={`Detach ${label}`}
                              sx={stylexStyles2.rule17}
                            >
                              <LuX sx={stylexStyles.rule4} />
                            </button>
                          </span>
                        )
                      })}
                    </div>
                    <DropdownMenu>
                      <DropdownMenuTrigger
                        asChild
                        disabled={
                          !session || isBusy || assignTargets.length === 0
                        }
                      >
                        <DropdownTriggerButton triggerStyle="ghost">
                          {isBusy ? 'Assigning…' : 'Assign to…'}
                        </DropdownTriggerButton>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuLabel>Assign this BA to</DropdownMenuLabel>
                        <DropdownMenuSeparator />
                        {assignTargets.map((t) => {
                          const isSelected = assignees.some(
                            (a) =>
                              a.ownerType === t.ownerType &&
                              a.ownerId === t.ownerId,
                          )
                          return (
                            <DropdownMenuItem
                              key={`${t.ownerType}:${t.ownerId}`}
                              onSelect={() => void handleAssign(baId, t)}
                            >
                              <LuCheck
                                sx={[
                                  stylexStyles.rule4,
                                  isSelected
                                    ? stylexStyles.rule5
                                    : stylexStyles.rule6,
                                ]}
                              />
                              <span>{t.label}</span>
                            </DropdownMenuItem>
                          )
                        })}
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </div>
                </li>
              )
            })}
          </ul>
        )}
      </div>
      <DetachAssignmentDialog
        target={detachTarget}
        busy={detaching}
        onCancel={handleDetachCancel}
        onConfirm={() => void handleDetachConfirm()}
      />
    </div>
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
