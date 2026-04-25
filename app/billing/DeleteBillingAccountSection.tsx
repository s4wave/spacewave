import { useCallback, useState } from 'react'
import { LuTrash2 } from 'react-icons/lu'

import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { cn } from '@s4wave/web/style/utils.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@s4wave/web/ui/tooltip.js'
import { BillingStatus } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import { deleteBillingAccountDisabledReason } from './billing-utils.js'

export interface DeleteBillingAccountSectionProps {
  billingAccountId: string
  displayName: string
  status?: BillingStatus
  assigneeCount: number
  disabledReasonOverride?: string | null
  onDeleted: () => void
}

// DeleteBillingAccountSection renders the billing-account delete action and its
// confirmation dialog.
export function DeleteBillingAccountSection({
  billingAccountId,
  displayName,
  status,
  assigneeCount,
  disabledReasonOverride,
  onDeleted,
}: DeleteBillingAccountSectionProps) {
  const session = SessionContext.useContext().value
  const [open, setOpen] = useState(false)
  const [confirmText, setConfirmText] = useState('')
  const [deleting, setDeleting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const disabledReason =
    disabledReasonOverride ??
    deleteBillingAccountDisabledReason(status, assigneeCount)
  const deleteDisabled = !session || deleting || !!disabledReason
  const confirmed = confirmText.trim() === 'DELETE'

  const handleOpenChange = useCallback(
    (next: boolean) => {
      if (deleting) return
      if (!next) {
        setConfirmText('')
        setError(null)
      }
      setOpen(next)
    },
    [deleting],
  )

  const handleDelete = useCallback(async () => {
    if (!session || !billingAccountId || deleting || !confirmed) return
    setDeleting(true)
    setError(null)
    try {
      await session.spacewave.deleteBillingAccount(billingAccountId)
      handleOpenChange(false)
      onDeleted()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Delete failed')
      setDeleting(false)
    }
  }, [
    session,
    billingAccountId,
    deleting,
    confirmed,
    handleOpenChange,
    onDeleted,
  ])

  const button = (
    <DashboardButton
      icon={<LuTrash2 className="h-3 w-3" />}
      onClick={() => handleOpenChange(true)}
      disabled={deleteDisabled}
      className="text-destructive hover:bg-destructive/10 hover:text-destructive disabled:hover:bg-transparent"
    >
      Delete billing account
    </DashboardButton>
  )

  return (
    <div className="space-y-2">
      <div className="text-foreground-alt/60 text-xs font-medium tracking-wider uppercase">
        Danger Zone
      </div>
      {disabledReason ?
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="inline-flex">{button}</span>
          </TooltipTrigger>
          <TooltipContent side="top" className="max-w-xs">
            {disabledReason}
          </TooltipContent>
        </Tooltip>
      : button}
      <div className="text-foreground-alt/40 text-xs">
        Permanently removes this billing account record after cancellation and
        detach are complete.
      </div>
      <Dialog open={open} onOpenChange={handleOpenChange}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Billing Account</DialogTitle>
            <DialogDescription>
              This permanently deletes{' '}
              <span className="text-foreground font-medium">
                {displayName || 'this billing account'}
              </span>
              . It cannot be assigned or reused again.
            </DialogDescription>
          </DialogHeader>
          <div>
            <label className="text-foreground-alt mb-1.5 block text-xs select-none">
              Type <span className="text-destructive font-medium">DELETE</span>{' '}
              to confirm.
            </label>
            <input
              type="text"
              value={confirmText}
              onChange={(e) => setConfirmText(e.target.value)}
              placeholder="DELETE"
              autoFocus
              className={cn(
                'border-foreground/20 bg-background/30 text-foreground placeholder:text-foreground-alt/50 w-full rounded-md border px-3 py-2 text-sm transition-colors outline-none',
                'focus:border-destructive/50',
              )}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && confirmed && !deleting) {
                  void handleDelete()
                }
              }}
            />
          </div>
          {error && <div className="text-destructive text-xs">{error}</div>}
          <DialogFooter>
            <button
              onClick={() => handleOpenChange(false)}
              disabled={deleting}
              className="text-foreground-alt hover:text-foreground rounded-md px-4 py-2 text-sm transition-colors disabled:cursor-not-allowed disabled:opacity-50"
            >
              Cancel
            </button>
            <button
              onClick={() => void handleDelete()}
              disabled={!confirmed || deleting}
              className={cn(
                'rounded-md border px-4 py-2 text-sm transition-all',
                'border-destructive bg-destructive/20 text-destructive hover:bg-destructive/30',
                'disabled:cursor-not-allowed disabled:opacity-50',
              )}
            >
              {deleting ? 'Deleting...' : 'Delete billing account'}
            </button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
