import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'
import { cn } from '@s4wave/web/style/utils.js'

export interface LogoutConfirmDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  loggingOut: boolean
  onConfirm: () => void
}

// LogoutConfirmDialog confirms session removal before revoking local access.
export function LogoutConfirmDialog({
  open,
  onOpenChange,
  loggingOut,
  onConfirm,
}: LogoutConfirmDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Log out of session?</DialogTitle>
          <DialogDescription>
            This will remove the session from your device. You will need your
            password or backup key to sign in again.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <button
            onClick={() => onOpenChange(false)}
            disabled={loggingOut}
            className="text-foreground-alt hover:text-foreground rounded-md px-4 py-2 text-sm transition-colors"
          >
            Cancel
          </button>
          <button
            onClick={onConfirm}
            disabled={loggingOut}
            className={cn(
              'rounded-md border px-4 py-2 text-sm transition-all',
              'border-destructive/30 bg-destructive/10 hover:bg-destructive/20',
              'disabled:cursor-not-allowed disabled:opacity-50',
            )}
          >
            {loggingOut ? 'Logging out…' : 'Log out'}
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
