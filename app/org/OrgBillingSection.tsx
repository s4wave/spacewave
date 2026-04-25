import { useCallback, useState } from 'react'
import { LuCheck, LuCreditCard, LuX } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
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
        icon={<LuCreditCard className="h-3.5 w-3.5" />}
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
  return (
    <span className="border-brand/15 text-brand/60 rounded-full border px-1.5 py-0.5 text-[0.55rem] font-medium">
      Active
    </span>
  )
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
    <div className="space-y-2">
      {!billingAccountId && (
        <InfoCard>
          <p className="text-foreground-alt text-xs">
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
            <p className="text-foreground-alt text-xs">
              No active billing on this account.
            </p>
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
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1">
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

  const triggerLabel =
    currentBaId ?
      assigning ? 'Assigning...'
      : 'Change billing account'
    : assigning ? 'Assigning...'
    : 'Assign billing account'

  return (
    <div className="flex flex-col items-start gap-1">
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
                  className={cn(
                    'h-3 w-3',
                    isSelected ? 'text-brand' : 'text-transparent',
                  )}
                />
                <span className="text-xs">{label}</span>
              </DropdownMenuItem>
            )
          })}
        </DropdownMenuContent>
      </DropdownMenu>
      {error && <span className="text-destructive text-[11px]">{error}</span>}
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
    <div className="flex flex-col items-start gap-1">
      <button
        onClick={() => setOpen(true)}
        disabled={!session}
        className="text-foreground-alt hover:text-destructive flex cursor-pointer items-center gap-1 text-[11px] transition-colors disabled:cursor-not-allowed disabled:opacity-50"
      >
        <LuX className="h-3 w-3" />
        <span>Detach billing</span>
      </button>
      {error && <span className="text-destructive text-[11px]">{error}</span>}
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
              onClick={handleCancel}
              disabled={detaching}
              className="text-foreground-alt hover:text-foreground cursor-pointer rounded px-3 py-1.5 text-xs transition-colors disabled:cursor-not-allowed disabled:opacity-50"
            >
              Cancel
            </button>
            <button
              onClick={() => void handleConfirm()}
              disabled={detaching}
              className="border-destructive/30 bg-destructive/10 hover:bg-destructive/20 text-destructive flex cursor-pointer items-center gap-1 rounded-md border px-3 py-1.5 text-xs font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-50"
            >
              <LuX className="h-3 w-3" />
              <span>{detaching ? 'Detaching...' : 'Detach'}</span>
            </button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
