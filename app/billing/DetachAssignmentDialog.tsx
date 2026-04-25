import { LuUnlink } from 'react-icons/lu'

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'

// DetachAssignmentTarget identifies the principal whose billing pointer is
// being cleared.
export interface DetachAssignmentTarget {
  ownerType: 'account' | 'organization'
  ownerId: string
  label: string
}

export interface DetachAssignmentDialogProps {
  target: DetachAssignmentTarget | null
  busy: boolean
  onCancel: () => void
  onConfirm: () => void
}

// DetachAssignmentDialog is the shared confirmation modal for clearing a
// billing account assignment from a principal. Controlled by the caller.
export function DetachAssignmentDialog({
  target,
  busy,
  onCancel,
  onConfirm,
}: DetachAssignmentDialogProps) {
  return (
    <Dialog
      open={target !== null}
      onOpenChange={(open) => {
        if (!open) onCancel()
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Detach billing from {target?.label}?</DialogTitle>
          <DialogDescription>
            The billing account assigned to{' '}
            <span className="text-foreground font-medium">{target?.label}</span>{' '}
            will be cleared. Resources owned by this principal will lose billing
            coverage and may move to the free tier or be blocked until a billing
            account is re-assigned.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <button
            onClick={onCancel}
            disabled={busy}
            className="text-foreground-alt hover:text-foreground cursor-pointer rounded px-3 py-1.5 text-xs transition-colors disabled:cursor-not-allowed disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            onClick={onConfirm}
            disabled={busy}
            className="border-destructive/30 bg-destructive/10 hover:bg-destructive/20 text-destructive flex cursor-pointer items-center gap-1 rounded-md border px-3 py-1.5 text-xs font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-50"
          >
            <LuUnlink className="h-3 w-3" />
            <span>{busy ? 'Detaching...' : 'Detach'}</span>
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
