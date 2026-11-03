import { useCallback, useState } from 'react'
import { LuCheck, LuCreditCard, LuX } from '@s4wave/web/images/icons.js'

import {
  SessionContext,
  useSessionNavigate,
} from '@s4wave/web/contexts/contexts.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { CollapsibleSection } from '@s4wave/web/ui/CollapsibleSection.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@s4wave/web/ui/DropdownMenu.js'
import { DropdownTriggerButton } from '@s4wave/web/ui/DropdownTriggerButton.js'
import { usePromise } from '@s4wave/web/hooks/usePromise.js'
import { BillingStatus } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

import { BillingAccountCard } from '../billing/BillingAccountCard.js'
import {
  BillingStateProvider,
  useBillingStateContext,
} from '../billing/BillingStateProvider.js'
import { isStatusActive } from '../billing/billing-utils.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  fonts as themeFonts,
  motion as themeMotion,
  radii as themeRadii,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: `color-mix(in oklab, ${themeColors['--color-brand']} 60%, transparent)`,
  },
  rule2: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule3: {
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'center',
    columnGap: `calc(${themeLayout['--spacing']} * 4)`,
    rowGap: themeLayout['--spacing'],
  },
  rule4: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'flex-start',
    gap: themeLayout['--spacing'],
  },
  rule5: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule6: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule7: {
    display: 'flex',
    cursor: { default: 'pointer', ':disabled': 'not-allowed' },
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-destructive'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule8: {
    cursor: { default: 'pointer', ':disabled': 'not-allowed' },
    borderRadius: '.25rem',
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule9: {
    display: 'flex',
    cursor: { default: 'pointer', ':disabled': 'not-allowed' },
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-destructive'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    opacity: { default: null, ':disabled': 0.5 },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule3: { color: themeColors['--color-brand'] },
  rule4: { color: '#0000' },
})

export interface OrgBillingSectionProps {
  orgId: string
  billingAccountId?: string
  open: boolean
  onOpenChange: (open: boolean) => void
}

// OrgBillingSection renders the Billing collapsible in OrganizationDetails.
// Watches the org's assigned BillingAccount via BillingStateProvider and
// surfaces status/interval/next-date, a Manage link to the BA detail view,
// and an owner-only picker that reassigns the org to a different managed BA.
export function OrgBillingSection({
  orgId,
  billingAccountId,
  open,
  onOpenChange,
}: OrgBillingSectionProps) {
  return (
    <BillingStateProvider billingAccountId={billingAccountId}>
      <CollapsibleSection
        title="Billing"
        icon={<LuCreditCard sx={stylexStyles.rule1} />}
        open={open}
        onOpenChange={onOpenChange}
        badge={<OrgBillingBadge />}
      >
        <OrgBillingBody orgId={orgId} billingAccountId={billingAccountId} />
      </CollapsibleSection>
    </BillingStateProvider>
  )
}

function OrgBillingBadge() {
  const { response } = useBillingStateContext()
  const billing = response?.billingAccount
  const hasActive = billing && isStatusActive(billing.status)
  if (!hasActive) return null
  return <span sx={stylexStyles2.rule1}>Active</span>
}

function OrgBillingBody({
  orgId,
  billingAccountId,
}: {
  orgId: string
  billingAccountId?: string
}) {
  const { response, loading } = useBillingStateContext()
  const navigateSession = useSessionNavigate()
  const billing = response?.billingAccount

  const handleManage = useCallback(() => {
    if (!billingAccountId) return
    navigateSession({ path: `billing/${billingAccountId}` })
  }, [navigateSession, billingAccountId])

  return (
    <div
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: space-y-2 */ 'space-y-2'
      }
    >
      {!billingAccountId && (
        <InfoCard>
          <p sx={stylexStyles2.rule2}>
            No billing account assigned to this organization.
          </p>
        </InfoCard>
      )}
      {billingAccountId && loading && !billing && (
        <InfoCard>
          <LoadingInline label="Loading billing info" tone="muted" size="sm" />
        </InfoCard>
      )}
      {billingAccountId &&
        (!billing || billing.status === BillingStatus.BillingStatus_NONE) && (
          <InfoCard>
            <p sx={stylexStyles2.rule2}>No active billing on this account.</p>
          </InfoCard>
        )}
      {billingAccountId &&
        billing &&
        billing.status !== BillingStatus.BillingStatus_NONE && (
          <BillingAccountCard
            label="Billing account"
            billing={billing}
            onManage={handleManage}
          />
        )}
      <div sx={stylexStyles2.rule3}>
        <OrgBillingAccountPicker orgId={orgId} currentBaId={billingAccountId} />
        {billingAccountId && <OrgBillingDetachAction orgId={orgId} />}
      </div>
    </div>
  )
}

