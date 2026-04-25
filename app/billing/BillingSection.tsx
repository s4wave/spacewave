import { LuCreditCard } from 'react-icons/lu'

import { useSessionNavigate } from '@s4wave/web/contexts/contexts.js'
import { SpacewaveOrgListContext } from '@s4wave/web/contexts/SpacewaveOrgListContext.js'
import { CollapsibleSection } from '@s4wave/web/ui/CollapsibleSection.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'
import { useStateNamespace, useStateAtom } from '@s4wave/web/state/persist.js'
import { BillingStatus } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import { useBillingStateContextSafe } from './BillingStateProvider.js'
import { isStatusActive } from './billing-utils.js'
import { BillingAccountCard } from './BillingAccountCard.js'

export interface BillingSectionProps {
  isLocal: boolean
  open?: boolean
  onOpenChange?: (open: boolean) => void
  onNavigateToPath?: (path: string) => void
}

// BillingSection shows a collapsible billing section with personal and org
// billing account cards. Cloud-only.
export function BillingSection({
  isLocal,
  open,
  onOpenChange,
  onNavigateToPath,
}: BillingSectionProps) {
  const ns = useStateNamespace(['session-settings'])
  const [storedOpen, setStoredOpen] = useStateAtom(ns, 'billing', true)
  const sectionOpen = open ?? storedOpen
  const handleOpenChange = onOpenChange ?? setStoredOpen

  const navigateSession = useSessionNavigate()
  const billingState = useBillingStateContextSafe()

  const orgList = SpacewaveOrgListContext.useContextSafe()
  const orgs = orgList?.organizations ?? []

  if (isLocal) return null
  if (!billingState) {
    throw new Error(
      'Billing state context not found. Wrap component in BillingStateProvider.',
    )
  }

  const billing = billingState.response?.billingAccount

  const hasActive = billing && isStatusActive(billing.status)

  const handleNavigate = (path: string) => {
    if (onNavigateToPath) {
      onNavigateToPath(path)
      return
    }
    navigateSession({ path })
  }

  return (
    <CollapsibleSection
      title="Billing"
      icon={<LuCreditCard className="h-3.5 w-3.5" />}
      open={sectionOpen}
      onOpenChange={handleOpenChange}
      badge={
        hasActive ?
          <span className="border-brand/15 text-brand/60 rounded-full border px-1.5 py-0.5 text-[0.55rem] font-medium">
            Active
          </span>
        : undefined
      }
    >
      <div className="space-y-2">
        {billing && billing.status !== BillingStatus.BillingStatus_NONE && (
          <BillingAccountCard
            label="Personal"
            billing={billing}
            onManage={() => {
              if (billing.id) {
                handleNavigate(`billing/${billing.id}`)
                return
              }
              handleNavigate('billing')
            }}
          />
        )}
        {billing && billing.status === BillingStatus.BillingStatus_NONE && (
          <button
            onClick={() => {
              handleNavigate('plan')
            }}
            className="border-brand/20 bg-brand/5 hover:bg-brand/10 w-full cursor-pointer rounded-md border p-2.5 text-left transition-colors"
          >
            <span className="text-foreground text-xs font-medium">
              Upgrade to Cloud
            </span>
            <p className="text-foreground-alt/50 mt-0.5 text-[0.6rem]">
              Subscribe to enable cloud sync and backup
            </p>
          </button>
        )}
        {!billing && (
          <LoadingInline label="Loading billing info" tone="muted" size="sm" />
        )}
        {orgs
          .filter((org) => org.billingAccountId)
          .map((org) => (
            <button
              key={org.id}
              onClick={() => {
                handleNavigate(`org/${org.id}/billing`)
              }}
              className="border-foreground/6 bg-background-card/20 hover:border-foreground/12 w-full cursor-pointer rounded-md border p-2.5 text-left transition-colors"
            >
              <div className="flex items-center justify-between">
                <span className="text-foreground text-xs font-medium">
                  {org.displayName || org.id}
                </span>
                <span className="text-brand/60 text-xs">Manage</span>
              </div>
            </button>
          ))}
      </div>
    </CollapsibleSection>
  )
}
