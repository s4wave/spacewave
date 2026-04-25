import { useCallback } from 'react'

import { cn } from '@s4wave/web/style/utils.js'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'

export const SUPPORT_EMAIL = 'support@aperture.us'

export interface EmailSupportDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

// EmailSupportDialog explains how to contact support and offers a mailto action.
export function EmailSupportDialog({
  open,
  onOpenChange,
}: EmailSupportDialogProps) {
  const handleOpenEmail = useCallback(() => {
    window.open(`mailto:${SUPPORT_EMAIL}`)
    onOpenChange(false)
  }, [onOpenChange])

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Email Support</DialogTitle>
          <DialogDescription>
            Need a hand? Reach out at{' '}
            <a
              href={`mailto:${SUPPORT_EMAIL}`}
              className="text-brand hover:text-brand-highlight underline underline-offset-4"
            >
              {SUPPORT_EMAIL}
            </a>{' '}
            and we'll get back to you.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <button
            onClick={() => onOpenChange(false)}
            className="text-foreground-alt hover:text-foreground rounded-md px-4 py-2 text-sm transition-colors"
          >
            Close
          </button>
          <button
            onClick={handleOpenEmail}
            className={cn(
              'rounded-md border px-4 py-2 text-sm transition-all',
              'border-brand/30 bg-brand/10 text-brand hover:border-brand/50 hover:bg-brand/15',
            )}
          >
            Open Email
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