function OrgBillingAccountPicker({
  orgId,
  currentBaId,
}: {
  orgId: string
  currentBaId?: string
}) {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const [assigning, setAssigning] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const { data } = usePromise(
    useCallback(
      (signal: AbortSignal) =>
        session?.spacewave.listManagedBillingAccounts(signal) ??
        Promise.resolve(null),
      [session],
    ),
  )
  const bas = data?.accounts ?? []

  const handleAssign = useCallback(
    async (baId: string) => {
      if (!session || assigning || !baId || baId === currentBaId) return
      setAssigning(true)
      setError(null)
      try {
        await session.spacewave.assignBillingAccount(
          baId,
          'organization',
          orgId,
        )
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e))
      } finally {
        setAssigning(false)
      }
    },
    [session, assigning, orgId, currentBaId],
  )

  if (bas.length === 0) return null

  const triggerLabel = currentBaId
    ? assigning
      ? 'Assigning...'
      : 'Change billing account'
    : assigning
      ? 'Assigning...'
      : 'Assign billing account'

  return (
    <div sx={stylexStyles2.rule4}>
      <DropdownMenu>
        <DropdownMenuTrigger asChild disabled={!session || assigning}>
          <DropdownTriggerButton triggerStyle="ghost">
            {triggerLabel}
          </DropdownTriggerButton>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start">
          <DropdownMenuLabel>Assign this organization to</DropdownMenuLabel>
          <DropdownMenuSeparator />
          {bas.map((ba) => {
            const baId = ba.id ?? ''
            const isSelected = baId === currentBaId
            const label = ba.displayName || baId
            return (
              <DropdownMenuItem
                key={baId}
                onSelect={() => void handleAssign(baId)}
              >
                <LuCheck
                  sx={[
                    stylexStyles.rule2,
                    isSelected ? stylexStyles.rule3 : stylexStyles.rule4,
                  ]}
                />
                <span sx={stylexStyles2.rule5}>{label}</span>
              </DropdownMenuItem>
            )
          })}
        </DropdownMenuContent>
      </DropdownMenu>
      {error && <span sx={stylexStyles2.rule6}>{error}</span>}
    </div>
  )
}

function OrgBillingDetachAction({ orgId }: { orgId: string }) {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const [open, setOpen] = useState(false)
  const [detaching, setDetaching] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleCancel = useCallback(() => {
    if (detaching) return
    setOpen(false)
    setError(null)
  }, [detaching])

  const handleConfirm = useCallback(async () => {
    if (!session || detaching) return
    setDetaching(true)
    setError(null)
    try {
      await session.spacewave.detachBillingAccount('organization', orgId)
      setOpen(false)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setDetaching(false)
    }
  }, [session, detaching, orgId])

  return (
    <div sx={stylexStyles2.rule4}>
      <button
        type="button"
        onClick={() => setOpen(true)}
        disabled={!session}
        sx={stylexStyles2.rule7}
      >
        <LuX sx={stylexStyles.rule2} />
        <span>Detach billing</span>
      </button>
      {error && <span sx={stylexStyles2.rule6}>{error}</span>}
      <Dialog
        open={open}
        onOpenChange={(next) => {
          if (!next) handleCancel()
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Detach billing from this organization?</DialogTitle>
            <DialogDescription>
              The billing account assigned to this organization will be cleared.
              Resources owned by this organization will lose billing coverage
              and may move to the free tier or be blocked until a billing
              account is re-assigned.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <button
              type="button"
              onClick={handleCancel}
              disabled={detaching}
              sx={stylexStyles2.rule8}
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={() => void handleConfirm()}
              disabled={detaching}
              sx={stylexStyles2.rule9}
            >
              <LuX sx={stylexStyles.rule2} />
              <span>{detaching ? 'Detaching…' : 'Detach'}</span>
            </button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
